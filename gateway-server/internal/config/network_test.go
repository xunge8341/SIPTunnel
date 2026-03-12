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
`
	_, err := ParseNetworkConfigYAML([]byte(raw))
	if err == nil {
		t.Fatal("expected validation error")
	}
	msg := err.Error()
	for _, keyword := range []string{"sip.listen_port", "sip.transport", "rtp.port_start", "rtp.max_packet_bytes"} {
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
