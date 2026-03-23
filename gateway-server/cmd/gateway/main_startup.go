package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"log/slog"

	"siptunnel/internal/config"
	"siptunnel/internal/diagnostics"
	"siptunnel/internal/netutil"
	"siptunnel/internal/observability"
	"siptunnel/internal/persistence"
	file "siptunnel/internal/repository/file"
	"siptunnel/internal/security"
	"siptunnel/internal/selfcheck"
	"siptunnel/internal/server"
	"siptunnel/internal/service/filetransfer"
	"siptunnel/internal/service/httpinvoke"
	"siptunnel/internal/service/sipcontrol"
	"siptunnel/internal/service/siptcp"
	"siptunnel/internal/startupsummary"
)

func runGatewayStartup(args []string) {
	observability.SetBuildInfo(version, commit, buildTime)
	server.SetBuildIdentity(version, commit, buildTime)
	log.SetPrefix(fmt.Sprintf("[build version=%s commit=%s build_time=%s] ", version, commit, buildTime))
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
	persistenceCfg := config.LoadPersistenceConfigFromEnv()
	if err := paths.EnsureWritable(); err != nil {
		log.Fatal(formatStartupFailure("startup directory validation", err, "", runtime.GOOS))
	}
	selfCheckInput, uiCfg, cfgLoad := buildSelfCheckInput(paths, cliConfigPath, defaultPort)
	nodeStoreForListen, nodeCfgErr := file.NewNodeConfigStore(filepath.Join(paths.FinalDir, "node_config.json"))
	if nodeCfgErr == nil {
		localNode := nodeStoreForListen.GetLocalNode()
		selfCheckInput.NetworkConfig.Mode = localNode.NetworkMode.Normalize()
		selfCheckInput.NetworkConfig.SIP.ListenIP = localNode.SIPListenIP
		selfCheckInput.NetworkConfig.SIP.ListenPort = localNode.SIPListenPort
		selfCheckInput.NetworkConfig.SIP.Transport = localNode.SIPTransport
		selfCheckInput.NetworkConfig.RTP.ListenIP = localNode.RTPListenIP
		selfCheckInput.NetworkConfig.RTP.PortStart = localNode.RTPPortStart
		selfCheckInput.NetworkConfig.RTP.PortEnd = localNode.RTPPortEnd
		selfCheckInput.NetworkConfig.RTP.Transport = localNode.RTPTransport
	}
	logEffectiveRuntimeNetworkState("post_node_store_override", selfCheckInput.NetworkConfig)
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
	var sipUDPCloser io.Closer
	sipVerifier := security.NewHMACSigner("siptunnel-boundary-secret")
	dispatcher := sipcontrol.NewDispatcher(sipVerifier, slog.Default(), nil)
	dispatcher.RegisterHandler(sipcontrol.NewCommandCreateHandler(nil))
	dispatcher.RegisterHandler(sipcontrol.NewFileCreateHandler(nil))
	dispatcher.RegisterHandler(sipcontrol.NewFileRetransmitRequestHandler(nil))
	dispatcher.RegisterHandler(sipcontrol.NewTaskCancelHandler(nil))
	_ = dispatcher
	if selfCheckInput.NetworkConfig.SIP.Enabled && selfCheckInput.NetworkConfig.SIP.Transport == "TCP" {
		dispatcher.SetTransport(sipcontrol.TransportTCP)
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
		}, siptcp.MessageHandlerFunc(func(ctx context.Context, meta siptcp.ConnectionMeta, payload []byte) ([]byte, error) {
			if !allowConfiguredPeer(meta.RemoteAddr, paths.FinalDir) {
				return nil, fmt.Errorf("remote peer is not allowed")
			}
			return server.RouteSignalPacket(ctx, meta.RemoteAddr, payload, func(ctx context.Context, payload []byte) ([]byte, error) {
				resp, err := dispatcher.Route(ctx, sipcontrol.InboundMessage{Body: payload})
				if err != nil {
					return nil, err
				}
				return resp.Body, nil
			})
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
	} else if selfCheckInput.NetworkConfig.SIP.Enabled && selfCheckInput.NetworkConfig.SIP.Transport == "UDP" {
		dispatcher.SetTransport(sipcontrol.TransportUDP)
		var err error
		addr := net.JoinHostPort(selfCheckInput.NetworkConfig.SIP.ListenIP, strconv.Itoa(selfCheckInput.NetworkConfig.SIP.ListenPort))
		if sipUDPCloser, err = startSIPUDPServer(context.Background(), addr, dispatcher, paths.FinalDir); err != nil {
			log.Fatal(formatStartupFailure("start sip udp server", err, addr, runtime.GOOS))
		}
		defer sipUDPCloser.Close()
	}
	log.Printf("network config loaded sip_transport=%s sip_listen=%s:%d rtp_transport=%s rtp_port_range=[%d,%d]", selfCheckInput.NetworkConfig.SIP.Transport, selfCheckInput.NetworkConfig.SIP.ListenIP, selfCheckInput.NetworkConfig.SIP.ListenPort, rtpTransport.Mode(), selfCheckInput.NetworkConfig.RTP.PortStart, selfCheckInput.NetworkConfig.RTP.PortEnd)
	effectiveSIPBudget := config.ResolveTransportPlanForConfig(selfCheckInput.NetworkConfig).RequestBodySizeLimit
	if selfCheckInput.NetworkConfig.SIP.UDPMessageSizeRisk() {
		log.Printf("sip udp message size risk detected transport=%s max_message_bytes=%d effective_udp_control_budget=%d recommended_max=%d", selfCheckInput.NetworkConfig.SIP.Transport, selfCheckInput.NetworkConfig.SIP.MaxMessageBytes, effectiveSIPBudget, config.SIPUDPRecommendedMaxMessageBytes)
	}
	nodeID := resolveNodeID()
	var unifiedSummary startupsummary.Summary
	handler, closer, err := server.NewHandlerWithOptions(server.HandlerOptions{
		LogDir: paths.LogDir,
		LogRetention: observability.LogRetentionPolicy{
			MaxSizeMB:  persistenceCfg.LogRetention.MaxSizeMB,
			MaxFiles:   persistenceCfg.LogRetention.MaxFiles,
			MaxAgeDays: persistenceCfg.LogRetention.MaxAgeDays,
		},
		AuditDir:         paths.AuditDir,
		DataDir:          paths.FinalDir,
		SQLitePath:       persistenceCfg.SQLitePath,
		UseMemoryBackend: strings.EqualFold(persistenceCfg.Backend, "memory"),
		Retention: persistence.RetentionPolicy{
			MaxTaskRecords:       persistenceCfg.Retention.MaxTaskRecords,
			MaxTaskAgeDays:       persistenceCfg.Retention.MaxTaskAgeDays,
			MaxAuditRecords:      persistenceCfg.Retention.MaxAuditRecords,
			MaxAuditAgeDays:      persistenceCfg.Retention.MaxAuditAgeDays,
			MaxDiagnosticRecords: persistenceCfg.Retention.MaxDiagnosticRecords,
			MaxDiagnosticAgeDays: persistenceCfg.Retention.MaxDiagnosticAgeDays,
		},
		CleanupInterval: persistenceCfg.CleanupInterval,
		RTPPortPool:     portPool,
		SelfCheckProvider: func(ctx context.Context) selfcheck.Report {
			runtimeSelfCheckInput := selfCheckInput
			runtimeSelfCheckInput.ExpectSIPPortOwnedByCurrentProcess = true
			return selfcheck.NewRunner().Run(ctx, runtimeSelfCheckInput)
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
		UIConfig: uiCfg,
	})
	if err != nil {
		log.Fatal(formatStartupFailure("initialize handler", err, "", runtime.GOOS))
	}
	dispatcher.RegisterHandler(server.NewSIPHTTPRelayHandler())
	if uiCfg.Enabled && uiCfg.Mode == "embedded" {
		handler, err = server.NewEmbeddedUIFallbackHandler(handler, server.EmbeddedUIOptions{BasePath: uiCfg.BasePath})
		if err != nil {
			log.Fatal(formatStartupFailure("initialize embedded ui handler", err, "", runtime.GOOS))
		}
	}

	selfCheckInput.ExpectSIPPortOwnedByCurrentProcess = true
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
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    1 << 20,
	}
	server.ApplyRuntimeHTTPMitigations("gateway-http", httpServer)
	pprofCfg := diagnostics.LoadPprofConfigFromEnv()
	pprofServer, err := diagnostics.StartPprofServer(pprofCfg, slog.Default())
	if err != nil {
		log.Fatal(formatStartupFailure("start pprof server", err, pprofCfg.ListenAddress, runtime.GOOS))
	}

	go func() {
		ln, err := net.Listen("tcp", httpServer.Addr)
		if err != nil {
			log.Fatal(formatStartupFailure("start gateway http listener", err, httpServer.Addr, runtime.GOOS))
		}
		ln = server.TuneTCPListener(ln)
		log.Printf("gateway server listening on %s (version=%s commit=%s build_time=%s)", httpServer.Addr, version, commit, buildTime)
		log.Print(unifiedSummary.ToLogText())
		if uiCfg.Mode == "external" {
			log.Printf("ui mode=external note=%q", "UI 由外部承载，请单独部署 gateway-ui 并将 API 指向 api_url")
		}
		if err := httpServer.Serve(ln); err != nil && err != http.ErrServerClosed {
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
	server.ApplyTransportTuning(runtimeCfg.Network.TransportTuning)
	log.Printf("network config resolved path=%q source=%s", cfgLoad.Path, cfgLoad.Source)
	log.Printf("transport tuning applied mode=%s udp_control_max_bytes=%d udp_catalog_max_bytes=%d inline_response_udp_budget_bytes=%d inline_response_safety_reserve_bytes=%d inline_response_envelope_overhead_bytes=%d inline_response_headroom_percent=%d udp_request_parallelism_per_device=%d udp_callback_parallelism_per_peer=%d udp_bulk_parallelism_per_device=%d boundary_rtp_payload_bytes=%d boundary_rtp_bitrate_bps=%d boundary_rtp_min_spacing_us=%d boundary_rtp_reorder_window_packets=%d boundary_rtp_loss_tolerance_packets=%d boundary_rtp_gap_timeout_ms=%d boundary_playback_rtp_reorder_window_packets=%d boundary_playback_rtp_loss_tolerance_packets=%d boundary_playback_rtp_gap_timeout_ms=%d boundary_fixed_window_bytes=%d boundary_fixed_window_threshold=%d boundary_http_window_bytes=%d boundary_http_window_threshold=%d boundary_segment_concurrency=%d boundary_http_segment_concurrency=%d standard_segment_concurrency=%d boundary_segment_retries=%d boundary_http_segment_retries=%d standard_segment_retries=%d boundary_response_start_wait_ms=%d boundary_range_response_start_wait_ms=%d boundary_resume_max_attempts=%d boundary_resume_per_range_retries=%d standard_window_bytes=%d standard_window_threshold=%d", runtimeCfg.Network.TransportTuning.Mode, runtimeCfg.Network.TransportTuning.UDPControlMaxBytes, runtimeCfg.Network.TransportTuning.UDPCatalogMaxBytes, runtimeCfg.Network.TransportTuning.InlineResponseUDPBudgetBytes, runtimeCfg.Network.TransportTuning.InlineResponseSafetyReserveBytes, runtimeCfg.Network.TransportTuning.InlineResponseEnvelopeOverheadBytes, runtimeCfg.Network.TransportTuning.InlineResponseHeadroomPercent, runtimeCfg.Network.TransportTuning.UDPRequestParallelismPerDevice, runtimeCfg.Network.TransportTuning.UDPCallbackParallelismPerPeer, runtimeCfg.Network.TransportTuning.UDPBulkParallelismPerDevice, runtimeCfg.Network.TransportTuning.BoundaryRTPPayloadBytes, runtimeCfg.Network.TransportTuning.BoundaryRTPBitrateBps, runtimeCfg.Network.TransportTuning.BoundaryRTPMinSpacingUS, runtimeCfg.Network.TransportTuning.BoundaryRTPReorderWindowPackets, runtimeCfg.Network.TransportTuning.BoundaryRTPLossTolerancePackets, runtimeCfg.Network.TransportTuning.BoundaryRTPGapTimeoutMS, runtimeCfg.Network.TransportTuning.BoundaryPlaybackRTPReorderWindowPackets, runtimeCfg.Network.TransportTuning.BoundaryPlaybackRTPLossTolerancePackets, runtimeCfg.Network.TransportTuning.BoundaryPlaybackRTPGapTimeoutMS, runtimeCfg.Network.TransportTuning.BoundaryFixedWindowBytes, runtimeCfg.Network.TransportTuning.BoundaryFixedWindowThreshold, runtimeCfg.Network.TransportTuning.BoundaryHTTPWindowBytes, runtimeCfg.Network.TransportTuning.BoundaryHTTPWindowThreshold, runtimeCfg.Network.TransportTuning.BoundarySegmentConcurrency, runtimeCfg.Network.TransportTuning.BoundaryHTTPSegmentConcurrency, runtimeCfg.Network.TransportTuning.StandardSegmentConcurrency, runtimeCfg.Network.TransportTuning.BoundarySegmentRetries, runtimeCfg.Network.TransportTuning.BoundaryHTTPSegmentRetries, runtimeCfg.Network.TransportTuning.StandardSegmentRetries, runtimeCfg.Network.TransportTuning.BoundaryResponseStartWaitMS, runtimeCfg.Network.TransportTuning.BoundaryRangeResponseWaitMS, runtimeCfg.Network.TransportTuning.BoundaryResumeMaxAttempts, runtimeCfg.Network.TransportTuning.BoundaryResumePerRangeRetries, runtimeCfg.Network.TransportTuning.StandardWindowBytes, runtimeCfg.Network.TransportTuning.StandardWindowThreshold)
	logAppliedTransportTuning(runtimeCfg.Network)
	if cfgLoad.AutoGenerated {
		log.Printf("startup config bootstrap auto_generated=true mode=%s config_path=%q next_step=%q", cfgLoad.GeneratedAsMode, cfgLoad.Path, "review generated config and adjust sip/rtp/storage/ops before production rollout")
	}

	in := selfcheck.Input{
		NetworkConfig:   runtimeCfg.Network,
		StoragePaths:    paths,
		RunMode:         resolveRunMode(),
		SuggestFreePort: parseBoolEnv("GATEWAY_SELFCHECK_SUGGEST_FREE_PORT"),
	}
	// GATEWAY_HTTPINVOKE_CONFIG 仅用于加载历史 route/api_code 配置（兼容/迁移路径，deprecated 术语）。
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

func logEffectiveRuntimeNetworkState(stage string, cfg config.NetworkConfig) {
	stage = strings.TrimSpace(stage)
	if stage == "" {
		stage = "effective"
	}
	log.Printf("startup runtime_network_effective stage=%s network_mode=%s sip_transport=%s sip_listen=%s:%d rtp_transport=%s rtp_port_range=[%d,%d] response_mode_policy=%s effective_inline_budget_bytes=%d", stage, cfg.Mode.Normalize(), strings.ToUpper(strings.TrimSpace(cfg.SIP.Transport)), strings.TrimSpace(cfg.SIP.ListenIP), cfg.SIP.ListenPort, strings.ToUpper(strings.TrimSpace(cfg.RTP.Transport)), cfg.RTP.PortStart, cfg.RTP.PortEnd, config.EffectiveResponseModePolicyLabel(cfg), config.EffectiveInlineResponseBodyBudgetBytes(cfg))
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
	// 业务执行层主线语义：HTTP 映射隧道；route/template 仅兼容存在。
	businessMessage := "业务执行层已激活，下游 HTTP 隧道映射可用"
	businessImpact := "A 网 HTTP 落地可执行"
	if routeCount <= 0 {
		businessState = "protocol_only"
		businessMessage = "协议层可启动，业务执行层未激活（未加载下游 HTTP 隧道映射）"
		businessImpact = "仅完成 SIP/RTP 协议交互，不会执行 A 网 HTTP 落地"
	}

	mode := networkCfg.Mode.Normalize()
	capability := config.DeriveCapability(mode)
	transportPlan := config.ResolveTransportPlanForConfig(networkCfg)
	transportTuningSummary := effectiveTransportTuningSummary(networkCfg)
	strategySnapshot := config.BuildEffectiveStrategySnapshot(networkCfg)
	uiDelivery := server.ReadEmbeddedUIDeliveryMetadata()

	return startupsummary.Summary{
		NodeID:      nodeID,
		NetworkMode: mode,
		Capability:  capability,
		CapabilitySummary: startupsummary.CapabilitySummary{
			Supported:   capability.SupportedFeatures(),
			Unsupported: capability.UnsupportedFeatures(),
			Items:       capability.Matrix(),
		},
		TransportPlan:          transportPlan,
		TransportTuningSummary: transportTuningSummary,
		ActiveStrategySummary: startupsummary.ActiveStrategySummary{
			ResponseModePolicy:            strategySnapshot.ResponseModePolicy,
			LargeResponseDeliveryFamily:   strategySnapshot.LargeResponseDeliveryFamily,
			SegmentedProfileSelector:      strategySnapshot.SegmentedProfileSelector,
			EntrySelectionPolicy:          strategySnapshot.EntrySelectionPolicy,
			UDPControlHeaderPolicy:        strategySnapshot.UDPControlHeaderPolicy,
			BoundaryRTPSendProfile:        strategySnapshot.BoundaryRTPSendProfile,
			BoundaryRTPToleranceProfile:   strategySnapshot.BoundaryRTPToleranceProfile,
			PlaybackRTPToleranceProfile:   strategySnapshot.PlaybackRTPToleranceProfile,
			GenericDownloadRTPSendProfile: strategySnapshot.GenericDownloadRTPSendProfile,
			GenericDownloadRTPTolerance:   strategySnapshot.GenericDownloadRTPTolerance,
			GenericDownloadCircuitPolicy:  strategySnapshot.GenericDownloadCircuitPolicy,
			GenericDownloadGuardPolicy:    strategySnapshot.GenericDownloadGuardPolicy,
		},
		UIDeliverySummary: startupsummary.UIDeliverySummary{
			MetadataPresent:         uiDelivery.MetadataPresent,
			ConsistencyStatus:       uiDelivery.ConsistencyStatus,
			ConsistencyDetail:       uiDelivery.ConsistencyDetail,
			BuildNonce:              uiDelivery.BuildNonce,
			EmbeddedAt:              uiDelivery.EmbeddedAt,
			UISourceLatestWrite:     uiDelivery.UISourceLatestWrite,
			EmbeddedHashSHA256:      uiDelivery.EmbeddedHashSHA256,
			AssetBaseMode:           uiDelivery.AssetBaseMode,
			RouterBasePathPolicy:    uiDelivery.RouterBasePathPolicy,
			DeliveryGuardStatus:     uiDelivery.DeliveryGuardStatus,
			DeliveryGuardDetail:     uiDelivery.DeliveryGuardDetail,
			DeliveryGuardRemoved:    uiDelivery.DeliveryGuardRemoved,
			DeliveryGuardRemaining:  uiDelivery.DeliveryGuardRemaining,
			DeliveryGuardActiveHits: uiDelivery.DeliveryGuardActiveHits,
		},
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
		DataSources: startupsummary.DataSources{
			NodeConfig: fmt.Sprintf("file:%s/node_config.json", filepath.Clean(storagePaths.FinalDir)),
			Peers:      fmt.Sprintf("file:%s/node_config.json", filepath.Clean(storagePaths.FinalDir)),
			Mappings:   fmt.Sprintf("file:%s/tunnel_mappings.json", filepath.Clean(storagePaths.FinalDir)),
			Mode:       fmt.Sprintf("runtime_config:%s", cfgLoad.Path),
			Capability: "derived_from_network_mode",
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
	if netutil.IsAddrInUseError(err) {
		b.WriteString(" | 端口冲突排查(PowerShell): Get-NetTCPConnection -LocalPort <端口> | Select-Object -First 5 -Property LocalAddress,LocalPort,State,OwningProcess; Get-Process -Id <PID> | Format-Table Id,ProcessName,Path")
		b.WriteString(" | 端口冲突排查(CMD): netstat -ano | findstr :<端口> && tasklist /FI \"PID eq <PID>\"")
		if strings.TrimSpace(listenAddr) != "" {
			b.WriteString(fmt.Sprintf(" | 当前监听地址=%s", listenAddr))
		}
	}
	return b.String()
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
