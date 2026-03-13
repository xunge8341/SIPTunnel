package selfcheck

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	"siptunnel/internal/config"
	"siptunnel/internal/service/httpinvoke"
)

type Level string

const (
	LevelInfo  Level = "info"
	LevelWarn  Level = "warn"
	LevelError Level = "error"
)

type Item struct {
	Name       string `json:"name"`
	Level      Level  `json:"level"`
	Message    string `json:"message"`
	Suggestion string `json:"suggestion"`
	ActionHint string `json:"action_hint"`
	DocLink    string `json:"doc_link,omitempty"`
}

type Summary struct {
	Info  int `json:"info"`
	Warn  int `json:"warn"`
	Error int `json:"error"`
}

type Report struct {
	GeneratedAt time.Time `json:"generated_at"`
	Overall     Level     `json:"overall"`
	Summary     Summary   `json:"summary"`
	Items       []Item    `json:"items"`
}

type Input struct {
	NetworkConfig    config.NetworkConfig
	StoragePaths     config.StoragePaths
	DownstreamRoutes []httpinvoke.RouteConfig
	DialTimeout      time.Duration
	RunMode          string
	SuggestFreePort  bool
}

type Runner struct {
	nowFn          func() time.Time
	interfaceIPs   func() (map[string]struct{}, error)
	listenTCP      func(network, address string) (net.Listener, error)
	listenUDP      func(network, address string) (net.PacketConn, error)
	ensureWritable func(path string) error
	dialTCP        func(ctx context.Context, address string) error
	execCommand    func(ctx context.Context, name string, args ...string) ([]byte, error)
	lookPath       func(file string) (string, error)
	findFreePort   func(transport string, ip string) (int, error)
}

func NewRunner() *Runner {
	dialer := &net.Dialer{Timeout: 2 * time.Second}
	return &Runner{
		nowFn:          func() time.Time { return time.Now().UTC() },
		interfaceIPs:   hostInterfaceIPs,
		listenTCP:      net.Listen,
		listenUDP:      net.ListenPacket,
		ensureWritable: ensureDirWritable,
		execCommand: func(ctx context.Context, name string, args ...string) ([]byte, error) {
			return exec.CommandContext(ctx, name, args...).CombinedOutput()
		},
		lookPath:     exec.LookPath,
		findFreePort: findAvailablePort,
		dialTCP: func(ctx context.Context, address string) error {
			conn, err := dialer.DialContext(ctx, "tcp", address)
			if err != nil {
				return err
			}
			_ = conn.Close()
			return nil
		},
	}
}

func (r *Runner) Run(ctx context.Context, in Input) Report {
	if r == nil {
		r = NewRunner()
	}
	if in.DialTimeout <= 0 {
		in.DialTimeout = 2 * time.Second
	}
	if !isProdMode(in.RunMode) && !in.SuggestFreePort {
		in.SuggestFreePort = true
	}

	items := []Item{}
	items = append(items, r.checkListenIP("sip.listen_ip", in.NetworkConfig.SIP.Enabled, in.NetworkConfig.SIP.ListenIP)...)
	items = append(items, r.checkListenIP("rtp.listen_ip", in.NetworkConfig.RTP.Enabled, in.NetworkConfig.RTP.ListenIP)...)
	items = append(items, r.checkSIPPortOccupancy(in.NetworkConfig.SIP, in)...)
	items = append(items, r.checkSIPUDPMessageSizeRisk(in.NetworkConfig.SIP)...)
	items = append(items, r.checkRTPRange(in.NetworkConfig.RTP)...)
	items = append(items, r.checkRTPTransportPlan(in.NetworkConfig.RTP)...)
	items = append(items, r.checkSIPRTPConflict(in.NetworkConfig)...)
	items = append(items, r.checkWritableDirs(in.StoragePaths)...)
	items = append(items, r.checkDownstreamReachability(ctx, in.DownstreamRoutes, in.DialTimeout)...)

	report := Report{GeneratedAt: r.nowFn(), Overall: LevelInfo, Items: items}
	for _, item := range items {
		switch item.Level {
		case LevelError:
			report.Summary.Error++
			report.Overall = LevelError
		case LevelWarn:
			report.Summary.Warn++
			if report.Overall != LevelError {
				report.Overall = LevelWarn
			}
		default:
			report.Summary.Info++
		}
	}
	return report
}

func (r Report) ToCLI() string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("self-check generated_at=%s overall=%s info=%d warn=%d error=%d\n", r.GeneratedAt.Format(time.RFC3339), r.Overall, r.Summary.Info, r.Summary.Warn, r.Summary.Error))
	for _, item := range r.Items {
		b.WriteString(fmt.Sprintf("- [%s] %s: %s | suggestion: %s | action_hint: %s", strings.ToUpper(string(item.Level)), item.Name, item.Message, item.Suggestion, item.ActionHint))
		if item.DocLink != "" {
			b.WriteString(" | doc: " + item.DocLink)
		}
		b.WriteString("\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

func item(name string, level Level, message string, suggestion string, actionHint string, docLink string) Item {
	return Item{Name: name, Level: level, Message: message, Suggestion: suggestion, ActionHint: actionHint, DocLink: docLink}
}

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
	owner := r.detectPortOwner(context.Background(), port)
	message := fmt.Sprintf("SIP 端口检查失败（%s %s:%d）：%v", transport, strings.TrimSpace(listenIP), port, bindErr)
	if owner != "" {
		message += "；疑似占用进程=" + owner
	}
	suggestionParts := []string{buildPortDiagnosticSuggestion(port)}
	if in.SuggestFreePort {
		if freePort, err := r.findFreePort(transport, listenIP); err == nil && freePort > 0 && freePort != port {
			suggestionParts = append(suggestionParts, fmt.Sprintf("开发模式建议先切换 sip.listen_port=%d 进行快速联调（变更后请重启并复核 /api/selfcheck）", freePort))
		}
	}
	suggestionParts = append(suggestionParts, "生产模式默认不自动改端口，请先释放冲突端口或人工修改 sip.listen_port")
	return item(name, LevelError, message, strings.Join(suggestionParts, "；"), "按建议定位占用进程并释放端口；变更后重启并复核", "docs/troubleshooting.md#33-端口冲突错误")
}

func buildPortDiagnosticSuggestion(port int) string {
	if runtime.GOOS == "windows" {
		return fmt.Sprintf("Windows 排查可执行：netstat -ano | findstr :%d；tasklist /fi \"PID eq <pid>\"", port)
	}
	return fmt.Sprintf("Linux 排查可执行：ss -ltnp；lsof -i :%d", port)
}

func (r *Runner) detectPortOwner(ctx context.Context, port int) string {
	if r == nil || r.lookPath == nil || r.execCommand == nil || port <= 0 {
		return ""
	}
	if runtime.GOOS == "windows" {
		pid := r.detectWindowsPID(ctx, port)
		if pid == "" {
			return ""
		}
		return r.lookupWindowsProcess(ctx, pid)
	}
	return r.lookupLinuxProcess(ctx, port)
}

func (r *Runner) lookupLinuxProcess(ctx context.Context, port int) string {
	if _, err := r.lookPath("lsof"); err != nil {
		return ""
	}
	out, err := r.execCommand(ctx, "lsof", "-nP", fmt.Sprintf("-i:%d", port))
	if err != nil {
		return ""
	}
	lines := strings.Split(string(out), "\n")
	for _, line := range lines[1:] {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		if _, err := strconv.Atoi(fields[1]); err != nil {
			continue
		}
		return fields[0] + "(pid=" + fields[1] + ")"
	}
	return ""
}

func (r *Runner) detectWindowsPID(ctx context.Context, port int) string {
	if _, err := r.lookPath("netstat"); err != nil {
		return ""
	}
	out, err := r.execCommand(ctx, "netstat", "-ano")
	if err != nil {
		return ""
	}
	target := ":" + strconv.Itoa(port)
	for _, line := range strings.Split(string(out), "\n") {
		if !strings.Contains(line, target) {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 5 {
			continue
		}
		pid := fields[len(fields)-1]
		if _, err := strconv.Atoi(pid); err == nil {
			return pid
		}
	}
	return ""
}

func (r *Runner) lookupWindowsProcess(ctx context.Context, pid string) string {
	if pid == "" {
		return ""
	}
	if _, err := r.lookPath("tasklist"); err != nil {
		return "pid=" + pid
	}
	out, err := r.execCommand(ctx, "tasklist", "/fi", "PID eq "+pid)
	if err != nil {
		return "pid=" + pid
	}
	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		if fields[len(fields)-1] == pid {
			return fields[0] + "(pid=" + pid + ")"
		}
	}
	return "pid=" + pid
}

func findAvailablePort(transport string, ip string) (int, error) {
	bindIP := strings.TrimSpace(ip)
	if bindIP == "" {
		bindIP = "0.0.0.0"
	}
	addr := net.JoinHostPort(bindIP, "0")
	if strings.EqualFold(transport, "UDP") {
		pc, err := net.ListenPacket("udp", addr)
		if err != nil {
			return 0, err
		}
		defer pc.Close()
		port := pc.LocalAddr().(*net.UDPAddr).Port
		return port, nil
	}
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return 0, err
	}
	defer ln.Close()
	port := ln.Addr().(*net.TCPAddr).Port
	return port, nil
}

func isProdMode(mode string) bool {
	return strings.EqualFold(strings.TrimSpace(mode), "prod")
}

func (r *Runner) checkSIPUDPMessageSizeRisk(sip config.SIPConfig) []Item {
	const name = "sip.udp_message_size_risk"
	if !sip.Enabled {
		return []Item{item(name, LevelInfo, "SIP 已禁用，跳过 UDP 报文长度风险检查", "如需启用 SIP，请配置后重新自检", "仅在启用 SIP/UDP 时关注该项", "")}
	}
	if !sip.UDPMessageSizeRisk() {
		return []Item{item(name, LevelInfo, "SIP 报文大小与 transport 匹配，无明显分片风险", "无需处理", "持续观察丢包率指标", "")}
	}
	return []Item{item(name, LevelWarn, fmt.Sprintf("当前 sip.transport=UDP 且 max_message_bytes=%d，超过推荐值 %d，存在分片/丢包风险", sip.MaxMessageBytes, config.SIPUDPRecommendedMaxMessageBytes), "建议降低 sip.max_message_bytes（如 <= 1300）或切换 sip.transport=TCP", "先在预发下调报文上限并做回归，再上线", "docs/troubleshooting.md#35-sip-transport-配置错误")}
}

func (r *Runner) checkRTPRange(rtp config.RTPConfig) []Item {
	const name = "rtp.port_range_validity"
	if !rtp.Enabled {
		return []Item{item(name, LevelInfo, "RTP 已禁用，跳过端口范围检查", "如需启用 RTP，请配置端口范围并重新自检", "启用 RTP 前先完成容量评估", "")}
	}
	if rtp.PortStart < 1 || rtp.PortStart > 65535 || rtp.PortEnd < 1 || rtp.PortEnd > 65535 {
		return []Item{item(name, LevelError, fmt.Sprintf("RTP 端口范围非法 [%d,%d]，必须在 [1,65535]", rtp.PortStart, rtp.PortEnd), "请将 rtp.port_start/rtp.port_end 调整到有效范围", "修正配置后执行预检与自检", "docs/troubleshooting.md#36-rtp-端口池错误")}
	}
	if rtp.PortStart > rtp.PortEnd {
		return []Item{item(name, LevelError, fmt.Sprintf("RTP 端口范围非法：start=%d 大于 end=%d", rtp.PortStart, rtp.PortEnd), "请确保 rtp.port_start <= rtp.port_end", "按并发峰值重新规划端口池", "docs/troubleshooting.md#36-rtp-端口池错误")}
	}
	return []Item{item(name, LevelInfo, fmt.Sprintf("RTP 端口范围 [%d,%d] 合法", rtp.PortStart, rtp.PortEnd), "无需处理", "定期复核端口池使用率", "")}
}

func (r *Runner) checkRTPTransportPlan(rtp config.RTPConfig) []Item {
	const name = "rtp.transport_plan"
	if !rtp.Enabled {
		return []Item{item(name, LevelInfo, "RTP 已禁用，跳过传输模式规划检查", "如需启用 RTP，请配置 transport 并重新自检", "启用前确认跨网链路策略", "")}
	}
	transport := strings.ToUpper(strings.TrimSpace(rtp.Transport))
	if transport == "TCP" {
		return []Item{item(name, LevelWarn, "当前 rtp.transport=TCP 已可用于联调验证（受控发布）", "生产环境建议保持 rtp.transport=UDP，TCP 仅用于跨网调试与防丢包验证", "若非排障窗口，请切回 UDP 并记录变更", "docs/troubleshooting.md#39-transport-不匹配")}
	}
	return []Item{item(name, LevelInfo, "当前 rtp.transport=UDP（生产默认）", "无需处理", "保持现网默认策略", "")}
}

func (r *Runner) checkSIPRTPConflict(cfg config.NetworkConfig) []Item {
	const name = "sip_rtp_port_conflict"
	if !cfg.SIP.Enabled || !cfg.RTP.Enabled {
		return []Item{item(name, LevelInfo, "SIP 或 RTP 未同时启用，跳过冲突检查", "启用双平面后建议重新自检", "按业务拓扑确认是否需要双平面", "")}
	}
	sipTransport := strings.ToUpper(strings.TrimSpace(cfg.SIP.Transport))
	rtpTransport := strings.ToUpper(strings.TrimSpace(cfg.RTP.Transport))
	if sipTransport != rtpTransport {
		return []Item{item(name, LevelInfo, fmt.Sprintf("传输层不同（SIP=%s, RTP=%s），无端口冲突", sipTransport, rtpTransport), "无需处理", "确认该 transport 组合符合演练方案", "docs/troubleshooting.md#39-transport-不匹配")}
	}
	if !sameBindAddress(cfg.SIP.ListenIP, cfg.RTP.ListenIP) {
		return []Item{item(name, LevelInfo, "SIP/RTP 监听 IP 不同，无冲突", "无需处理", "保留分平面监听策略", "")}
	}
	if cfg.SIP.ListenPort >= cfg.RTP.PortStart && cfg.SIP.ListenPort <= cfg.RTP.PortEnd {
		return []Item{item(name, LevelError, fmt.Sprintf("SIP 端口 %d 与 RTP 范围 [%d,%d] 冲突", cfg.SIP.ListenPort, cfg.RTP.PortStart, cfg.RTP.PortEnd), "请调整 sip.listen_port 或 rtp 端口范围，避免重叠", "按端口规划文档重新分配后再上线", "docs/troubleshooting.md#33-端口冲突错误")}
	}
	return []Item{item(name, LevelInfo, "SIP 与 RTP 端口无冲突", "无需处理", "保持当前配置", "")}
}

func (r *Runner) checkWritableDirs(paths config.StoragePaths) []Item {
	dirs := []struct {
		name string
		path string
	}{
		{name: "storage.temp_dir_writable", path: paths.TempDir},
		{name: "storage.final_dir_writable", path: paths.FinalDir},
		{name: "storage.audit_dir_writable", path: paths.AuditDir},
	}
	items := make([]Item, 0, len(dirs))
	for _, dir := range dirs {
		if strings.TrimSpace(dir.path) == "" {
			items = append(items, item(dir.name, LevelError, "目录路径为空", "请配置有效目录路径并确保目录可写", "补齐目录配置并校验挂载点", "docs/troubleshooting.md#34-目录不可写错误"))
			continue
		}
		if err := r.ensureWritable(dir.path); err != nil {
			items = append(items, item(dir.name, LevelError, fmt.Sprintf("目录不可写: %v", err), "请检查目录权限、属主与磁盘空间", "按服务用户执行 touch 写入测试并修复权限", "docs/troubleshooting.md#34-目录不可写错误"))
			continue
		}
		items = append(items, item(dir.name, LevelInfo, fmt.Sprintf("目录 %q 可写", dir.path), "无需处理", "维持权限基线", ""))
	}
	return items
}

func (r *Runner) checkDownstreamReachability(ctx context.Context, routes []httpinvoke.RouteConfig, timeout time.Duration) []Item {
	const name = "downstream.http_base_reachability"
	if len(routes) == 0 {
		return []Item{item(name, LevelWarn, "未配置下游 HTTP 路由，跳过可达性检查", "请加载 httpinvoke 路由配置后重试", "确认业务所需 api_code 已完成模板映射并发布", "docs/troubleshooting.md#310-下游-http-未配置")}
	}
	set := map[string]struct{}{}
	for _, route := range routes {
		if strings.TrimSpace(route.TargetHost) == "" || route.TargetPort <= 0 {
			continue
		}
		set[net.JoinHostPort(route.TargetHost, fmt.Sprintf("%d", route.TargetPort))] = struct{}{}
	}
	if len(set) == 0 {
		return []Item{item(name, LevelWarn, "路由中缺少有效 target_host/target_port，无法检查可达性", "请补齐下游目标地址配置", "逐条校验 target_host 与 target_port", "docs/troubleshooting.md#310-下游-http-未配置")}
	}
	addrs := make([]string, 0, len(set))
	for addr := range set {
		addrs = append(addrs, addr)
	}
	sort.Strings(addrs)
	items := make([]Item, 0, len(addrs))
	for _, addr := range addrs {
		cctx, cancel := context.WithTimeout(ctx, timeout)
		err := r.dialTCP(cctx, addr)
		cancel()
		if err != nil {
			items = append(items, item(name, LevelError, fmt.Sprintf("下游地址 %s 不可达: %v", addr, err), "请检查目标服务状态、网络连通性、ACL/防火墙策略", "先在网关主机执行 curl/telnet 验证，再联系下游服务负责人", "docs/troubleshooting.md#31-result_code-异常http-调用结果异常"))
			continue
		}
		items = append(items, item(name, LevelInfo, fmt.Sprintf("下游地址 %s TCP 可达", addr), "无需处理", "保持连通性监控", ""))
	}
	return items
}

func hostInterfaceIPs() (map[string]struct{}, error) {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}
	out := make(map[string]struct{})
	for _, iface := range ifaces {
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			ip, _, err := net.ParseCIDR(addr.String())
			if err != nil {
				continue
			}
			out[ip.String()] = struct{}{}
		}
	}
	return out, nil
}

func ensureDirWritable(dir string) error {
	cleaned := filepath.Clean(dir)
	if err := os.MkdirAll(cleaned, 0o755); err != nil {
		return fmt.Errorf("create directory: %w", err)
	}
	f, err := os.CreateTemp(cleaned, ".self-check-write-*")
	if err != nil {
		return fmt.Errorf("directory is not writable: %w", err)
	}
	name := f.Name()
	_ = f.Close()
	_ = os.Remove(name)
	return nil
}

func isWildcardIP(ip string) bool {
	return ip == "0.0.0.0" || ip == "::"
}

func sameBindAddress(a, b string) bool {
	a = strings.TrimSpace(a)
	b = strings.TrimSpace(b)
	if a == b {
		return true
	}
	return isWildcardIP(a) || isWildcardIP(b)
}
