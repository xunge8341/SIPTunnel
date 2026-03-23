package config

import (
	"os"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestDefaultNetworkConfig(t *testing.T) {
	cfg := DefaultNetworkConfig()
	if cfg.SIP.Transport != "TCP" {
		t.Fatalf("SIP transport default = %s, want TCP", cfg.SIP.Transport)
	}
	if cfg.RTP.Transport != "UDP" {
		t.Fatalf("RTP transport default = %s, want UDP", cfg.RTP.Transport)
	}
	if !cfg.SIP.TCPKeepAliveEnabled {
		t.Fatal("SIP tcp_keepalive_enabled default should be true")
	}
	if cfg.SIP.TCPKeepAliveIntervalMS <= 0 || cfg.SIP.TCPReadBufferBytes <= 0 || cfg.SIP.TCPWriteBufferBytes <= 0 || cfg.SIP.MaxConnections <= 0 {
		t.Fatal("SIP TCP lifecycle defaults should be positive")
	}
	if cfg.RTP.TCPReadTimeoutMS <= 0 || cfg.RTP.TCPWriteTimeoutMS <= 0 || cfg.RTP.MaxTCPSessions <= 0 {
		t.Fatal("RTP TCP lifecycle defaults should be positive")
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("default config should be valid: %v", err)
	}
}

func TestParseNetworkConfigYAML_InjectsDefaultsForMissingFields(t *testing.T) {
	raw := `
sip:
  enabled: true
rtp:
  enabled: true
  port_start: 25000
  port_end: 25099
`
	cfg, err := ParseNetworkConfigYAML([]byte(raw))
	if err != nil {
		t.Fatalf("ParseNetworkConfigYAML error: %v", err)
	}
	if cfg.SIP.ListenPort != 5060 {
		t.Fatalf("SIP listen port default = %d, want 5060", cfg.SIP.ListenPort)
	}
	if cfg.SIP.Transport != "TCP" {
		t.Fatalf("SIP transport default = %s, want TCP", cfg.SIP.Transport)
	}
	if cfg.RTP.Transport != "UDP" {
		t.Fatalf("RTP transport default = %s, want UDP", cfg.RTP.Transport)
	}
	if cfg.RTP.PortStart != 25000 || cfg.RTP.PortEnd != 25099 {
		t.Fatalf("RTP ports = [%d,%d], want [25000,25099]", cfg.RTP.PortStart, cfg.RTP.PortEnd)
	}
}

func TestParseNetworkConfigYAML_InvalidRanges(t *testing.T) {
	raw := `
sip:
  enabled: true
  listen_ip: 127.0.0.1
  listen_port: 70000
  transport: SCTP
rtp:
  enabled: true
  listen_ip: 127.0.0.1
  port_start: 22000
  port_end: 21000
  max_packet_bytes: -1
  tcp_read_timeout_ms: -1
  tcp_write_timeout_ms: -1
  max_tcp_sessions: -1
`
	_, err := ParseNetworkConfigYAML([]byte(raw))
	if err == nil {
		t.Fatal("expected validation error")
	}
	msg := err.Error()
	for _, keyword := range []string{"sip.listen_port", "sip.transport", "rtp.port_start", "rtp.max_packet_bytes", "rtp.tcp_read_timeout_ms", "rtp.tcp_write_timeout_ms", "rtp.max_tcp_sessions"} {
		if !strings.Contains(msg, keyword) {
			t.Fatalf("error %q should contain %q", msg, keyword)
		}
	}
}

func TestParseNetworkConfigYAML_PortConflict(t *testing.T) {
	raw := `
sip:
  enabled: true
  listen_ip: 0.0.0.0
  listen_port: 20010
  transport: UDP
rtp:
  enabled: true
  listen_ip: 127.0.0.1
  port_start: 20000
  port_end: 20100
  transport: UDP
`
	_, err := ParseNetworkConfigYAML([]byte(raw))
	if err == nil {
		t.Fatal("expected conflict error")
	}
	if !strings.Contains(err.Error(), "network port conflict") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestParseNetworkConfigYAML_RTPTransportTCPAccepted(t *testing.T) {
	raw := `
sip:
  enabled: true
  listen_ip: 127.0.0.1
  listen_port: 5060
  transport: TCP
rtp:
  enabled: true
  listen_ip: 127.0.0.1
  port_start: 23000
  port_end: 23099
  transport: tcp
`
	cfg, err := ParseNetworkConfigYAML([]byte(raw))
	if err != nil {
		t.Fatalf("ParseNetworkConfigYAML error: %v", err)
	}
	if cfg.RTP.Transport != "TCP" {
		t.Fatalf("rtp transport=%s, want TCP", cfg.RTP.Transport)
	}
}

func TestParseNetworkConfigYAML_ExplicitDisableRespected(t *testing.T) {
	raw := `
sip:
  enabled: false
rtp:
  enabled: false
`
	cfg, err := ParseNetworkConfigYAML([]byte(raw))
	if err != nil {
		t.Fatalf("ParseNetworkConfigYAML error: %v", err)
	}
	if cfg.SIP.Enabled {
		t.Fatal("expected sip.enabled=false")
	}
	if cfg.RTP.Enabled {
		t.Fatal("expected rtp.enabled=false")
	}
}

func TestConfigYAMLSample_NetworkSectionValid(t *testing.T) {
	data, err := os.ReadFile("../../configs/config.yaml")
	if err != nil {
		t.Fatalf("read config.yaml: %v", err)
	}
	var fileCfg struct {
		Network NetworkConfig `yaml:"network"`
	}
	if err := yaml.Unmarshal(data, &fileCfg); err != nil {
		t.Fatalf("unmarshal config.yaml: %v", err)
	}
	fileCfg.Network.ApplyDefaults()
	if err := fileCfg.Network.Validate(); err != nil {
		t.Fatalf("network section should be valid: %v", err)
	}
}

func TestSIPConfigUDPMessageSizeRisk(t *testing.T) {
	cfg := DefaultNetworkConfig().SIP
	cfg.Transport = "UDP"
	cfg.MaxMessageBytes = SIPUDPRecommendedMaxMessageBytes + 1
	if !cfg.UDPMessageSizeRisk() {
		t.Fatal("expected UDP message size risk")
	}
	cfg.MaxMessageBytes = SIPUDPRecommendedMaxMessageBytes
	if cfg.UDPMessageSizeRisk() {
		t.Fatal("expected no risk when equals recommended limit")
	}
}

func TestParseNetworkConfigYAML_DefaultModeInjected(t *testing.T) {
	raw := `
sip:
  enabled: true
rtp:
  enabled: true
`
	cfg, err := ParseNetworkConfigYAML([]byte(raw))
	if err != nil {
		t.Fatalf("ParseNetworkConfigYAML error: %v", err)
	}
	if cfg.Mode != DefaultNetworkMode() {
		t.Fatalf("mode=%s, want %s", cfg.Mode, DefaultNetworkMode())
	}
}

func TestParseNetworkConfigYAML_InvalidMode(t *testing.T) {
	raw := `
mode: INVALID_MODE
sip:
  enabled: true
rtp:
  enabled: true
`
	_, err := ParseNetworkConfigYAML([]byte(raw))
	if err == nil {
		t.Fatal("expected invalid network.mode error")
	}
	if !strings.Contains(err.Error(), "network.mode") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestTransportTuningDefaultsApplied(t *testing.T) {
	cfg, err := ParseNetworkConfigYAML([]byte(`
mode: SENDER_SIP__RECEIVER_RTP
sip:
  enabled: true
  listen_ip: 127.0.0.1
  listen_port: 5060
  transport: UDP
rtp:
  enabled: true
  listen_ip: 127.0.0.1
  port_start: 20000
  port_end: 20100
  transport: UDP
transport_tuning:
  udp_control_max_bytes: 1100
  boundary_fixed_window_bytes: 524288
`))
	if err != nil {
		t.Fatalf("ParseNetworkConfigYAML error: %v", err)
	}
	if got := cfg.TransportTuning.UDPControlMaxBytes; got != 1100 {
		t.Fatalf("udp_control_max_bytes=%d, want 1100", got)
	}
	if got := cfg.TransportTuning.BoundaryFixedWindowBytes; got != 524288 {
		t.Fatalf("boundary_fixed_window_bytes=%d, want 524288", got)
	}
	if got := cfg.TransportTuning.BoundarySegmentConcurrency; got <= 0 {
		t.Fatalf("BoundarySegmentConcurrency=%d, want default >0", got)
	}
}

func TestTransportTuningRTPGapDefaultsApplied(t *testing.T) {
	cfg, err := ParseNetworkConfigYAML([]byte(`
sip:
  enabled: true
rtp:
  enabled: true
transport_tuning:
  boundary_rtp_payload_bytes: 768
`))
	if err != nil {
		t.Fatalf("ParseNetworkConfigYAML error: %v", err)
	}
	defaults := DefaultTransportTuningConfig()
	if cfg.TransportTuning.BoundaryRTPReorderWindowPackets != defaults.BoundaryRTPReorderWindowPackets {
		t.Fatalf("boundary_rtp_reorder_window_packets=%d, want %d", cfg.TransportTuning.BoundaryRTPReorderWindowPackets, defaults.BoundaryRTPReorderWindowPackets)
	}
	if cfg.TransportTuning.BoundaryRTPLossTolerancePackets != defaults.BoundaryRTPLossTolerancePackets {
		t.Fatalf("boundary_rtp_loss_tolerance_packets=%d, want %d", cfg.TransportTuning.BoundaryRTPLossTolerancePackets, defaults.BoundaryRTPLossTolerancePackets)
	}
	if cfg.TransportTuning.BoundaryRTPGapTimeoutMS != defaults.BoundaryRTPGapTimeoutMS {
		t.Fatalf("boundary_rtp_gap_timeout_ms=%d, want %d", cfg.TransportTuning.BoundaryRTPGapTimeoutMS, defaults.BoundaryRTPGapTimeoutMS)
	}
	if cfg.TransportTuning.BoundaryPlaybackRTPReorderWindowPackets != defaults.BoundaryPlaybackRTPReorderWindowPackets {
		t.Fatalf("boundary_playback_rtp_reorder_window_packets=%d, want %d", cfg.TransportTuning.BoundaryPlaybackRTPReorderWindowPackets, defaults.BoundaryPlaybackRTPReorderWindowPackets)
	}
	if cfg.TransportTuning.BoundaryPlaybackRTPLossTolerancePackets != defaults.BoundaryPlaybackRTPLossTolerancePackets {
		t.Fatalf("boundary_playback_rtp_loss_tolerance_packets=%d, want %d", cfg.TransportTuning.BoundaryPlaybackRTPLossTolerancePackets, defaults.BoundaryPlaybackRTPLossTolerancePackets)
	}
	if cfg.TransportTuning.BoundaryPlaybackRTPGapTimeoutMS != defaults.BoundaryPlaybackRTPGapTimeoutMS {
		t.Fatalf("boundary_playback_rtp_gap_timeout_ms=%d, want %d", cfg.TransportTuning.BoundaryPlaybackRTPGapTimeoutMS, defaults.BoundaryPlaybackRTPGapTimeoutMS)
	}
}

func TestTransportTuningRejectsInvalidRTPGapSettings(t *testing.T) {
	cfg := DefaultTransportTuningConfig()
	cfg.BoundaryRTPReorderWindowPackets = 0
	cfg.BoundaryRTPLossTolerancePackets = -1
	cfg.BoundaryRTPGapTimeoutMS = 50
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected invalid RTP gap tuning error")
	}
}
