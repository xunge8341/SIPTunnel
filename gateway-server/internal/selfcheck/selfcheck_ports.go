package selfcheck

import (
	"fmt"
	"net"
	"runtime"
	"strings"

	"siptunnel/internal/config"
)

func (r *Runner) checkListenIP(name string, enabled bool, ip string) []Item {
	if !enabled {
		return []Item{item(name, LevelInfo, "已禁用，跳过 listen_ip 存在性检查", "如需启用该平面，请配置有效 listen_ip 并重新执行自检", "确认是否启用该平面；启用后重跑 /api/selfcheck", "docs/troubleshooting.md#38-通配地址-0000-风险")}
	}
	trimmed := strings.TrimSpace(ip)
	if trimmed == "" {
		return []Item{item(name, LevelError, "listen_ip 为空", "请在配置中填写本机可绑定 IP（或 0.0.0.0）", "补齐配置后重启服务并复核自检", "docs/troubleshooting.md#32-配置校验错误")}
	}
	if isWildcardIP(trimmed) {
		return []Item{item(name, LevelWarn, fmt.Sprintf("listen_ip=%s 为通配地址，无法精确校验网卡存在性", trimmed), "若需严格约束到指定网卡，请改为明确的本机 IP", "生产优先绑定专用网卡 IP；变更后执行灰度重启", "docs/troubleshooting.md#38-通配地址-0000-风险")}
	}
	ips, err := r.interfaceIPs()
	if err != nil {
		return []Item{item(name, LevelWarn, fmt.Sprintf("获取本机网卡地址失败: %v", err), "请检查容器/主机网络命名空间与权限", "进入主机执行 ip addr / ipconfig，核对网卡后再重试", "docs/troubleshooting.md#32-配置校验错误")}
	}
	if _, ok := ips[trimmed]; !ok {
		return []Item{item(name, LevelError, fmt.Sprintf("listen_ip=%s 不在本机网卡地址列表中", trimmed), "请改为本机可用 IP，或确认网卡/IP 已正确下发", "先修正配置，再执行 systemctl restart siptunnel-gateway", "docs/troubleshooting.md#32-配置校验错误")}
	}
	return []Item{item(name, LevelInfo, fmt.Sprintf("listen_ip=%s 存在于本机网卡", trimmed), "无需处理", "保持现状并纳入变更基线", "")}
}

func (r *Runner) checkSIPPortOccupancy(sip config.SIPConfig, in Input) []Item {
	const name = "sip.listen_port_occupancy"
	if !sip.Enabled {
		return []Item{item(name, LevelInfo, "SIP 已禁用，跳过端口占用检查", "如需启用 SIP，请配置 listen_port 并重新自检", "确认该节点是否承担 SIP 控制面", "docs/operations.md#91-自检覆盖项")}
	}
	addr := net.JoinHostPort(strings.TrimSpace(sip.ListenIP), fmt.Sprintf("%d", sip.ListenPort))
	transport := strings.ToUpper(strings.TrimSpace(sip.Transport))
	switch transport {
	case "TCP", "TLS":
		ln, err := r.listenTCP("tcp", addr)
		if err != nil {
			return []Item{r.buildPortOccupancyError(name, transport, sip.ListenIP, sip.ListenPort, err, in)}
		}
		_ = ln.Close()
	case "UDP":
		pc, err := r.listenUDP("udp", addr)
		if err != nil {
			return []Item{r.buildPortOccupancyError(name, transport, sip.ListenIP, sip.ListenPort, err, in)}
		}
		_ = pc.Close()
	default:
		return []Item{item(name, LevelError, fmt.Sprintf("不支持的 SIP transport=%s", sip.Transport), "请将 sip.transport 设置为 TCP/UDP/TLS", "修正配置并重启；必要时回滚到最近可用版本", "docs/troubleshooting.md#35-sip-transport-配置错误")}
	}
	return []Item{item(name, LevelInfo, fmt.Sprintf("SIP 监听地址 %s 可成功绑定", addr), "无需处理", "保留当前端口规划", "")}
}

func (r *Runner) buildPortOccupancyError(name string, transport string, listenIP string, port int, bindErr error, in Input) Item {
	owner := r.detectPortOwnerInfo(port)
	if in.ExpectSIPPortOwnedByCurrentProcess || owner.SelfOwned {
		ownerText := owner.Summary
		if ownerText == "" {
			ownerText = "current-gateway-process"
		}
		message := fmt.Sprintf("SIP 监听地址 %s:%d 已由当前 gateway 进程绑定（%s），运行态自检视为正常", strings.TrimSpace(listenIP), port, ownerText)
		return item(name, LevelInfo, message, "无需处理", "保持当前监听状态；如需验证外部占用，请先停止当前进程后再执行离线自检", "")
	}
	message := fmt.Sprintf("SIP 端口检查失败（%s %s:%d）：%v", transport, strings.TrimSpace(listenIP), port, bindErr)
	if owner.Summary != "" {
		message += "；疑似占用进程=" + owner.Summary
	}
	suggestionParts := []string{buildPortDiagnosticSuggestionForOS(runnerGOOS(r), port)}
	if in.SuggestFreePort {
		if freePort, err := r.findFreePort(transport, listenIP); err == nil && freePort > 0 && freePort != port {
			suggestionParts = append(suggestionParts, fmt.Sprintf("开发模式建议先切换 sip.listen_port=%d 进行快速联调（变更后请重启并复核 /api/selfcheck）", freePort))
		}
	}
	suggestionParts = append(suggestionParts, "生产模式默认不自动改端口，请先释放冲突端口或人工修改 sip.listen_port")
	return item(name, LevelError, message, strings.Join(suggestionParts, "；"), "按建议定位占用进程并释放端口；变更后重启并复核", "docs/troubleshooting.md#33-端口冲突错误")
}

func runnerGOOS(r *Runner) string {
	if r != nil && r.goos != nil {
		if goos := strings.TrimSpace(r.goos()); goos != "" {
			return goos
		}
	}
	return runtime.GOOS
}

func buildPortDiagnosticSuggestion(port int) string {
	return buildPortDiagnosticSuggestionForOS(runnerGOOS(nil), port)
}

func buildPortDiagnosticSuggestionForOS(goos string, port int) string {
	if goos == "windows" {
		return fmt.Sprintf("Windows 排查可执行：Get-NetTCPConnection -LocalPort %d；netstat -ano | findstr :%d；tasklist /fi \"PID eq <pid>\"", port, port)
	}
	return fmt.Sprintf("Linux 排查可执行：ss -ltnp；lsof -i :%d", port)
}
