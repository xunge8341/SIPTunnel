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

	"gopkg.in/yaml.v3"

	"siptunnel/internal/config"
	"siptunnel/internal/selfcheck"
	"siptunnel/internal/server"
	"siptunnel/internal/service/httpinvoke"
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
	log.Printf("network config loaded sip_transport=%s sip_listen=%s:%d rtp_transport=%s rtp_port_range=[%d,%d]", selfCheckInput.NetworkConfig.SIP.Transport, selfCheckInput.NetworkConfig.SIP.ListenIP, selfCheckInput.NetworkConfig.SIP.ListenPort, selfCheckInput.NetworkConfig.RTP.Transport, selfCheckInput.NetworkConfig.RTP.PortStart, selfCheckInput.NetworkConfig.RTP.PortEnd)
	if selfCheckInput.NetworkConfig.SIP.UDPMessageSizeRisk() {
		log.Printf("sip udp message size risk detected transport=%s max_message_bytes=%d recommended_max=%d", selfCheckInput.NetworkConfig.SIP.Transport, selfCheckInput.NetworkConfig.SIP.MaxMessageBytes, config.SIPUDPRecommendedMaxMessageBytes)
	}
	handler, closer, err := server.NewHandlerWithOptions(server.HandlerOptions{
		LogDir:   paths.LogDir,
		AuditDir: paths.AuditDir,
		SelfCheckProvider: func(ctx context.Context) selfcheck.Report {
			return selfcheck.NewRunner().Run(ctx, selfCheckInput)
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
