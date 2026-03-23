# 生产日志复盘与联调日志补强（2026-03-21）

## 本轮结论

1. RTP 弱网下的主要问题不是“窗口太小”本身，而是**头部小缺口长期不愈合时，接收侧只会继续缓存 future packets，直到 overflow 或 sequence discontinuity**。
2. 因为旧逻辑只在 **socket 读超时** 时判定 `rtp pending gap timeout`，所以当网络仍在持续来包时，会出现“pending 已经非常大，但仍不提前收敛”的现象。
3. 启动后运行态自检仍去做“能否再次 bind SIP 端口”的静态判定，会把**当前 gateway 自己已经持有的 5060/UDP**误报成 blocking error。
4. 启动阶段存在两份“网络/策略事实”：配置文件初值与 node_config 覆盖后的最终运行值。日志若只打前者，现场会看到 `response_mode_policy` 和最终摘要不一致。

## 本轮代码决策

### A. RTP 接收侧补“活跃乱序缺口超时”
- 继续保留原有 `ReadDeadline`/idle timeout 判定。
- 新增 **active gap stall** 判定：
  - 只要 reorder pending 已经存在；
  - 且 gap 持续时间超过 `policy.GapTimeout`；
  - 即使 socket 仍在持续收包，也会提前判定为 `rtp pending gap timeout ... while_receiving=true`。
- 目标：避免一个很小的 head gap 把事务拖到 2~3 分钟后才 overflow。

### B. RTP 汇总日志补 peak 指标
新增/补齐：
- `peak_pending`
- `peak_gap_packets`
- `max_gap_hold_ms`
- `rtp_gap_tolerated`
- `rtp_gap_timeouts`
- `rtp_fec_recovered`

这些指标进入：
- `gb28181 media stage=rtp_ps_summary`
- `gb28181 relay stage=transaction_summary`

### C. gap 日志降噪但保留关键阈值
`rtp_ps_reorder_buffered` / `rtp_ps_gap_tolerated` 改成：
- 首次必打；
- 早期少量必打；
- 之后按固定步长与阈值打点；
- 同时补 `gap_age_ms`。

### D. 运行态自检识别“当前进程已持有 SIP 端口”
- 新增 runtime self-check 输入位 `ExpectSIPPortOwnedByCurrentProcess`。
- 网关运行态 `/api/selfcheck` 与启动后自检都按“当前 gateway 已持有 SIP 监听端口”解释，不再把它报成 blocking error。
- 仍保留离线/预启动场景下的真实端口冲突检测能力。

### E. 启动日志补最终运行事实
新增：
- `startup runtime_network_effective ...`

用途：
- 在 node_config 覆盖后再次输出最终 `network_mode / sip_transport / sip_listen / rtp_range / response_mode_policy / effective_inline_budget_bytes`，避免现场把“配置初值日志”误读成“最终运行值”。

### F. response_mode_policy 对外标签统一
- 运行态 `response_mode_decision` 的 `response_mode_policy` 改为与启动摘要、自检、策略快照一致的外部标签，避免一个写 `safe_budget_auto`、另一个写 `AUTO(...)`。

## 验收边界

- 这些改动属于 **运行态自检 / 观测 / RTP 接收收敛逻辑** 补强。
- 本轮没有新增新的 transport tuning 参数；仍优先通过既有窗口/容忍/超时来推导行为，而不是再堆策略开关。
