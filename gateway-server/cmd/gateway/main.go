package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"log/slog"

	"gopkg.in/yaml.v3"

	"siptunnel/internal/config"
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
	port := readPort()
	paths := config.LoadStoragePathsFromEnv()
	if err := paths.EnsureWritable(); err != nil {
		log.Fatalf("startup directory validation failed: %v", err)
	}
	selfCheckInput := buildSelfCheckInput(paths)
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
					ListenIP:           selfCheckInput.NetworkConfig.RTP.ListenIP,
					PortStart:          selfCheckInput.NetworkConfig.RTP.PortStart,
					PortEnd:            selfCheckInput.NetworkConfig.RTP.PortEnd,
					Transport:          rtpTransport.Mode(),
					ActiveTransfers:    poolStats.Used,
					UsedPorts:          poolStats.Used,
					AvailablePorts:     poolStats.Available,
					PortPoolTotal:      poolStats.Total,
					PortPoolUsed:       poolStats.Used,
					PortAllocFailTotal: poolStats.AllocFailTotal,
				},
				RecentBindErrors:    []string{},
				RecentNetworkErrors: []string{},
			}
		},
	})
	if err != nil {
		log.Fatalf("initialize handler failed: %v", err)
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
		Addr:              ":" + port,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		log.Printf("gateway server listening on %s (version=%s commit=%s build_time=%s)", httpServer.Addr, version, commit, buildTime)
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
	if err := httpServer.Shutdown(ctx); err != nil {
		log.Printf("graceful shutdown failed: %v", err)
	}
}

func buildSelfCheckInput(paths config.StoragePaths) selfcheck.Input {
	in := selfcheck.Input{
		NetworkConfig: loadNetworkConfig(),
		StoragePaths:  paths,
	}
	if routePath := os.Getenv("GATEWAY_HTTPINVOKE_CONFIG"); routePath != "" {
		routeCfg, err := httpinvoke.LoadConfig(routePath)
		if err != nil {
			log.Printf("load GATEWAY_HTTPINVOKE_CONFIG=%q failed: %v", routePath, err)
			return in
		}
		in.DownstreamRoutes = routeCfg.Routes
	}
	return in
}

func loadNetworkConfig() config.NetworkConfig {
	path := os.Getenv("GATEWAY_NETWORK_CONFIG")
	if path == "" {
		path = "configs/config.yaml"
	}
	data, err := os.ReadFile(path)
	if err != nil {
		log.Printf("load network config path=%q failed: %v, fallback to defaults", path, err)
		return config.DefaultNetworkConfig()
	}
	var fileCfg struct {
		Network config.NetworkConfig `yaml:"network"`
	}
	if err := yaml.Unmarshal(data, &fileCfg); err != nil {
		log.Printf("parse network config path=%q failed: %v, fallback to defaults", path, err)
		return config.DefaultNetworkConfig()
	}
	fileCfg.Network.ApplyDefaults()
	if err := fileCfg.Network.Validate(); err != nil {
		log.Printf("validate network config path=%q failed: %v, fallback to defaults", path, err)
		return config.DefaultNetworkConfig()
	}
	return fileCfg.Network
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
