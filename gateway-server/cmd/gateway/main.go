package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path"
	"path/filepath"
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
)

var (
	version   = "dev"
	commit    = "unknown"
	buildTime = "unknown"
)

func main() {
	if handled, err := handleConfigCommands(os.Args[1:]); handled {
		if err != nil {
			log.Fatalf("command failed: %v", err)
		}
		return
	}

	cliConfigPath, err := readCLIConfigPath(os.Args[1:])
	if err != nil {
		log.Fatalf("parse flags failed: %v", err)
	}
	defaultPort := readPort()
	paths := config.LoadStoragePathsFromEnv()
	if err := paths.EnsureWritable(); err != nil {
		log.Fatalf("startup directory validation failed: %v", err)
	}
	selfCheckInput, uiCfg := buildSelfCheckInput(paths, cliConfigPath, defaultPort)
	rtpTransport, err := filetransfer.NewTransport(selfCheckInput.NetworkConfig.RTP.Transport)
	if err != nil {
		log.Fatalf("init rtp transport failed: %v", err)
	}
	if err := rtpTransport.Bootstrap(selfCheckInput.NetworkConfig.RTP); err != nil {
		log.Fatalf("bootstrap rtp transport mode=%s failed: %v", rtpTransport.Mode(), err)
	}
	portPool, err := filetransfer.NewMemoryRTPPortPool(selfCheckInput.NetworkConfig.RTP.PortStart, selfCheckInput.NetworkConfig.RTP.PortEnd)
	if err != nil {
		log.Fatalf("init rtp port pool failed: %v", err)
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
			log.Fatalf("start sip tcp server failed: %v", err)
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
	handler, closer, err := server.NewHandlerWithOptions(server.HandlerOptions{
		LogDir:   paths.LogDir,
		AuditDir: paths.AuditDir,
		SelfCheckProvider: func(ctx context.Context) selfcheck.Report {
			return selfcheck.NewRunner().Run(ctx, selfCheckInput)
		},
		NetworkStatusFunc: func(context.Context) server.NodeNetworkStatus {
			poolStats := portPool.Stats()
			sipSnapshot := sipMetrics.Snapshot()
			rtpSnapshot := rtpTransport.Snapshot()
			return server.NodeNetworkStatus{
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
	})
	if err != nil {
		log.Fatalf("initialize handler failed: %v", err)
	}
	if uiCfg.Enabled && uiCfg.Mode == "embedded" {
		handler, err = server.NewEmbeddedUIFallbackHandler(handler, server.EmbeddedUIOptions{BasePath: uiCfg.BasePath})
		if err != nil {
			log.Fatalf("initialize embedded ui handler failed: %v", err)
		}
	}

	report := selfcheck.NewRunner().Run(context.Background(), selfCheckInput)
	if raw, err := json.Marshal(report); err == nil {
		log.Printf("env_self_check_report=%s", raw)
	}
	log.Print(report.ToCLI())
	if report.Overall == selfcheck.LevelError {
		log.Fatal("environment self-check failed, please fix errors before startup")
	}

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
		log.Fatalf("start pprof server failed: %v", err)
	}

	go func() {
		log.Printf("gateway server listening on %s (version=%s commit=%s build_time=%s)", httpServer.Addr, version, commit, buildTime)
		uiURL := fmt.Sprintf("http://%s:%d%s", uiCfg.ListenIP, uiCfg.ListenPort, uiCfg.BasePath)
		if uiCfg.ListenIP == "0.0.0.0" {
			uiURL = fmt.Sprintf("http://%s:%d%s", "127.0.0.1", uiCfg.ListenPort, uiCfg.BasePath)
		}
		log.Printf("ui service mode=%s enabled=%t ui_url=%s", uiCfg.Mode, uiCfg.Enabled, uiURL)
		log.Printf("storage paths temp_dir=%q final_dir=%q audit_dir=%q log_dir=%q", paths.TempDir, paths.FinalDir, paths.AuditDir, paths.LogDir)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server failed: %v", err)
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

func buildSelfCheckInput(paths config.StoragePaths, cliConfigPath string, defaultPort string) (selfcheck.Input, config.UIConfig) {
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
			return in, runtimeCfg.UI
		}
		in.DownstreamRoutes = routeCfg.Routes
	}
	return in, runtimeCfg.UI
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
	if len(args) == 0 {
		return false, nil
	}
	switch strings.ToLower(strings.TrimSpace(args[0])) {
	case "init-config":
		cliPath, err := readCLIConfigPath(args[1:])
		if err != nil {
			return true, err
		}
		path := resolveConfigOutputPath(cliPath, os.Getenv("GATEWAY_CONFIG"), os.Getwd)
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
		b, err := defaultConfigYAML()
		if err != nil {
			return true, err
		}
		fmt.Print(string(b))
		return true, nil
	case "validate-config":
		fs := flag.NewFlagSet("validate-config", flag.ContinueOnError)
		cfgPath := fs.String("f", "", "config file path")
		fs.SetOutput(os.Stderr)
		if err := fs.Parse(args[1:]); err != nil {
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
	candidate, ok := pickExistingConfigCandidate(cliConfigPath, os.Getenv("GATEWAY_CONFIG"), os.Executable, os.Getwd, fileExists)
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
		return cfg, configLoadResult{Path: generatedPath, Source: configSourceDefaultGenerated, AutoGenerated: true, GeneratedAsMode: generatedMode}, nil
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
	return runtimeCfg, configLoadResult{Path: candidate.path, Source: candidate.source}, nil
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

func defaultConfigYAML() ([]byte, error) {
	networkCfg := config.DefaultNetworkConfig()
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

func resolveConfigOutputPath(cliConfigPath string, envConfigPath string, getwd func() (string, error)) string {
	if p := strings.TrimSpace(cliConfigPath); p != "" {
		return p
	}
	if p := strings.TrimSpace(envConfigPath); p != "" {
		return p
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
	b, err := defaultConfigYAML()
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
	path := resolveConfigOutputPath(cliConfigPath, envConfigPath, getwd)
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

func pickExistingConfigCandidate(cliConfigPath string, envConfigPath string, executablePath func() (string, error), getwd func() (string, error), exists func(string) bool) (configCandidate, bool) {
	exePath, err := executablePath()
	if err != nil {
		exePath = ""
	}
	cwd, err := getwd()
	if err != nil {
		cwd = "."
	}
	for _, candidate := range configCandidates(cliConfigPath, envConfigPath, executableDir(exePath), cwd) {
		if exists(candidate.path) {
			return candidate, true
		}
	}
	return configCandidate{}, false
}

func configCandidates(cliConfigPath string, envConfigPath string, exeDir string, cwd string) []configCandidate {
	candidates := make([]configCandidate, 0, 6)
	if p := strings.TrimSpace(cliConfigPath); p != "" {
		candidates = append(candidates, configCandidate{path: p, source: configSourceCLI})
	}
	if p := strings.TrimSpace(envConfigPath); p != "" {
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

func readPort() string {
	port := os.Getenv("GATEWAY_PORT")
	if port == "" {
		return "18080"
	}
	if _, err := strconv.Atoi(port); err != nil {
		log.Printf("invalid GATEWAY_PORT=%q, fallback to 18080", port)
		return "18080"
	}
	return port
}
