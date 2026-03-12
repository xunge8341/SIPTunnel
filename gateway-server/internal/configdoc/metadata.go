package configdoc

import (
	"fmt"
	"strconv"
	"strings"
)

type RiskLevel string

const (
	RiskLow    RiskLevel = "low"
	RiskMedium RiskLevel = "medium"
	RiskHigh   RiskLevel = "high"
)

type ParamMeta struct {
	Name        string
	Type        string
	Default     any
	HotReload   bool
	Risk        RiskLevel
	Description string
	NetworkKey  bool
}

type Profile string

const (
	ProfileExample Profile = "example"
	ProfileDev     Profile = "dev"
	ProfileTest    Profile = "test"
	ProfileProd    Profile = "prod"
)

func Catalog() []ParamMeta {
	return []ParamMeta{
		{Name: "server.port", Type: "int", Default: 18080, HotReload: false, Risk: RiskMedium, Description: "网关 HTTP 管理端口。", NetworkKey: true},
		{Name: "storage.temp_dir", Type: "string", Default: "./data/temp", HotReload: false, Risk: RiskLow, Description: "文件分片临时目录。"},
		{Name: "storage.final_dir", Type: "string", Default: "./data/final", HotReload: false, Risk: RiskLow, Description: "文件组装完成目录。"},
		{Name: "storage.audit_dir", Type: "string", Default: "./data/audit", HotReload: false, Risk: RiskLow, Description: "审计日志落盘目录。"},
		{Name: "storage.log_dir", Type: "string", Default: "./data/logs", HotReload: false, Risk: RiskLow, Description: "运行日志目录。"},
		{Name: "network.sip.enabled", Type: "bool", Default: true, HotReload: true, Risk: RiskMedium, Description: "启用 SIP 控制面。", NetworkKey: true},
		{Name: "network.sip.listen_ip", Type: "string", Default: "0.0.0.0", HotReload: false, Risk: RiskHigh, Description: "SIP 监听 IP。", NetworkKey: true},
		{Name: "network.sip.listen_port", Type: "int", Default: 5060, HotReload: false, Risk: RiskHigh, Description: "SIP 监听端口。", NetworkKey: true},
		{Name: "network.sip.transport", Type: "string", Default: "TCP", HotReload: false, Risk: RiskHigh, Description: "SIP 传输层协议（TCP/UDP/TLS）。", NetworkKey: true},
		{Name: "network.sip.advertise_ip", Type: "string", Default: "", HotReload: false, Risk: RiskMedium, Description: "SIP 对端可见地址。", NetworkKey: true},
		{Name: "network.sip.domain", Type: "string", Default: "", HotReload: true, Risk: RiskLow, Description: "SIP 域名。"},
		{Name: "network.sip.max_message_bytes", Type: "int", Default: 65535, HotReload: true, Risk: RiskHigh, Description: "SIP 最大报文大小（UDP 超 1300 存在分片风险）。", NetworkKey: true},
		{Name: "network.sip.read_timeout_ms", Type: "int", Default: 5000, HotReload: true, Risk: RiskMedium, Description: "SIP 读超时（毫秒）。", NetworkKey: true},
		{Name: "network.sip.write_timeout_ms", Type: "int", Default: 5000, HotReload: true, Risk: RiskMedium, Description: "SIP 写超时（毫秒）。", NetworkKey: true},
		{Name: "network.sip.idle_timeout_ms", Type: "int", Default: 60000, HotReload: true, Risk: RiskLow, Description: "SIP 空闲连接超时（毫秒）。", NetworkKey: true},
		{Name: "network.rtp.enabled", Type: "bool", Default: true, HotReload: true, Risk: RiskMedium, Description: "启用 RTP 文件面。", NetworkKey: true},
		{Name: "network.rtp.listen_ip", Type: "string", Default: "0.0.0.0", HotReload: false, Risk: RiskHigh, Description: "RTP 监听 IP。", NetworkKey: true},
		{Name: "network.rtp.advertise_ip", Type: "string", Default: "", HotReload: false, Risk: RiskMedium, Description: "RTP 对端可见地址。", NetworkKey: true},
		{Name: "network.rtp.port_start", Type: "int", Default: 20000, HotReload: false, Risk: RiskHigh, Description: "RTP 端口池起始端口。", NetworkKey: true},
		{Name: "network.rtp.port_end", Type: "int", Default: 20100, HotReload: false, Risk: RiskHigh, Description: "RTP 端口池结束端口。", NetworkKey: true},
		{Name: "network.rtp.transport", Type: "string", Default: "UDP", HotReload: false, Risk: RiskHigh, Description: "RTP 传输协议（当前仅 UDP 正式上线）。", NetworkKey: true},
		{Name: "network.rtp.max_packet_bytes", Type: "int", Default: 1400, HotReload: true, Risk: RiskHigh, Description: "RTP 单包大小。", NetworkKey: true},
		{Name: "network.rtp.max_inflight_transfers", Type: "int", Default: 64, HotReload: true, Risk: RiskMedium, Description: "并发传输上限。"},
		{Name: "network.rtp.receive_buffer_bytes", Type: "int", Default: 4194304, HotReload: true, Risk: RiskMedium, Description: "RTP 接收缓冲区大小。", NetworkKey: true},
		{Name: "network.rtp.transfer_timeout_ms", Type: "int", Default: 30000, HotReload: true, Risk: RiskMedium, Description: "文件传输超时（毫秒）。", NetworkKey: true},
		{Name: "network.rtp.retransmit_max_rounds", Type: "int", Default: 3, HotReload: true, Risk: RiskLow, Description: "重传最大轮次。"},
		{Name: "media.port_range.start", Type: "int", Default: 20000, HotReload: false, Risk: RiskMedium, Description: "部署规划媒体端口起始值。", NetworkKey: true},
		{Name: "media.port_range.end", Type: "int", Default: 20100, HotReload: false, Risk: RiskMedium, Description: "部署规划媒体端口结束值。", NetworkKey: true},
		{Name: "node.role", Type: "string", Default: "receiver", HotReload: false, Risk: RiskMedium, Description: "节点角色（receiver/sender）。"},
	}
}

func profileValues(profile Profile) map[string]any {
	values := map[string]any{}
	for _, item := range Catalog() {
		values[item.Name] = item.Default
	}
	switch profile {
	case ProfileDev:
		values["network.sip.listen_ip"] = "127.0.0.1"
		values["network.rtp.listen_ip"] = "127.0.0.1"
		values["network.sip.max_message_bytes"] = 4096
		values["network.rtp.max_inflight_transfers"] = 8
	case ProfileTest:
		values["network.sip.listen_ip"] = "127.0.0.1"
		values["network.rtp.listen_ip"] = "127.0.0.1"
		values["network.sip.listen_port"] = 15060
		values["network.rtp.port_start"] = 21000
		values["network.rtp.port_end"] = 21020
		values["network.rtp.max_inflight_transfers"] = 16
	case ProfileProd:
		values["network.sip.advertise_ip"] = "10.20.30.10"
		values["network.rtp.advertise_ip"] = "10.20.30.10"
		values["network.sip.domain"] = "prod.siptunnel.local"
		values["network.rtp.max_inflight_transfers"] = 128
		values["network.rtp.receive_buffer_bytes"] = 8388608
		values["network.rtp.transfer_timeout_ms"] = 45000
	}
	return values
}

func formatDefault(v any) string {
	switch t := v.(type) {
	case string:
		if t == "" {
			return "\"\""
		}
		return t
	case bool:
		if t {
			return "true"
		}
		return "false"
	case int:
		return strconv.Itoa(t)
	default:
		return fmt.Sprintf("%v", t)
	}
}

func markRisk(meta ParamMeta) string {
	if meta.NetworkKey && meta.Risk == RiskHigh {
		return "⚠️ HIGH-NET"
	}
	return strings.ToUpper(string(meta.Risk))
}
