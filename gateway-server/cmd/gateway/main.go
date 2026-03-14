package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"log/slog"

	"gopkg.in/yaml.v3"

	"siptunnel/internal/config"
	"siptunnel/internal/diagnostics"
	"siptunnel/internal/selfcheck"
	"siptunnel/internal/server"
	"siptunnel/internal/service/filetransfer"
	"siptunnel/internal/service/httpinvoke"
	"siptunnel/internal/service/siptcp"
	"siptunnel/internal/startupsummary"
)

var (
	version   = "dev"
	commit    = "unknown"
	buildTime = "unknown"
)

func main() {
	if err := runMain(os.Args[1:], runGatewayStartup); err != nil {
		log.Fatalf("command failed: %v", err)
	}
}

func runMain(args []string, startup func([]string)) error {
	if handled, err := handleConfigCommands(args); handled {
		return err
	}
	startup(args)
	return nil
}

func runGatewayStartup(args []string) {
	runMode := resolveRunMode()
	cliConfigPath, err := readCLIConfigPath(args)
	if err != nil {
		log.Fatalf("parse flags failed: %v", err)
	}
	defaultPort := readPort(runMode, runtime.GOOS)
	windowsExeDir := ""
	if runtime.GOOS == "windows" {
		windowsExeDir = executableDir(getExecutablePathOrEmpty(os.Executable))
		ensureWindowsDefaultDataDir(windowsExeDir)
	}
	paths := config.LoadStoragePathsFromEnv()
	if err := paths.EnsureWritable(); err != nil {
		log.Fatal(formatStartupFailure("startup directory validation", err, "", runtime.GOOS))
	}
	selfCheckInput, uiCfg, cfgLoad := buildSelfCheckInput(paths, cliConfigPath, defaultPort)
	rtpTransport, err := filetransfer.NewTransport(selfCheckInput.NetworkConfig.RTP.Transport)
	if err != nil {
		log.Fatal(formatStartupFailure("init rtp transport", err, "", runtime.GOOS))
	}
	if err := rtpTransport.Bootstrap(selfCheckInput.NetworkConfig.RTP); err != nil {
		log.Fatal(formatStartupFailure(fmt.Sprintf("bootstrap rtp transport mode=%s", rtpTransport.Mode()), err, "", runtime.GOOS))
	}
	portPool, err := filetransfer.NewMemoryRTPPortPool(selfCheckInput.NetworkConfig.RTP.PortStart, selfCheckInput.NetworkConfig.RTP.PortEnd)
	if err != nil {
		log.Fatal(formatStartupFailure("init rtp port pool", err, "", runtime.GOOS))
	}
	sipMetrics := &siptcp.ConnectionMetrics{}
	var sipServer *siptcp.Server
	if selfCheckInput.NetworkConfig.SIP.Enabled && selfCheckInput.NetworkConfig.SIP.Transport == "TCP" {
		sipServer = siptcp.NewServer(siptcp.Config{
			ListenAddress:        selfCheckInput.NetworkConfig.SIP.ListenIP + ":" + strconv.Itoa(selfCheckInput.NetworkConfig.SIP.ListenPort),
			ReadTimeout:          time.Duration(selfCheckInput.NetworkConfig.SIP.ReadTimeoutMS) * time.Millisecond,
			WriteTimeout:         time.Duration(selfCheckInput.NetworkConfig.SIP.WriteTimeoutMS) * time.Millisecond,
			IdleTimeout:          time.Duration(selfCheckInput.NetworkConfig.SIP.IdleTimeoutMS) * time.Millisecond,
			MaxMessageBytes:      selfCheckInput.NetworkConfig.SIP.MaxMessageBytes,
			TCPKeepAliveEnabled:  selfCheckInput.NetworkConfig.SIP.TCPKeepAliveEnabled,
			TCPKeepAliveInterval: time.Duration(selfCheckInput.NetworkConfig.SIP.TCPKeepAliveIntervalMS) * time.Millisecond,
			TCPReadBufferBytes:   selfCheckInput.NetworkConfig.SIP.TCPReadBufferBytes,
			TCPWriteBufferBytes:  selfCheckInput.NetworkConfig.SIP.TCPWriteBufferBytes,
			MaxConnections:       selfCheckInput.NetworkConfig.SIP.MaxConnections,
		}, siptcp.MessageHandlerFunc(func(_ context.Context, meta siptcp.ConnectionMeta, payload []byte) ([]byte, error) {
			log.Printf("sip tcp message received transport=tcp connection_id=%s remote_addr=%s local_addr=%s bytes=%d", meta.ConnectionID, meta.RemoteAddr, meta.LocalAddr, len(payload))
			return payload, nil
		}), slog.Default(), sipMetrics)
		if err := sipServer.Start(context.Background()); err != nil {
			listenAddr := selfCheckInput.NetworkConfig.SIP.ListenIP + ":" + strconv.Itoa(selfCheckInput.NetworkConfig.SIP.ListenPort)
			log.Fatal(formatStartupFailure("start sip tcp server", err, listenAddr, runtime.GOOS))
		}
		defer func() {
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := sipServer.Shutdown(shutdownCtx); err != nil {
				log.Printf("shutdown sip tcp server failed: %v", err)
			}
		}()
	}
	log.Printf("network config loaded sip_transport=%s sip_listen=%s:%d rtp_transport=%s rtp_port_range=[%d,%d]", selfCheckInput.NetworkConfig.SIP.Transport, selfCheckInput.NetworkConfig.SIP.ListenIP, selfCheckInput.NetworkConfig.SIP.ListenPort, rtpTransport.Mode(), selfCheckInput.NetworkConfig.RTP.PortStart, selfCheckInput.NetworkConfig.RTP.PortEnd)
	if selfCheckInput.NetworkConfig.SIP.UDPMessageSizeRisk() {
		log.Printf("sip udp message size risk detected transport=%s max_message_bytes=%d recommended_max=%d", selfCheckInput.NetworkConfig.SIP.Transport, selfCheckInput.NetworkConfig.SIP.MaxMessageBytes, config.SIPUDPRecommendedMaxMessageBytes)
	}
	nodeID := resolveNodeID()
	var unifiedSummary startupsummary.Summary
	handler, closer, err := server.NewHandlerWithOptions(server.HandlerOptions{
		LogDir:   paths.LogDir,
		AuditDir: paths.AuditDir,
		DataDir:  paths.FinalDir,
		SelfCheckProvider: func(ctx context.Context) selfcheck.Report {
			return selfcheck.NewRunner().Run(ctx, selfCheckInput)
		},
		NetworkStatusFunc: func(context.Context) server.NodeNetworkStatus {
			poolStats := portPool.Stats()
			sipSnapshot := sipMetrics.Snapshot()
			rtpSnapshot := rtpTransport.Snapshot()
			mode := selfCheckInput.NetworkConfig.Mode.Normalize()
			capability := config.DeriveCapability(mode)
			return server.NodeNetworkStatus{
				NetworkMode: mode,
				Capability:  capability,
				CapabilitySummary: startupsummary.CapabilitySummary{
					Supported:   capability.SupportedFeatures(),
					Unsupported: capability.UnsupportedFeatures(),
					Items:       capability.Matrix(),
				},
				SIP: server.SIPNetworkStatus{
					ListenIP:                 selfCheckInput.NetworkConfig.SIP.ListenIP,
					ListenPort:               selfCheckInput.NetworkConfig.SIP.ListenPort,
					Transport:                selfCheckInput.NetworkConfig.SIP.Transport,
					CurrentSessions:          0,
					CurrentConnections:       int(sipSnapshot.CurrentConnections),
					AcceptedConnectionsTotal: sipSnapshot.AcceptedConnectionsTotal,
					ClosedConnectionsTotal:   sipSnapshot.ClosedConnectionsTotal,
					ReadTimeoutTotal:         sipSnapshot.ReadTimeoutTotal,
					WriteTimeoutTotal:        sipSnapshot.WriteTimeoutTotal,
					ConnectionErrorTotal:     sipSnapshot.ConnectionErrorTotal,
					TCPKeepAliveEnabled:      selfCheckInput.NetworkConfig.SIP.TCPKeepAliveEnabled,
					TCPKeepAliveIntervalMS:   selfCheckInput.NetworkConfig.SIP.TCPKeepAliveIntervalMS,
					TCPReadBufferBytes:       selfCheckInput.NetworkConfig.SIP.TCPReadBufferBytes,
					TCPWriteBufferBytes:      selfCheckInput.NetworkConfig.SIP.TCPWriteBufferBytes,
					MaxConnections:           selfCheckInput.NetworkConfig.SIP.MaxConnections,
				},
				RTP: server.RTPNetworkStatus{
					ListenIP:            selfCheckInput.NetworkConfig.RTP.ListenIP,
					PortStart:           selfCheckInput.NetworkConfig.RTP.PortStart,
					PortEnd:             selfCheckInput.NetworkConfig.RTP.PortEnd,
					Transport:           rtpTransport.Mode(),
					ActiveTransfers:     poolStats.Used,
					UsedPorts:           poolStats.Used,
					AvailablePorts:      poolStats.Available,
					PortPoolTotal:       poolStats.Total,
					PortPoolUsed:        poolStats.Used,
					PortAllocFailTotal:  poolStats.AllocFailTotal,
					TCPSessionsCurrent:  rtpSnapshot.TCPSessionsCurrent,
					TCPSessionsTotal:    rtpSnapshot.TCPSessionsTotal,
					TCPReadErrorsTotal:  rtpSnapshot.TCPReadErrorsTotal,
					TCPWriteErrorsTotal: rtpSnapshot.TCPWriteErrorsTotal,
				},
				RecentBindErrors:    []string{},
				RecentNetworkErrors: []string{},
			}
		},
		StartupSummaryProvider: func(context.Context) startupsummary.Summary {
			return unifiedSummary
		},
	})
	if err != nil {
		log.Fatal(formatStartupFailure("initialize handler", err, "", runtime.GOOS))
	}
	if uiCfg.Enabled && uiCfg.Mode == "embedded" {
		handler, err = server.NewEmbeddedUIFallbackHandler(handler, server.EmbeddedUIOptions{BasePath: uiCfg.BasePath})
		if err != nil {
			log.Fatal(formatStartupFailure("initialize embedded ui handler", err, "", runtime.GOOS))
		}
	}

	report := selfcheck.NewRunner().Run(context.Background(), selfCheckInput)
	if raw, err := json.Marshal(report); err == nil {
		log.Printf("env_self_check_report=%s", raw)
	}
	log.Print(report.ToCLI())
	if shouldBlockStartupOnSelfCheckError(report, runMode) {
		log.Fatal(formatStartupFailure("environment self-check", errors.New("please fix errors before startup"), "", runtime.GOOS))
	}
	if report.Overall == selfcheck.LevelError {
		log.Printf("environment self-check has blocking-level findings, but startup is allowed in run_mode=%s (degraded mode)", runMode)
	}
	unifiedSummary = buildStartupSummary(nodeID, cfgLoad, uiCfg, selfCheckInput.NetworkConfig, paths, defaultPort, rtpTransport.Mode(), report, len(selfCheckInput.DownstreamRoutes), runMode)

	if closer != nil {
		defer func() {
			if err := closer.Close(); err != nil {
				log.Printf("close resources failed: %v", err)
			}
		}()
	}

	httpServer := &http.Server{
		Addr:              resolveHTTPListenAddr(defaultPort, uiCfg),
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
	}
	pprofCfg := diagnostics.LoadPprofConfigFromEnv()
	pprofServer, err := diagnostics.StartPprofServer(pprofCfg, slog.Default())
	if err != nil {
		log.Fatal(formatStartupFailure("start pprof server", err, pprofCfg.ListenAddress, runtime.GOOS))
	}

	go func() {
		log.Printf("gateway server listening on %s (version=%s commit=%s build_time=%s)", httpServer.Addr, version, commit, buildTime)
		log.Print(unifiedSummary.ToLogText())
		if uiCfg.Mode == "external" {
			log.Printf("ui mode=external note=%q", "UI 由外部承载，请单独部署 gateway-ui 并将 API 指向 api_url")
		}
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal(formatStartupFailure("start gateway http server", err, httpServer.Addr, runtime.GOOS))
		}
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := diagnostics.ShutdownServer(ctx, pprofServer); err != nil {
		log.Printf("shutdown pprof server failed: %v", err)
	}
	if err := httpServer.Shutdown(ctx); err != nil {
		log.Printf("graceful shutdown failed: %v", err)
	}
}

func buildSelfCheckInput(paths config.StoragePaths, cliConfigPath string, defaultPort string) (selfcheck.Input, config.UIConfig, configLoadResult) {
	runtimeCfg, cfgLoad, err := loadRuntimeConfig(cliConfigPath)
	if parsedPort, parseErr := strconv.Atoi(defaultPort); parseErr == nil {
		runtimeCfg.UI.ApplyDefaults(parsedPort)
	}
	if err != nil {
		log.Fatalf("load network config failed: %v", err)
	}
	log.Printf("network config resolved path=%q source=%s", cfgLoad.Path, cfgLoad.Source)
	if cfgLoad.AutoGenerated {
		log.Printf("startup config bootstrap auto_generated=true mode=%s config_path=%q next_step=%q", cfgLoad.GeneratedAsMode, cfgLoad.Path, "review generated config and adjust sip/rtp/storage/ops before production rollout")
	}

	in := selfcheck.Input{
		NetworkConfig:   runtimeCfg.Network,
		StoragePaths:    paths,
		RunMode:         resolveRunMode(),
		SuggestFreePort: parseBoolEnv("GATEWAY_SELFCHECK_SUGGEST_FREE_PORT"),
	}
	if routePath := os.Getenv("GATEWAY_HTTPINVOKE_CONFIG"); routePath != "" {
		routeCfg, err := httpinvoke.LoadConfig(routePath)
		if err != nil {
			log.Printf("load GATEWAY_HTTPINVOKE_CONFIG=%q failed: %v", routePath, err)
			return in, runtimeCfg.UI, cfgLoad
		}
		in.DownstreamRoutes = routeCfg.Routes
	}
	return in, runtimeCfg.UI, cfgLoad
}

func buildStartupSummary(nodeID string, cfgLoad configLoadResult, uiCfg config.UIConfig, networkCfg config.NetworkConfig, storagePaths config.StoragePaths, defaultPort string, currentTransport string, report selfcheck.Report, routeCount int, runMode string) startupsummary.Summary {
	apiURL := fmt.Sprintf("http://127.0.0.1:%s/api", defaultPort)
	uiURL := "disabled"
	if uiCfg.Mode == "embedded" {
		host := uiCfg.ListenIP
		if host == "0.0.0.0" {
			host = "127.0.0.1"
		}
		apiURL = fmt.Sprintf("http://%s:%d/api", host, uiCfg.ListenPort)
		if uiCfg.Enabled {
			uiURL = fmt.Sprintf("http://%s:%d%s", host, uiCfg.ListenPort, uiCfg.BasePath)
		}
	} else if uiCfg.Enabled {
		uiURL = "external"
	}

	businessState := "active"
	businessMessage := "业务执行层已激活，下游 HTTP 路由映射可用"
	businessImpact := "A 网 HTTP 落地可执行"
	if routeCount <= 0 {
		businessState = "protocol_only"
		businessMessage = "协议层可启动，业务执行层未激活（未加载下游 HTTP 路由）"
		businessImpact = "仅完成 SIP/RTP 协议交互，不会执行 A 网 HTTP 落地"
	}

	mode := networkCfg.Mode.Normalize()
	capability := config.DeriveCapability(mode)
	transportPlan := config.ResolveTransportPlan(mode, capability)

	return startupsummary.Summary{
		NodeID:      nodeID,
		NetworkMode: mode,
		Capability:  capability,
		CapabilitySummary: startupsummary.CapabilitySummary{
			Supported:   capability.SupportedFeatures(),
			Unsupported: capability.UnsupportedFeatures(),
			Items:       capability.Matrix(),
		},
		TransportPlan:       transportPlan,
		ConfigPath:          cfgLoad.Path,
		ConfigSource:        string(cfgLoad.Source),
		RunMode:             runMode,
		AutoGeneratedConfig: cfgLoad.AutoGenerated,
		ConfigCandidates:    cfgLoad.Candidates,
		UIMode:              uiCfg.Mode,
		UIURL:               uiURL,
		APIURL:              apiURL,
		SIPListen: startupsummary.ListenEndpoint{
			IP:        networkCfg.SIP.ListenIP,
			Port:      networkCfg.SIP.ListenPort,
			Transport: strings.ToUpper(networkCfg.SIP.Transport),
		},
		RTPListen: startupsummary.RTPListen{
			IP:        networkCfg.RTP.ListenIP,
			PortRange: fmt.Sprintf("%d-%d", networkCfg.RTP.PortStart, networkCfg.RTP.PortEnd),
			Transport: strings.ToUpper(currentTransport),
		},
		StorageDirs: startupsummary.StorageDirs{
			TempDir:  storagePaths.TempDir,
			FinalDir: storagePaths.FinalDir,
			AuditDir: storagePaths.AuditDir,
			LogDir:   storagePaths.LogDir,
		},
		BusinessExecution: startupsummary.BusinessExecutionStatus{
			State:      businessState,
			RouteCount: routeCount,
			Message:    businessMessage,
			Impact:     businessImpact,
		},
		SelfCheckSummary: startupsummary.SelfCheckSummary{
			GeneratedAt: report.GeneratedAt.UTC().Format(time.RFC3339),
			Overall:     string(report.Overall),
			Info:        report.Summary.Info,
			Warn:        report.Summary.Warn,
			Error:       report.Summary.Error,
		},
	}
}

func resolveNodeID() string {
	nodeID := strings.TrimSpace(os.Getenv("GATEWAY_NODE_ID"))
	if nodeID == "" {
		return "gateway-a-01"
	}
	return nodeID
}

type configSource string

const (
	configSourceCLI              configSource = "cli"
	configSourceEnv              configSource = "env"
	configSourceExeDir           configSource = "exe_dir"
	configSourceCWD              configSource = "cwd"
	configSourceDefaultGenerated configSource = "default_generated"
)

type configCandidate struct {
	path   string
	source configSource
}

type configLoadResult struct {
	Path            string
	Source          configSource
	AutoGenerated   bool
	GeneratedAsMode string
	Candidates      []string
}

func readCLIConfigPath(args []string) (string, error) {
	fs := flag.NewFlagSet("gateway", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	configPath := fs.String("config", "", "gateway config file path")
	if err := fs.Parse(args); err != nil {
		return "", err
	}
	return strings.TrimSpace(*configPath), nil
}

func handleConfigCommands(args []string) (bool, error) {
	cmd, cmdArgs, ok := extractToolCommand(args)
	if !ok {
		return false, nil
	}
	switch cmd {
	case "init-config":
		cliPath, err := readCLIConfigPath(cmdArgs)
		if err != nil {
			return true, err
		}
		path := resolveConfigOutputPath(cliPath, os.Getenv("GATEWAY_CONFIG"), os.Getwd, os.Executable, runtime.GOOS)
		created, err := writeConfigIfNotExists(path, false)
		if err != nil {
			return true, err
		}
		if created {
			log.Printf("config initialized path=%q next_step=%q", path, "edit config then run gateway.exe --config <path>")
		} else {
			log.Printf("config already exists path=%q next_step=%q", path, "use print-default-config to view defaults")
		}
		if err := config.LoadStoragePathsFromEnv().EnsureWritable(); err != nil {
			return true, err
		}
		return true, nil
	case "print-default-config":
		b, err := defaultConfigYAML(runtime.GOOS)
		if err != nil {
			return true, err
		}
		fmt.Print(string(b))
		return true, nil
	case "validate-config":
		fs := flag.NewFlagSet("validate-config", flag.ContinueOnError)
		cfgPath := fs.String("f", "", "config file path")
		fs.SetOutput(os.Stderr)
		if err := fs.Parse(cmdArgs); err != nil {
			return true, err
		}
		if strings.TrimSpace(*cfgPath) == "" {
			return true, errors.New("validate-config requires -f <path>")
		}
		data, err := os.ReadFile(strings.TrimSpace(*cfgPath))
		if err != nil {
			return true, err
		}
		runtimeCfg, err := parseRuntimeConfigFromTopLevelYAML(data)
		if err != nil {
			return true, err
		}
		if err := runtimeCfg.Network.Validate(); err != nil {
			return true, err
		}
		if err := runtimeCfg.UI.Validate(); err != nil {
			return true, err
		}
		if err := config.LoadStoragePathsFromEnv().EnsureWritable(); err != nil {
			return true, err
		}
		log.Printf("config validation passed path=%q next_step=%q", *cfgPath, "start gateway with --config <path>")
		return true, nil
	default:
		return false, nil
	}
}

type runtimeConfig struct {
	Network config.NetworkConfig
	UI      config.UIConfig
}

func loadRuntimeConfig(cliConfigPath string) (runtimeConfig, configLoadResult, error) {
	candidate, candidates, ok := pickExistingConfigCandidate(cliConfigPath, os.Getenv("GATEWAY_CONFIG"), os.Executable, os.Getwd, fileExists)
	if !ok {
		generatedPath, generatedMode, err := handleMissingConfigFile(cliConfigPath, os.Getenv("GATEWAY_CONFIG"), os.Getwd)
		if err != nil {
			return runtimeConfig{}, configLoadResult{}, err
		}
		data, err := os.ReadFile(generatedPath)
		if err != nil {
			return runtimeConfig{}, configLoadResult{}, fmt.Errorf("read generated config path=%q: %w", generatedPath, err)
		}
		cfg, err := parseRuntimeConfigFromTopLevelYAML(data)
		if err != nil {
			return runtimeConfig{}, configLoadResult{}, fmt.Errorf("parse generated config path=%q: %w", generatedPath, err)
		}
		if err := cfg.Network.Validate(); err != nil {
			return runtimeConfig{}, configLoadResult{}, fmt.Errorf("validate generated network config path=%q: %w", generatedPath, err)
		}
		if err := cfg.UI.Validate(); err != nil {
			return runtimeConfig{}, configLoadResult{}, fmt.Errorf("validate generated ui config path=%q: %w", generatedPath, err)
		}
		return cfg, configLoadResult{Path: generatedPath, Source: configSourceDefaultGenerated, AutoGenerated: true, GeneratedAsMode: generatedMode, Candidates: candidates}, nil
	}

	data, err := os.ReadFile(candidate.path)
	if err != nil {
		return runtimeConfig{}, configLoadResult{}, fmt.Errorf("load network config path=%q source=%s: %w", candidate.path, candidate.source, err)
	}
	runtimeCfg, err := parseRuntimeConfigFromTopLevelYAML(data)
	if err != nil {
		return runtimeConfig{}, configLoadResult{}, fmt.Errorf("parse network config path=%q source=%s: %w", candidate.path, candidate.source, err)
	}
	if err := runtimeCfg.Network.Validate(); err != nil {
		return runtimeConfig{}, configLoadResult{}, fmt.Errorf("validate network config path=%q source=%s: %w", candidate.path, candidate.source, err)
	}
	if err := runtimeCfg.UI.Validate(); err != nil {
		return runtimeConfig{}, configLoadResult{}, fmt.Errorf("validate ui config path=%q source=%s: %w", candidate.path, candidate.source, err)
	}
	return runtimeCfg, configLoadResult{Path: candidate.path, Source: candidate.source, Candidates: candidates}, nil
}

type fullConfigFile struct {
	Server        map[string]any       `yaml:"server,omitempty"`
	SIP           config.SIPConfig     `yaml:"sip,omitempty"`
	RTP           config.RTPConfig     `yaml:"rtp,omitempty"`
	Storage       map[string]any       `yaml:"storage,omitempty"`
	Observability map[string]any       `yaml:"observability,omitempty"`
	UI            config.UIConfig      `yaml:"ui,omitempty"`
	Ops           map[string]any       `yaml:"ops,omitempty"`
	Network       config.NetworkConfig `yaml:"network,omitempty"`
}

func parseRuntimeConfigFromTopLevelYAML(data []byte) (runtimeConfig, error) {
	var fileCfg fullConfigFile
	if err := yaml.Unmarshal(data, &fileCfg); err != nil {
		return runtimeConfig{}, err
	}
	networkCfg := config.DefaultNetworkConfig()
	uiCfg := config.DefaultUIConfig()
	if fileCfg.Network != (config.NetworkConfig{}) {
		networkCfg = fileCfg.Network
	}
	if fileCfg.SIP != (config.SIPConfig{}) {
		networkCfg.SIP = fileCfg.SIP
	}
	if fileCfg.RTP != (config.RTPConfig{}) {
		networkCfg.RTP = fileCfg.RTP
	}
	networkCfg.ApplyDefaults()
	if fileCfg.UI != (config.UIConfig{}) {
		uiCfg = fileCfg.UI
	}
	uiCfg.ApplyDefaults(networkCfg.SIP.ListenPort)
	return runtimeConfig{Network: networkCfg, UI: uiCfg}, nil
}

func defaultConfigYAML(osName string) ([]byte, error) {
	networkCfg := config.DefaultNetworkConfig()
	if osName == "windows" {
		networkCfg.SIP.ListenPort = pickFriendlySIPPort()
	}
	yamlCfg := fullConfigFile{
		Server: map[string]any{"port": 18080},
		SIP:    networkCfg.SIP,
		RTP:    networkCfg.RTP,
		Storage: map[string]any{
			"temp_dir":  "./data/temp",
			"final_dir": "./data/final",
			"audit_dir": "./data/audit",
			"log_dir":   "./data/logs",
		},
		Observability: map[string]any{
			"log_level":       "info",
			"pprof_enabled":   false,
			"metrics_enabled": true,
		},
		UI: config.UIConfig{Enabled: true, Mode: "external", ListenIP: "127.0.0.1", ListenPort: 18080, BasePath: "/"},
		Ops: map[string]any{
			"run_mode":               "dev",
			"allow_auto_init_config": true,
		},
		Network: networkCfg,
	}
	b, err := yaml.Marshal(yamlCfg)
	if err != nil {
		return nil, err
	}
	return append([]byte("# generated by gateway init-config\n"), b...), nil
}

func resolveRunMode() string {
	mode := strings.ToLower(strings.TrimSpace(os.Getenv("GATEWAY_MODE")))
	if mode == "" {
		return "dev"
	}
	if mode == "test" || mode == "prod" || mode == "dev" {
		return mode
	}
	return "dev"
}

func shouldBlockStartupOnSelfCheckError(report selfcheck.Report, runMode string) bool {
	if report.Overall != selfcheck.LevelError {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(runMode), "prod")
}

func resolveConfigOutputPath(cliConfigPath string, envConfigPath string, getwd func() (string, error), executablePath func() (string, error), osName string) string {
	if p := strings.TrimSpace(cliConfigPath); p != "" {
		return p
	}
	if p := strings.TrimSpace(envConfigPath); p != "" {
		return p
	}
	if osName == "windows" {
		exeDir := executableDir(getExecutablePathOrEmpty(executablePath))
		if exeDir != "" {
			return filepath.Join(exeDir, "configs", "config.yaml")
		}
	}
	cwd, err := getwd()
	if err != nil {
		cwd = "."
	}
	return filepath.Join(cwd, "configs", "config.yaml")
}

func writeConfigIfNotExists(path string, template bool) (bool, error) {
	if fileExists(path) {
		return false, nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return false, err
	}
	b, err := defaultConfigYAML(runtime.GOOS)
	if err != nil {
		return false, err
	}
	if template {
		b = append([]byte("# PRODUCTION TEMPLATE: 请先修改参数后再启动网关\n"), b...)
	}
	if err := os.WriteFile(path, b, 0o644); err != nil {
		return false, err
	}
	return true, nil
}

func handleMissingConfigFile(cliConfigPath string, envConfigPath string, getwd func() (string, error)) (string, string, error) {
	mode := resolveRunMode()
	path := resolveConfigOutputPath(cliConfigPath, envConfigPath, getwd, os.Executable, runtime.GOOS)
	created, err := writeConfigIfNotExists(path, mode == "prod")
	if err != nil {
		return "", mode, err
	}
	if created {
		log.Printf("config file auto generated path=%q mode=%s", path, mode)
	} else {
		log.Printf("config file already exists, skip generation path=%q", path)
	}
	if mode == "prod" {
		return path, mode, errors.New("production mode detected, template generated. please edit config and restart")
	}
	return path, mode, nil
}

func pickExistingConfigCandidate(cliConfigPath string, envConfigPath string, executablePath func() (string, error), getwd func() (string, error), exists func(string) bool) (configCandidate, []string, bool) {
	exePath := getExecutablePathOrEmpty(executablePath)
	cwd, err := getwd()
	if err != nil {
		cwd = "."
	}
	allCandidates := configCandidates(cliConfigPath, envConfigPath, executableDir(exePath), cwd, runtime.GOOS)
	candidatePaths := make([]string, 0, len(allCandidates))
	for _, item := range allCandidates {
		candidatePaths = append(candidatePaths, fmt.Sprintf("%s (%s)", item.path, item.source))
	}
	for _, candidate := range allCandidates {
		if exists(candidate.path) {
			return candidate, candidatePaths, true
		}
	}
	return configCandidate{}, candidatePaths, false
}

func configCandidates(cliConfigPath string, envConfigPath string, exeDir string, cwd string, osName string) []configCandidate {
	candidates := make([]configCandidate, 0, 6)
	if p := strings.TrimSpace(cliConfigPath); p != "" {
		if osName == "windows" && filepath.IsAbs(p) == false && exeDir != "" {
			candidates = append(candidates, configCandidate{path: filepath.Join(exeDir, p), source: configSourceCLI})
		}
		candidates = append(candidates, configCandidate{path: p, source: configSourceCLI})
	}
	if p := strings.TrimSpace(envConfigPath); p != "" {
		if osName == "windows" && filepath.IsAbs(p) == false && exeDir != "" {
			candidates = append(candidates, configCandidate{path: filepath.Join(exeDir, p), source: configSourceEnv})
		}
		candidates = append(candidates, configCandidate{path: p, source: configSourceEnv})
	}
	if exeDir != "" {
		candidates = append(candidates,
			configCandidate{path: filepath.Join(exeDir, "configs", "config.yaml"), source: configSourceExeDir},
			configCandidate{path: filepath.Join(exeDir, "config.yaml"), source: configSourceExeDir},
		)
	}
	candidates = append(candidates,
		configCandidate{path: filepath.Join(cwd, "configs", "config.yaml"), source: configSourceCWD},
		configCandidate{path: filepath.Join(cwd, "config.yaml"), source: configSourceCWD},
	)
	return candidates
}

func getExecutablePathOrEmpty(executablePath func() (string, error)) string {
	if executablePath == nil {
		return ""
	}
	path, err := executablePath()
	if err != nil {
		return ""
	}
	return path
}

func ensureWindowsDefaultDataDir(exeDir string) {
	if exeDir == "" {
		return
	}
	if strings.TrimSpace(os.Getenv("GATEWAY_DATA_DIR")) != "" {
		return
	}
	for _, key := range []string{"GATEWAY_TEMP_DIR", "GATEWAY_FINAL_DIR", "GATEWAY_AUDIT_DIR", "GATEWAY_LOG_DIR"} {
		if strings.TrimSpace(os.Getenv(key)) != "" {
			return
		}
	}
	_ = os.Setenv("GATEWAY_DATA_DIR", filepath.Join(exeDir, "data"))
}

func formatStartupFailure(stage string, err error, listenAddr string, osName string) string {
	msg := fmt.Sprintf("%s failed: %v", stage, err)
	if osName != "windows" {
		return msg
	}
	b := &strings.Builder{}
	b.WriteString(msg)
	b.WriteString(" | Windows 排查建议: 1) 确认在 gateway.exe 所在目录执行，或显式传入 --config .\\configs\\config.yaml；2) 检查配置与数据目录是否具备写权限；3) 若路径来自快捷方式，请将‘起始位置’设置为程序目录")
	if isAddrInUseError(err) {
		b.WriteString(" | 端口冲突排查(PowerShell): Get-NetTCPConnection -LocalPort <端口> | Select-Object -First 5 -Property LocalAddress,LocalPort,State,OwningProcess; Get-Process -Id <PID> | Format-Table Id,ProcessName,Path")
		b.WriteString(" | 端口冲突排查(CMD): netstat -ano | findstr :<端口> && tasklist /FI \"PID eq <PID>\"")
		if strings.TrimSpace(listenAddr) != "" {
			b.WriteString(fmt.Sprintf(" | 当前监听地址=%s", listenAddr))
		}
	}
	return b.String()
}

func isAddrInUseError(err error) bool {
	if err == nil {
		return false
	}
	text := strings.ToLower(err.Error())
	return strings.Contains(text, "address already in use") || strings.Contains(text, "only one usage of each socket address")
}

func executableDir(executablePath string) string {
	if executablePath == "" {
		return ""
	}
	if strings.Contains(executablePath, "\\") {
		normalized := strings.ReplaceAll(executablePath, "\\", "/")
		return strings.ReplaceAll(path.Dir(normalized), "/", "\\")
	}
	return filepath.Dir(executablePath)
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func parseBoolEnv(name string) bool {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return false
	}
	v, err := strconv.ParseBool(raw)
	if err != nil {
		return false
	}
	return v
}

func resolveHTTPListenAddr(defaultPort string, uiCfg config.UIConfig) string {
	if uiCfg.Enabled && uiCfg.Mode == "embedded" {
		return fmt.Sprintf("%s:%d", uiCfg.ListenIP, uiCfg.ListenPort)
	}
	return ":" + defaultPort
}

func readPort(runMode string, osName string) string {
	port := os.Getenv("GATEWAY_PORT")
	if port == "" {
		if runMode == "prod" {
			return "18080"
		}
		return pickFriendlyStartupPort(osName)
	}
	if _, err := strconv.Atoi(port); err != nil {
		log.Printf("invalid GATEWAY_PORT=%q, fallback to 18080", port)
		return pickFriendlyStartupPort(osName)
	}
	return port
}

func pickFriendlyStartupPort(osName string) string {
	candidates := []int{18080, 18180, 18081, 8080}
	if osName == "windows" {
		candidates = []int{18180, 18080, 18081, 8080}
	}
	selected := pickFirstAvailablePort(candidates, func(port int) bool {
		return isTCPAddrAvailable("127.0.0.1", port)
	})
	return strconv.Itoa(selected)
}

func pickFriendlySIPPort() int {
	candidates := []int{59226, 15060, 25060, 35060, 5060}
	return pickFirstAvailablePort(candidates, func(port int) bool {
		return isTCPAddrAvailable("0.0.0.0", port)
	})
}

func pickFirstAvailablePort(candidates []int, checker func(int) bool) int {
	for _, port := range candidates {
		if checker(port) {
			return port
		}
	}
	return candidates[0]
}

func isTCPAddrAvailable(host string, port int) bool {
	ln, err := net.Listen("tcp", fmt.Sprintf("%s:%d", host, port))
	if err != nil {
		return false
	}
	_ = ln.Close()
	return true
}

func extractToolCommand(args []string) (string, []string, bool) {
	if len(args) == 0 {
		return "", nil, false
	}
	toolCommands := map[string]struct{}{"init-config": {}, "print-default-config": {}, "validate-config": {}}
	for idx, arg := range args {
		for _, token := range strings.Fields(strings.ToLower(strings.TrimSpace(strings.ReplaceAll(arg, `\\n`, "\n")))) {
			token = strings.Trim(token, `"'`)
			if _, ok := toolCommands[token]; ok {
				return token, args[idx+1:], true
			}
		}
	}
	return "", nil, false
}
