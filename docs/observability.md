# Observability 指南

本文档给出 SIPTunnel 在生产环境的监控落地基线：Prometheus 告警规则、Grafana 仪表盘和落地规范。

## 1. 资产目录

- Prometheus 告警：`deploy/observability/prometheus/alerts.yaml`
- Grafana Dashboard：`deploy/observability/grafana/siptunnel-ops-dashboard.json`

## 2. 告警命名与标签规范

### 2.1 告警命名

统一使用：`SIPTunnel<领域><语义>`，例如：`SIPTunnelTaskFailureRateHigh`。

### 2.2 统一标签

每条告警必须包含以下 labels：

- `severity`：`warning` / `critical`
- `team`：`siptunnel-ops`
- `service`：`siptunnel-gateway`
- `category`：`connectivity` / `task` / `traffic` / `media` / `transport` / `runtime` / `storage`

### 2.3 统一注解

每条告警都包含：

- `summary`：一句话说明
- `description`：包含阈值、窗口和排查方向
- `runbook_url`：落到本页对应锚点

## 3. 告警规则（高频场景）

| 告警 | 触发条件（示例表达式） | 建议动作 |
|---|---|---|
| `SIPTunnelConnectionErrorSpike` | `rate(siptunnel_sip_tcp_connection_errors_total[5m]) > 5` | 检查对端连通性、连接数上限、SIP TCP 读写超时。 |
| `SIPTunnelTaskFailureRateHigh` | `failed/total > 5%`（10m） | 核查失败码分布、模板路由、依赖接口质量。 |
| `SIPTunnelRateLimitHitHigh` | `rate_limit_hits/requests > 20%`（5m） | 判断是否突发流量，必要时调整 RPS/Burst。 |
| `SIPTunnelRTPPortAllocFailure` | `increase(siptunnel_rtp_port_alloc_fail_total[5m]) > 0` | 立即扩充 RTP 端口池并检查端口占用。 |
| `SIPTunnelTransportRecoveryFailed` | `increase(siptunnel_transport_recovery_failed_total[10m]) > 0` | 排查网络劣化与重连恢复机制。 |
| `SIPTunnelGoroutineGrowthAnomaly` | `max(go_goroutines)-min(go_goroutines) > 300`（15m） | 检查泄漏、死锁、阻塞 IO。 |
| `SIPTunnelDataDiskUsageHigh` | `/var/lib/siptunnel` 使用率 > 85%（15m） | 执行归档/清理策略，必要时扩容。 |

## 4. Grafana Dashboard 设计

Dashboard 名称：`SIPTunnel 运维高频指标`

核心面板：

1. **SIP TCP 面板**
   - 当前连接数
   - 连接错误速率
   - 读超时速率
2. **RTP UDP/TCP 面板**
   - 活跃传输数
   - 端口池占用
   - TCP 写错误速率
3. **任务面板**
   - 任务失败率
   - 处理中任务数
4. **文件传输面板**
   - 发送吞吐 / 接收吞吐
   - 活跃文件任务
5. **限流与错误面板**
   - 限流命中速率
   - 错误码速率分布
   - 传输恢复失败次数（10m）

> 设计原则：以“值班 5 分钟内判断健康”为目标，默认首页仅保留高频指标与处置所需上下文。

## 5. 告警 Runbook 锚点

### Alert: SIPTunnelConnectionErrorSpike
先看 SIP TCP 连接错误和读写超时是否同步抬升，再确认对端网络与连接池上限。

### Alert: SIPTunnelTaskFailureRateHigh
按任务类型聚合失败码，优先排查失败率突增的 api_code 与目标服务。

### Alert: SIPTunnelRateLimitHitHigh
核对近 5 分钟请求总量与突发峰值，确认是否需要临时扩容或调整限流策略。

### Alert: SIPTunnelRTPPortAllocFailure
检查 RTP 端口池剩余量、系统端口占用、是否存在僵尸会话未释放。

### Alert: SIPTunnelTransportRecoveryFailed
结合链路抖动日志与重试轨迹，判断是瞬时网络问题还是恢复逻辑异常。

### Alert: SIPTunnelGoroutineGrowthAnomaly
对照 pprof goroutine 与阻塞 profile，定位泄漏来源。

### Alert: SIPTunnelDataDiskUsageHigh
检查落盘目录中的任务文件与审计日志，按保留策略归档后清理。

## 6. 启用步骤

1. Prometheus 加载 `deploy/observability/prometheus/alerts.yaml`。
2. Grafana 导入 `deploy/observability/grafana/siptunnel-ops-dashboard.json`。
3. 确认 `job="siptunnel-gateway"` 与 `job="node-exporter"` 的抓取标签与本地环境一致。
4. 在预发做一次告警回放（连接异常、限流、端口池耗尽）后再进入生产。
