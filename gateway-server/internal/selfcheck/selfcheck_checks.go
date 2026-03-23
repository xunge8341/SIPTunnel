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
	"siptunnel/internal/netbind"
	"siptunnel/internal/service/httpinvoke"
)

func (r *Runner) checkSIPUDPMessageSizeRisk(cfg config.NetworkConfig) []Item {
	sip := cfg.SIP
	const name = "sip.udp_message_size_risk"
	if !sip.Enabled {
		return []Item{item(name, LevelInfo, "SIP 已禁用，跳过 UDP 报文长度风险检查", "如需启用 SIP，请配置后重新自检", "仅在启用 SIP/UDP 时关注该项", "")}
	}
	effectiveLimit := config.ResolveTransportPlanForConfig(cfg).RequestBodySizeLimit
	if !sip.UDPMessageSizeRisk() {
		return []Item{item(name, LevelInfo, fmt.Sprintf("SIP 报文大小与 transport 匹配，无明显分片风险（effective_udp_control_budget=%d）", effectiveLimit), "无需处理", "持续观察丢包率指标", "")}
	}
	if effectiveLimit > 0 && effectiveLimit <= config.SIPUDPRecommendedMaxMessageBytes {
		return []Item{item(name, LevelInfo, fmt.Sprintf("当前 sip.max_message_bytes=%d 偏大，但运行时 transport_tuning 已将控制面收口到 %d 字节", sip.MaxMessageBytes, effectiveLimit), "保持 transport_tuning 与启动摘要口径一致", "继续核对大响应路径是否自动切换 RTP", "docs/troubleshooting.md#35-sip-transport-配置错误")}
	}
	return []Item{item(name, LevelWarn, fmt.Sprintf("当前 sip.transport=UDP 且 max_message_bytes=%d，effective_udp_control_budget=%d，超过推荐值 %d，存在分片/丢包风险", sip.MaxMessageBytes, effectiveLimit, config.SIPUDPRecommendedMaxMessageBytes), "建议降低 sip.max_message_bytes（如 <= 1300）或切换 sip.transport=TCP", "先在预发下调报文上限并做回归，再上线", "docs/troubleshooting.md#35-sip-transport-配置错误")}
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

func (r *Runner) checkRTPTransportPlan(cfg config.NetworkConfig) []Item {
	const name = "rtp.transport_plan"
	rtp := cfg.RTP
	if !rtp.Enabled {
		return []Item{item(name, LevelInfo, "RTP 已禁用，跳过传输模式规划检查", "如需启用 RTP，请配置 transport 并重新自检", "启用前确认跨网链路策略", "")}
	}
	transport := strings.ToUpper(strings.TrimSpace(rtp.Transport))
	message := fmt.Sprintf("rtp.transport=%s udp_control_max_bytes=%d udp_catalog_max_bytes=%d effective_inline_budget_bytes=%d udp_request_parallelism_per_device=%d udp_callback_parallelism_per_peer=%d udp_bulk_parallelism_per_device=%d udp_small_request_max_wait_ms=%d inline_response_headroom_ratio=%.2f response_mode_policy=%s", transport, cfg.TransportTuning.UDPControlMaxBytes, cfg.TransportTuning.UDPCatalogMaxBytes, config.EffectiveInlineResponseBodyBudgetBytes(cfg), cfg.TransportTuning.UDPRequestParallelismPerDevice, cfg.TransportTuning.UDPCallbackParallelismPerPeer, cfg.TransportTuning.UDPBulkParallelismPerDevice, cfg.TransportTuning.UDPSmallRequestMaxWaitMS, cfg.TransportTuning.InlineResponseHeadroomRatio, config.EffectiveResponseModePolicyLabel(cfg))
	if transport == "TCP" {
		return []Item{item(name, LevelWarn, message+"；当前 rtp.transport=TCP 已可用于联调验证（受控发布）", "生产环境建议保持 rtp.transport=UDP，TCP 仅用于跨网调试与防丢包验证", "若非排障窗口，请切回 UDP 并记录变更", "docs/troubleshooting.md#39-transport-不匹配")}
	}
	return []Item{item(name, LevelInfo, message+"；当前 rtp.transport=UDP（生产默认）", "无需处理", "保持现网默认策略", "")}
}

func (r *Runner) checkTransportTuningConvergence(cfg config.NetworkConfig) []Item {
	const name = "transport_tuning.generic_download_balance"
	p := config.ConvergedGenericDownloadProfile(cfg.TransportTuning)
	message := fmt.Sprintf("generic_download configured payload=%d bitrate_bps=%d socket_buffer_bytes=%d reorder=%d loss=%d gap_timeout_ms=%d fec=%t/%d => effective payload=%d bitrate_bps=%d socket_buffer_bytes=%d reorder=%d loss=%d gap_timeout_ms=%d fec=%t/%d", cfg.TransportTuning.BoundaryRTPPayloadBytes, cfg.TransportTuning.GenericDownloadRTPBitrateBps, cfg.TransportTuning.GenericDownloadRTPSocketBufferBytes, cfg.TransportTuning.GenericDownloadRTPReorderWindowPackets, cfg.TransportTuning.GenericDownloadRTPLossTolerancePackets, cfg.TransportTuning.GenericDownloadRTPGapTimeoutMS, cfg.TransportTuning.GenericDownloadRTPFECEnabled || cfg.TransportTuning.GenericDownloadRTPFECGroupPackets > 1, cfg.TransportTuning.GenericDownloadRTPFECGroupPackets, p.PayloadBytes, p.BitrateBps, p.SocketBufferBytes, p.ReorderWindowPackets, p.LossTolerancePackets, p.GapTimeoutMS, p.FECEnabled, p.FECGroupPackets)
	if p.AggressiveConfig || p.FECForced {
		return []Item{item(name, LevelWarn, message+"；检测到大文件 RTP 下载策略过激，运行时会自动收口以减小发送/接收失衡与 HOL 阻塞", "建议先把配置文件直接收敛到 effective 值，避免现场出现‘配置一个值、运行另一个值’的口径分叉", "优先保持 payload<=1200、generic_download_rtp_bitrate_bps<=min(total/2,16Mbps)、reorder<=768、loss<=256、gap_timeout<=1200，并开启 FEC", "docs/troubleshooting.md#311-大文件rtp-发送接收失衡")}
	}
	return []Item{item(name, LevelInfo, message+"；当前 generic_download RTP 策略已处于收敛档", "无需处理", "继续关注 writer_block_ms、rtp_peak_pending、gap_fast_forward 与 fec_recovered", "")}
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
	if !netbind.SameBindAddress(cfg.SIP.ListenIP, cfg.RTP.ListenIP) {
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
		return []Item{item(name, LevelWarn, "未配置下游 HTTP 路由：当前处于协议层可启动、业务执行层未激活状态（仅跳过可达性检查）", "请加载最小 httpinvoke 路由配置以激活业务执行层", "确认业务所需 api_code 已完成模板映射并发布；完成后重启并复核 /api/selfcheck", "docs/troubleshooting.md#310-下游-http-未配置")}
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
