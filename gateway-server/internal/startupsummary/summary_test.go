package startupsummary

import (
	"fmt"
	"strings"
	"testing"
)

func TestSummaryToLogText(t *testing.T) {
	s := Summary{
		NodeID:            "gateway-a-01",
		ConfigPath:        "./configs/config.yaml",
		ConfigSource:      "cli",
		UIMode:            "embedded",
		UIURL:             "http://127.0.0.1:18080/",
		APIURL:            "http://127.0.0.1:18080/api",
		SIPListen:         ListenEndpoint{IP: "10.0.0.2", Port: 5060, Transport: "TCP"},
		RTPListen:         RTPListen{IP: "10.0.0.2", PortRange: "20000-20100", Transport: "UDP"},
		StorageDirs:       StorageDirs{TempDir: "./data/temp", FinalDir: "./data/final", AuditDir: "./data/audit", LogDir: "./data/logs"},
		BusinessExecution: BusinessExecutionStatus{State: "protocol_only", RouteCount: 0, Message: "协议层可启动，业务执行层未激活（未加载下游 HTTP 路由）", Impact: "仅完成 SIP/RTP 协议交互，不会执行 A 网 HTTP 落地"},
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
		"business_execution: state=protocol_only route_count=0 message=协议层可启动，业务执行层未激活（未加载下游 HTTP 路由） impact=仅完成 SIP/RTP 协议交互，不会执行 A 网 HTTP 落地",
		"self_check_summary: generated_at=2026-01-02T03:04:05Z overall=warn info=6 warn=1 error=0",
	} {
		if !strings.Contains(text, expected) {
			t.Fatalf("expected text contains %q\nfull:\n%s", expected, text)
		}
	}
}

func ExampleSummary_ToLogText() {
	s := Summary{
		NodeID:            "gateway-a-01",
		ConfigPath:        "./configs/config.yaml",
		ConfigSource:      "env",
		UIMode:            "external",
		UIURL:             "external",
		APIURL:            "http://127.0.0.1:18080/api",
		SIPListen:         ListenEndpoint{IP: "0.0.0.0", Port: 5060, Transport: "TCP"},
		RTPListen:         RTPListen{IP: "0.0.0.0", PortRange: "20000-20100", Transport: "UDP"},
		StorageDirs:       StorageDirs{TempDir: "./data/temp", FinalDir: "./data/final", AuditDir: "./data/audit", LogDir: "./data/logs"},
		BusinessExecution: BusinessExecutionStatus{State: "active", RouteCount: 2, Message: "业务执行层已激活，下游 HTTP 路由映射可用", Impact: "A 网 HTTP 落地可执行"},
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
	// - business_execution: state=active route_count=2 message=业务执行层已激活，下游 HTTP 路由映射可用 impact=A 网 HTTP 落地可执行
	// - self_check_summary: generated_at=2026-01-02T03:04:05Z overall=info info=7 warn=0 error=0
}
