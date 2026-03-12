package selfcheck

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sort"
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
}

type Runner struct {
	nowFn          func() time.Time
	interfaceIPs   func() (map[string]struct{}, error)
	listenTCP      func(network, address string) (net.Listener, error)
	listenUDP      func(network, address string) (net.PacketConn, error)
	ensureWritable func(path string) error
	dialTCP        func(ctx context.Context, address string) error
}

func NewRunner() *Runner {
	dialer := &net.Dialer{Timeout: 2 * time.Second}
	return &Runner{
		nowFn:          func() time.Time { return time.Now().UTC() },
		interfaceIPs:   hostInterfaceIPs,
		listenTCP:      net.Listen,
		listenUDP:      net.ListenPacket,
		ensureWritable: ensureDirWritable,
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

	items := []Item{}
	items = append(items, r.checkListenIP("sip.listen_ip", in.NetworkConfig.SIP.Enabled, in.NetworkConfig.SIP.ListenIP)...)
	items = append(items, r.checkListenIP("rtp.listen_ip", in.NetworkConfig.RTP.Enabled, in.NetworkConfig.RTP.ListenIP)...)
	items = append(items, r.checkSIPPortOccupancy(in.NetworkConfig.SIP)...)
	items = append(items, r.checkSIPUDPMessageSizeRisk(in.NetworkConfig.SIP)...)
	items = append(items, r.checkRTPRange(in.NetworkConfig.RTP)...)
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
		b.WriteString(fmt.Sprintf("- [%s] %s: %s | suggestion: %s\n", strings.ToUpper(string(item.Level)), item.Name, item.Message, item.Suggestion))
	}
	return strings.TrimRight(b.String(), "\n")
}

func (r *Runner) checkListenIP(name string, enabled bool, ip string) []Item {
	if !enabled {
		return []Item{{Name: name, Level: LevelInfo, Message: "已禁用，跳过 listen_ip 存在性检查", Suggestion: "如需启用该平面，请配置有效 listen_ip 并重新执行自检"}}
	}
	trimmed := strings.TrimSpace(ip)
	if trimmed == "" {
		return []Item{{Name: name, Level: LevelError, Message: "listen_ip 为空", Suggestion: "请在配置中填写本机可绑定 IP（或 0.0.0.0）"}}
	}
	if isWildcardIP(trimmed) {
		return []Item{{Name: name, Level: LevelWarn, Message: fmt.Sprintf("listen_ip=%s 为通配地址，无法精确校验网卡存在性", trimmed), Suggestion: "若需严格约束到指定网卡，请改为明确的本机 IP"}}
	}
	ips, err := r.interfaceIPs()
	if err != nil {
		return []Item{{Name: name, Level: LevelWarn, Message: fmt.Sprintf("获取本机网卡地址失败: %v", err), Suggestion: "请检查容器/主机网络命名空间与权限"}}
	}
	if _, ok := ips[trimmed]; !ok {
		return []Item{{Name: name, Level: LevelError, Message: fmt.Sprintf("listen_ip=%s 不在本机网卡地址列表中", trimmed), Suggestion: "请改为本机可用 IP，或确认网卡/IP 已正确下发"}}
	}
	return []Item{{Name: name, Level: LevelInfo, Message: fmt.Sprintf("listen_ip=%s 存在于本机网卡", trimmed), Suggestion: "无需处理"}}
}

func (r *Runner) checkSIPPortOccupancy(sip config.SIPConfig) []Item {
	const name = "sip.listen_port_occupancy"
	if !sip.Enabled {
		return []Item{{Name: name, Level: LevelInfo, Message: "SIP 已禁用，跳过端口占用检查", Suggestion: "如需启用 SIP，请配置 listen_port 并重新自检"}}
	}
	addr := net.JoinHostPort(strings.TrimSpace(sip.ListenIP), fmt.Sprintf("%d", sip.ListenPort))
	transport := strings.ToUpper(strings.TrimSpace(sip.Transport))
	switch transport {
	case "TCP", "TLS":
		ln, err := r.listenTCP("tcp", addr)
		if err != nil {
			return []Item{{Name: name, Level: LevelError, Message: fmt.Sprintf("SIP 端口检查失败：%v", err), Suggestion: "请释放冲突端口或调整 sip.listen_port"}}
		}
		_ = ln.Close()
	case "UDP":
		pc, err := r.listenUDP("udp", addr)
		if err != nil {
			return []Item{{Name: name, Level: LevelError, Message: fmt.Sprintf("SIP 端口检查失败：%v", err), Suggestion: "请释放冲突端口或调整 sip.listen_port"}}
		}
		_ = pc.Close()
	default:
		return []Item{{Name: name, Level: LevelError, Message: fmt.Sprintf("不支持的 SIP transport=%s", sip.Transport), Suggestion: "请将 sip.transport 设置为 TCP/UDP/TLS"}}
	}
	return []Item{{Name: name, Level: LevelInfo, Message: fmt.Sprintf("SIP 监听地址 %s 可成功绑定", addr), Suggestion: "无需处理"}}
}

func (r *Runner) checkSIPUDPMessageSizeRisk(sip config.SIPConfig) []Item {
	const name = "sip.udp_message_size_risk"
	if !sip.Enabled {
		return []Item{{Name: name, Level: LevelInfo, Message: "SIP 已禁用，跳过 UDP 报文长度风险检查", Suggestion: "如需启用 SIP，请配置后重新自检"}}
	}
	if !sip.UDPMessageSizeRisk() {
		return []Item{{Name: name, Level: LevelInfo, Message: "SIP 报文大小与 transport 匹配，无明显分片风险", Suggestion: "无需处理"}}
	}
	return []Item{{Name: name, Level: LevelWarn, Message: fmt.Sprintf("当前 sip.transport=UDP 且 max_message_bytes=%d，超过推荐值 %d，存在分片/丢包风险", sip.MaxMessageBytes, config.SIPUDPRecommendedMaxMessageBytes), Suggestion: "建议降低 sip.max_message_bytes（如 <= 1300）或切换 sip.transport=TCP"}}
}

func (r *Runner) checkRTPRange(rtp config.RTPConfig) []Item {
	const name = "rtp.port_range_validity"
	if !rtp.Enabled {
		return []Item{{Name: name, Level: LevelInfo, Message: "RTP 已禁用，跳过端口范围检查", Suggestion: "如需启用 RTP，请配置端口范围并重新自检"}}
	}
	if rtp.PortStart < 1 || rtp.PortStart > 65535 || rtp.PortEnd < 1 || rtp.PortEnd > 65535 {
		return []Item{{Name: name, Level: LevelError, Message: fmt.Sprintf("RTP 端口范围非法 [%d,%d]，必须在 [1,65535]", rtp.PortStart, rtp.PortEnd), Suggestion: "请将 rtp.port_start/rtp.port_end 调整到有效范围"}}
	}
	if rtp.PortStart > rtp.PortEnd {
		return []Item{{Name: name, Level: LevelError, Message: fmt.Sprintf("RTP 端口范围非法：start=%d 大于 end=%d", rtp.PortStart, rtp.PortEnd), Suggestion: "请确保 rtp.port_start <= rtp.port_end"}}
	}
	return []Item{{Name: name, Level: LevelInfo, Message: fmt.Sprintf("RTP 端口范围 [%d,%d] 合法", rtp.PortStart, rtp.PortEnd), Suggestion: "无需处理"}}
}

func (r *Runner) checkSIPRTPConflict(cfg config.NetworkConfig) []Item {
	const name = "sip_rtp_port_conflict"
	if !cfg.SIP.Enabled || !cfg.RTP.Enabled {
		return []Item{{Name: name, Level: LevelInfo, Message: "SIP 或 RTP 未同时启用，跳过冲突检查", Suggestion: "启用双平面后建议重新自检"}}
	}
	sipTransport := strings.ToUpper(strings.TrimSpace(cfg.SIP.Transport))
	rtpTransport := strings.ToUpper(strings.TrimSpace(cfg.RTP.Transport))
	if sipTransport != rtpTransport {
		return []Item{{Name: name, Level: LevelInfo, Message: fmt.Sprintf("传输层不同（SIP=%s, RTP=%s），无端口冲突", sipTransport, rtpTransport), Suggestion: "无需处理"}}
	}
	if !sameBindAddress(cfg.SIP.ListenIP, cfg.RTP.ListenIP) {
		return []Item{{Name: name, Level: LevelInfo, Message: "SIP/RTP 监听 IP 不同，无冲突", Suggestion: "无需处理"}}
	}
	if cfg.SIP.ListenPort >= cfg.RTP.PortStart && cfg.SIP.ListenPort <= cfg.RTP.PortEnd {
		return []Item{{Name: name, Level: LevelError, Message: fmt.Sprintf("SIP 端口 %d 与 RTP 范围 [%d,%d] 冲突", cfg.SIP.ListenPort, cfg.RTP.PortStart, cfg.RTP.PortEnd), Suggestion: "请调整 sip.listen_port 或 rtp 端口范围，避免重叠"}}
	}
	return []Item{{Name: name, Level: LevelInfo, Message: "SIP 与 RTP 端口无冲突", Suggestion: "无需处理"}}
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
			items = append(items, Item{Name: dir.name, Level: LevelError, Message: "目录路径为空", Suggestion: "请配置有效目录路径并确保目录可写"})
			continue
		}
		if err := r.ensureWritable(dir.path); err != nil {
			items = append(items, Item{Name: dir.name, Level: LevelError, Message: fmt.Sprintf("目录不可写: %v", err), Suggestion: "请检查目录权限、属主与磁盘空间"})
			continue
		}
		items = append(items, Item{Name: dir.name, Level: LevelInfo, Message: fmt.Sprintf("目录 %q 可写", dir.path), Suggestion: "无需处理"})
	}
	return items
}

func (r *Runner) checkDownstreamReachability(ctx context.Context, routes []httpinvoke.RouteConfig, timeout time.Duration) []Item {
	const name = "downstream.http_base_reachability"
	if len(routes) == 0 {
		return []Item{{Name: name, Level: LevelWarn, Message: "未配置下游 HTTP 路由，跳过可达性检查", Suggestion: "请加载 httpinvoke 路由配置后重试"}}
	}
	set := map[string]struct{}{}
	for _, route := range routes {
		if strings.TrimSpace(route.TargetHost) == "" || route.TargetPort <= 0 {
			continue
		}
		set[net.JoinHostPort(route.TargetHost, fmt.Sprintf("%d", route.TargetPort))] = struct{}{}
	}
	if len(set) == 0 {
		return []Item{{Name: name, Level: LevelWarn, Message: "路由中缺少有效 target_host/target_port，无法检查可达性", Suggestion: "请补齐下游目标地址配置"}}
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
			items = append(items, Item{Name: name, Level: LevelError, Message: fmt.Sprintf("下游地址 %s 不可达: %v", addr, err), Suggestion: "请检查目标服务状态、网络连通性、ACL/防火墙策略"})
			continue
		}
		items = append(items, Item{Name: name, Level: LevelInfo, Message: fmt.Sprintf("下游地址 %s TCP 可达", addr), Suggestion: "无需处理"})
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
