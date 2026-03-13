package startupsummary

import (
	"fmt"
	"strings"
	"testing"
)

func TestSummaryToLogText(t *testing.T) {
	s := Summary{
		NodeID:       "gateway-a-01",
		ConfigPath:   "./configs/config.yaml",
		ConfigSource: "cli",
		UIMode:       "embedded",
		UIURL:        "http://127.0.0.1:18080/",
		APIURL:       "http://127.0.0.1:18080/api",
		SIPListen:    ListenEndpoint{IP: "10.0.0.2", Port: 5060, Transport: "TCP"},
		RTPListen:    RTPListen{IP: "10.0.0.2", PortRange: "20000-20100", Transport: "UDP"},
		StorageDirs:  StorageDirs{TempDir: "./data/temp", FinalDir: "./data/final", AuditDir: "./data/audit", LogDir: "./data/logs"},
		SelfCheckSummary: SelfCheckSummary{
			GeneratedAt: "2026-01-02T03:04:05Z",
			Overall:     "warn",
			Info:        6,
			Warn:        1,
			Error:       0,
		},
	}
	text := s.ToLogText()
	for _, expected := range []string{
		"startup summary:",
		"node_id: gateway-a-01",
		"config: path=./configs/config.yaml source=cli",
		"ui: mode=embedded url=http://127.0.0.1:18080/",
		"api_url: http://127.0.0.1:18080/api",
		"sip_listen: ip=10.0.0.2 port=5060 transport=TCP",
		"rtp_listen: ip=10.0.0.2 port_range=20000-20100 transport=UDP",
		"storage_dirs: temp=./data/temp final=./data/final audit=./data/audit log=./data/logs",
		"self_check_summary: generated_at=2026-01-02T03:04:05Z overall=warn info=6 warn=1 error=0",
	} {
		if !strings.Contains(text, expected) {
			t.Fatalf("expected text contains %q\nfull:\n%s", expected, text)
		}
	}
}

func ExampleSummary_ToLogText() {
	s := Summary{
		NodeID:       "gateway-a-01",
		ConfigPath:   "./configs/config.yaml",
		ConfigSource: "env",
		UIMode:       "external",
		UIURL:        "external",
		APIURL:       "http://127.0.0.1:18080/api",
		SIPListen:    ListenEndpoint{IP: "0.0.0.0", Port: 5060, Transport: "TCP"},
		RTPListen:    RTPListen{IP: "0.0.0.0", PortRange: "20000-20100", Transport: "UDP"},
		StorageDirs:  StorageDirs{TempDir: "./data/temp", FinalDir: "./data/final", AuditDir: "./data/audit", LogDir: "./data/logs"},
		SelfCheckSummary: SelfCheckSummary{
			GeneratedAt: "2026-01-02T03:04:05Z",
			Overall:     "info",
			Info:        7,
			Warn:        0,
			Error:       0,
		},
	}
	fmt.Println(s.ToLogText())
	// Output:
	// startup summary:
	// - node_id: gateway-a-01
	// - config: path=./configs/config.yaml source=env
	// - ui: mode=external url=external
	// - api_url: http://127.0.0.1:18080/api
	// - sip_listen: ip=0.0.0.0 port=5060 transport=TCP
	// - rtp_listen: ip=0.0.0.0 port_range=20000-20100 transport=UDP
	// - storage_dirs: temp=./data/temp final=./data/final audit=./data/audit log=./data/logs
	// - self_check_summary: generated_at=2026-01-02T03:04:05Z overall=info info=7 warn=0 error=0
}
