package selfcheck

import (
	"context"
	"errors"
	"net"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"siptunnel/internal/config"
	"siptunnel/internal/service/httpinvoke"
)

func TestRunnerRun_AllPass(t *testing.T) {
	base := t.TempDir()
	opened := 0
	runner := &Runner{
		nowFn: func() time.Time { return time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC) },
		interfaceIPs: func() (map[string]struct{}, error) {
			return map[string]struct{}{"127.0.0.1": {}}, nil
		},
		listenTCP: func(_, _ string) (net.Listener, error) {
			opened++
			return fakeListener{}, nil
		},
		listenUDP: func(_, _ string) (net.PacketConn, error) {
			opened++
			return fakePacketConn{}, nil
		},
		ensureWritable: func(path string) error {
			if strings.TrimSpace(path) == "" {
				return errors.New("empty")
			}
			return nil
		},
		dialTCP: func(_ context.Context, address string) error {
			if address != "127.0.0.1:19001" {
				t.Fatalf("unexpected address: %s", address)
			}
			return nil
		},
	}

	cfg := config.DefaultNetworkConfig()
	cfg.SIP.ListenIP = "127.0.0.1"
	cfg.SIP.Transport = "TCP"
	cfg.RTP.ListenIP = "127.0.0.1"
	cfg.RTP.PortStart = 22000
	cfg.RTP.PortEnd = 22010

	report := runner.Run(t.Context(), Input{
		NetworkConfig: cfg,
		StoragePaths: config.StoragePaths{
			TempDir:  filepath.Join(base, "temp"),
			FinalDir: filepath.Join(base, "final"),
			AuditDir: filepath.Join(base, "audit"),
		},
		DownstreamRoutes: []httpinvoke.RouteConfig{{TargetHost: "127.0.0.1", TargetPort: 19001}},
	})

	if report.Overall != LevelInfo {
		t.Fatalf("overall=%s, want info", report.Overall)
	}
	if report.Summary.Error != 0 || report.Summary.Warn != 0 {
		t.Fatalf("unexpected summary: %+v", report.Summary)
	}
	foundUDPSize := false
	for _, item := range report.Items {
		if item.Name == "sip.udp_message_size_risk" && item.Level == LevelInfo {
			foundUDPSize = true
		}
	}
	if !foundUDPSize {
		t.Fatalf("expected sip.udp_message_size_risk info item, items=%+v", report.Items)
	}
	if opened != 1 {
		t.Fatalf("expected sip bind check called once, got %d", opened)
	}
	if !strings.Contains(report.ToCLI(), "overall=info") {
		t.Fatalf("unexpected cli output: %s", report.ToCLI())
	}
}

func TestRunnerRun_ErrorsAndWarns(t *testing.T) {
	runner := &Runner{
		nowFn: func() time.Time { return time.Now().UTC() },
		interfaceIPs: func() (map[string]struct{}, error) {
			return map[string]struct{}{"10.0.0.8": {}}, nil
		},
		listenTCP: func(_, _ string) (net.Listener, error) {
			return nil, errors.New("address already in use")
		},
		listenUDP: func(_, _ string) (net.PacketConn, error) { return fakePacketConn{}, nil },
		ensureWritable: func(path string) error {
			if strings.Contains(path, "final") {
				return errors.New("permission denied")
			}
			return nil
		},
		dialTCP: func(_ context.Context, _ string) error { return errors.New("i/o timeout") },
	}

	cfg := config.DefaultNetworkConfig()
	cfg.SIP.ListenIP = "10.0.0.9"
	cfg.SIP.ListenPort = 20005
	cfg.SIP.Transport = "UDP"
	cfg.RTP.ListenIP = "0.0.0.0"
	cfg.RTP.Transport = "UDP"
	cfg.RTP.PortStart = 20000
	cfg.RTP.PortEnd = 100

	report := runner.Run(t.Context(), Input{
		NetworkConfig:    cfg,
		StoragePaths:     config.StoragePaths{TempDir: "./temp", FinalDir: "./final", AuditDir: "./audit"},
		DownstreamRoutes: []httpinvoke.RouteConfig{{TargetHost: "127.0.0.1", TargetPort: 18080}},
	})

	if report.Overall != LevelError {
		t.Fatalf("overall=%s, want error", report.Overall)
	}
	if report.Summary.Error == 0 {
		t.Fatalf("expected errors in summary: %+v", report.Summary)
	}
	if report.Summary.Warn == 0 {
		t.Fatalf("expected warns in summary: %+v", report.Summary)
	}
}

func TestDownstreamReachability_NoRoutesWarn(t *testing.T) {
	r := NewRunner()
	report := r.Run(t.Context(), Input{NetworkConfig: config.NetworkConfig{}})
	foundWarn := false
	for _, item := range report.Items {
		if item.Name == "downstream.http_base_reachability" && item.Level == LevelWarn {
			foundWarn = true
		}
	}
	if !foundWarn {
		t.Fatalf("expected downstream warn, items=%+v", report.Items)
	}
}

type fakeListener struct{}

func (fakeListener) Accept() (net.Conn, error) { return nil, errors.New("not implemented") }
func (fakeListener) Close() error              { return nil }
func (fakeListener) Addr() net.Addr            { return &net.TCPAddr{} }

type fakePacketConn struct{}

func (fakePacketConn) ReadFrom([]byte) (int, net.Addr, error) {
	return 0, nil, errors.New("not implemented")
}
func (fakePacketConn) WriteTo([]byte, net.Addr) (int, error) { return 0, nil }
func (fakePacketConn) Close() error                          { return nil }
func (fakePacketConn) LocalAddr() net.Addr                   { return &net.UDPAddr{} }
func (fakePacketConn) SetDeadline(time.Time) error           { return nil }
func (fakePacketConn) SetReadDeadline(time.Time) error       { return nil }
func (fakePacketConn) SetWriteDeadline(time.Time) error      { return nil }

func TestRunnerRun_SIPUDPMessageSizeRiskWarn(t *testing.T) {
	runner := &Runner{
		nowFn: func() time.Time { return time.Now().UTC() },
		interfaceIPs: func() (map[string]struct{}, error) {
			return map[string]struct{}{"127.0.0.1": {}}, nil
		},
		listenTCP:      func(_, _ string) (net.Listener, error) { return fakeListener{}, nil },
		listenUDP:      func(_, _ string) (net.PacketConn, error) { return fakePacketConn{}, nil },
		ensureWritable: func(_ string) error { return nil },
		dialTCP:        func(_ context.Context, _ string) error { return nil },
	}
	cfg := config.DefaultNetworkConfig()
	cfg.SIP.ListenIP = "127.0.0.1"
	cfg.SIP.Transport = "UDP"
	cfg.SIP.MaxMessageBytes = config.SIPUDPRecommendedMaxMessageBytes + 100
	cfg.RTP.Enabled = false

	report := runner.Run(t.Context(), Input{
		NetworkConfig: cfg,
		StoragePaths:  config.StoragePaths{TempDir: "./tmp", FinalDir: "./final", AuditDir: "./audit"},
	})

	for _, item := range report.Items {
		if item.Name == "sip.udp_message_size_risk" {
			if item.Level != LevelWarn {
				t.Fatalf("unexpected level=%s", item.Level)
			}
			return
		}
	}
	t.Fatalf("risk item not found: %+v", report.Items)
}
