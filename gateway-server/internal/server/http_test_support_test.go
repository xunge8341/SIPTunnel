package server

import (
	"context"
	"net"
	"net/http"
	"siptunnel/internal/config"
	"siptunnel/internal/nodeconfig"
	"siptunnel/internal/observability"
	"siptunnel/internal/repository"
	filerepo "siptunnel/internal/repository/file"
	memrepo "siptunnel/internal/repository/memory"
	"siptunnel/internal/selfcheck"
	"siptunnel/internal/service"
	"siptunnel/internal/service/taskengine"
	"siptunnel/internal/startupsummary"
	"strconv"
	"testing"
	"time"
)

func buildTestHandler(t *testing.T) (http.Handler, repository.TaskRepository, observability.AuditStore) {
	t.Helper()
	repo := memrepo.NewTaskRepository()
	audit := observability.NewInMemoryAuditStore()
	nodeStore, err := filerepo.NewNodeConfigStore(t.TempDir() + "/node_config.json")
	if err != nil {
		t.Fatalf("new node config store failed: %v", err)
	}
	local := nodeStore.GetLocalNode()
	local.NodeID = "34020000002000000001"
	local.NodeName = "Gateway A"
	if _, err := nodeStore.UpdateLocalNode(local); err != nil {
		t.Fatalf("seed local node failed: %v", err)
	}
	if _, err := nodeStore.CreatePeer(nodeconfig.PeerNodeConfig{PeerNodeID: "34020000002000000002", PeerName: "Peer B", PeerSignalingIP: "10.20.0.20", PeerSignalingPort: 5060, PeerMediaIP: "10.20.0.20", PeerMediaPortStart: 32000, PeerMediaPortEnd: 32100, SupportedNetworkMode: config.NetworkModeSenderSIPReceiverRTP, Enabled: true}); err != nil {
		t.Fatalf("seed peer failed: %v", err)
	}
	mappingStore, err := filerepo.NewTunnelMappingStore(t.TempDir() + "/tunnel_mappings.json")
	if err != nil {
		t.Fatalf("new tunnel mapping store failed: %v", err)
	}
	_, _ = mappingStore.Create(TunnelMapping{MappingID: "asset.sync", Name: "asset.sync", Enabled: true, PeerNodeID: "34020000002000000002", LocalBindIP: "127.0.0.1", LocalBindPort: 21080, LocalBasePath: "/sync", RemoteTargetIP: "127.0.0.1", RemoteTargetPort: 8080, RemoteBasePath: "/sync", AllowedMethods: []string{"POST"}, ConnectTimeoutMS: 500, RequestTimeoutMS: 1000, ResponseTimeoutMS: 1000, MaxRequestBodyBytes: 1024, MaxResponseBodyBytes: 2048})
	deps := handlerDeps{
		logger:           observability.NewStructuredLogger(nil),
		audit:            audit,
		repo:             repo,
		engine:           taskengine.NewEngine(repo, service.RetryPolicy{MaxAttempts: 3, BaseBackoff: time.Second}),
		limits:           OpsLimits{RPS: 100, Burst: 200, MaxConcurrent: 50},
		routes:           map[string]OpsRoute{"asset.sync": {APICode: "asset.sync", HTTPMethod: "POST", HTTPPath: "/sync", Enabled: true}},
		mappings:         mappingStore,
		nodeStore:        nodeStore,
		nodeConfigSource: "file:/tmp/test/node_config.json",
		mappingSource:    "file:/tmp/test/tunnel_mappings.json",
		selfCheckProvider: func(_ context.Context) selfcheck.Report {
			return selfcheck.Report{Overall: selfcheck.LevelInfo, Summary: selfcheck.Summary{Info: 1, Warn: 1, Error: 1}, Items: []selfcheck.Item{{Name: "sample-info", Level: selfcheck.LevelInfo, Message: "ok", Suggestion: "none", ActionHint: "keep"}, {Name: "sample-warn", Level: selfcheck.LevelWarn, Message: "warn", Suggestion: "check", ActionHint: "verify"}, {Name: "sample-error", Level: selfcheck.LevelError, Message: "err", Suggestion: "fix", ActionHint: "recover"}}}
		},
		startupSummaryFn: func(_ context.Context) startupsummary.Summary {
			capability := config.DeriveCapability(config.NetworkModeSenderSIPReceiverRTP)
			return startupsummary.Summary{NodeID: "n1", NetworkMode: config.NetworkModeSenderSIPReceiverRTP, Capability: capability, CapabilitySummary: startupsummary.CapabilitySummary{Supported: capability.SupportedFeatures(), Unsupported: capability.UnsupportedFeatures(), Items: capability.Matrix()}, TransportPlan: config.ResolveTransportPlan(config.NetworkModeSenderSIPReceiverRTP), ConfigPath: "./configs/config.yaml", ConfigSource: "cli", UIMode: "embedded", UIURL: "http://127.0.0.1:18080/", APIURL: "http://127.0.0.1:18080/api"}
		},
		networkStatusFunc: func(_ context.Context) NodeNetworkStatus {
			capability := config.DeriveCapability(config.NetworkModeSenderSIPReceiverRTP)
			return NodeNetworkStatus{
				NetworkMode:         config.NetworkModeSenderSIPReceiverRTP,
				Capability:          capability,
				CapabilitySummary:   startupsummary.CapabilitySummary{Supported: capability.SupportedFeatures(), Unsupported: capability.UnsupportedFeatures(), Items: capability.Matrix()},
				TransportPlan:       config.ResolveTransportPlan(config.NetworkModeSenderSIPReceiverRTP),
				SIP:                 SIPNetworkStatus{ListenIP: "10.10.1.10", ListenPort: 5060, Transport: "TCP", CurrentSessions: 12, CurrentConnections: 7},
				RTP:                 RTPNetworkStatus{ListenIP: "10.10.1.10", PortStart: 30000, PortEnd: 30020, Transport: "UDP", ActiveTransfers: 3, UsedPorts: 6, AvailablePorts: 15, PortPoolTotal: 21, PortPoolUsed: 6, PortAllocFailTotal: 2},
				RecentBindErrors:    []string{"sip: bind 10.10.1.10:5061 failed"},
				RecentNetworkErrors: []string{"rtp: write timeout to 10.20.1.20:30001"},
			}
		},
	}
	deps.protection = defaultProtectionSettings()
	deps.protectionRuntime = newProtectionRuntime(deps.limits)
	deps.sessionMgr = newTunnelSessionManager(&fakeRegistrar{registerCodes: []int{200}}, deps.tunnelConfig)
	deps.sessionMgr.Start()
	t.Cleanup(func() { _ = deps.sessionMgr.Close() })
	return newMux(deps), repo, audit
}

func reserveFreePort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("allocate free port failed: %v", err)
	}
	defer ln.Close()
	return ln.Addr().(*net.TCPAddr).Port
}

func reserveFreeMappingPort(t *testing.T) int {
	t.Helper()
	for port := config.DefaultMappingPortStart; port <= config.DefaultMappingPortEnd; port++ {
		ln, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
		if err != nil {
			continue
		}
		_ = ln.Close()
		return port
	}
	t.Fatalf("allocate free mapping port in [%d,%d] failed", config.DefaultMappingPortStart, config.DefaultMappingPortEnd)
	return 0
}
