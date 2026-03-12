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
  port_end: 25050
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
	if cfg.RTP.PortStart != 25000 || cfg.RTP.PortEnd != 25050 {
		t.Fatalf("RTP ports = [%d,%d], want [25000,25050]", cfg.RTP.PortStart, cfg.RTP.PortEnd)
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
  port_end: 23020
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
