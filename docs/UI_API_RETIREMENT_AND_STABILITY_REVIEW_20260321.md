# UI 能力清退与现场稳定性完善说明（2026-03-21）

## 这轮专项的目标

这轮不是继续堆新功能，而是从运维视角做两件更重要的事：

1. 把 UI 里未接线、未挂主路由、但仍保留在源码里的历史能力做一次清退，避免控制台给运维留下“似乎可用、实际 404”的假象。
2. 结合现场日志，针对“WEB 登录、VLC 播放、大文件下载、播放与下载并行”做一轮针对性稳态补强。

## 运维侧结论：这批 UI 能力应清退，不应继续补后台

这批页面/接口在当前工程里有三个共同特征：

- 不在主路由，不是当前生产控制台的主链路；
- 后台没有闭环 handler；
- 一旦重新挂到菜单里，真实模式会直接 404 或落到未定义语义。

因此，从运维角度，它们不适合作为“预留能力”继续长期保留。保留它们的成本包括：

- 让 UI/后台/API 文档长期存在双口径；
- 让联调和验收继续被“页面还在、能力未接线”误导；
- 让后续版本继续背负不必要的兼容与测试负担。

所以本轮按“兵贵精不在多”的原则，直接清退这批未接线能力，而不是继续补一套后台接口。

## 已清退的 UI 能力

### 已删除的页面

- `ConfigGovernanceView.vue`
- `ConfigTransferView.vue`
- `NetworkConfigView.vue`
- `NodeStatusView.vue`

### 已删除的测试

- `src/views/__tests__/NodeStatusView.spec.ts`
- `src/api/__tests__/gatewayConfigTransfer.spec.ts`

### 已删除的真实 API 方法

- 配置治理：`/config-governance`、`/config-governance/export`、`/config-governance/rollback`
- 配置传输：`/config/transfer/export`、`/config/transfer/import`、`/config/transfer/template`
- 网络配置：`/network/config`
- 诊断导出：`/diagnostics/exports`
- 部署模式：`/system/deployment-mode`

说明：`mockGateway.ts` 里的 mock-only 数据保留为开发夹具，不再参与生产路由与真实 API 面。

## 结合现场日志的核心判断

### 1. 8201 登录失败不是前端问题，而是控制面 UDP 请求超限

现场日志里，登录前的 GET `publickey` 请求是正常的；真正失败的是 POST `/api/gmvcs/uap/cas/login`：

- `request_body_bytes=205`
- `sip_bytes=1307`
- `limit=1300`
- `final_status=udp_request_control_oversize`

也就是说，8201 的登录失败并不是 UI 登录逻辑错了，而是 GB28181/UDP 控制面把登录 POST 包装成 SIP/MANSCDP 后，控制消息只超了 7 字节就被网关拒掉。

### 2. VLC 一类播放请求里，存在被误分到下载策略的情况

现场日志已经能看到：

- `client_range=true`
- 但 `range_playback=false`
- 最后走的是 `profile=generic_download`

这会导致播放器首包和播放过程被下载策略接管，而不是走更轻快的 playback 策略。

### 3. 四路下载慢、下载与播放互相拖累，主因仍是下载链不稳且过于激进

现场日志里，大文件下载反复出现：

- `profile=generic_download`
- `pending` 持续堆高
- `gap_tolerated` 持续累积
- 最终 `rtp sequence discontinuity beyond tolerance`
- 随后 `resume_plan / segment_restart`

这说明问题不是“带宽数字不够大”，而是下载链在 RTP 弱网下恢复成本高、窗口内抖动大、默认 aggressiveness 偏高。

## 本轮已落地的代码改动

### 1. 修正播放/下载分类边界

在 `traffic_profile_policy.go` 中重新梳理了 playback 判定：

- 真实客户端 `Range` 与内部 `Range` 不再混用同一事实；
- 内部分段 child 不再因为内部 `Range` 而误判成播放；
- 为 segmented playback 增加显式 `X-SIPTunnel-Playback-Intent`，让 playback 子请求保持语义不丢失；
- 对 `path=/` 且带客户端 `Range`、无 attachment 迹象的请求补充启发式识别，减少 VLC 类请求误落到 generic download。

### 2. 8201 登录的 UDP 控制面减负

在 `compactTunnelRequestHeaders` 里，对 UDP 请求控制面压缩时去掉了冗余的 `Content-Length` 镜像头。

登录请求的失败点只超 7 字节；这次减负的目标不是放宽约束，而是在不放大 UDP 控制面风险的前提下，让常见登录请求重新回到安全范围内。

同时，`request_control_oversize` 日志补了 `suggested_limit`，便于后续联调看到“差多少字节、建议上调到多少”。

### 3. 下载默认稳态收一档，优先保系统平稳

在默认 `TransportTuningConfig` 中，把下载侧的 aggressiveness 进一步下调：

- `udp_bulk_parallelism_per_device: 6 -> 4`
- `generic_download_segment_concurrency: 2 -> 1`
- `generic_download_total_bitrate_bps: 48Mbps -> 32Mbps`
- `generic_download_rtp_bitrate_bps: 12Mbps -> 8Mbps`

这样做的目的不是追求单下载峰值，而是让：

- 四路并发下载更平滑；
- 播放与下载并行时，播放不被下载挤死；
- RTP 弱网场景下，下载链的恢复成本更可控。

## 这轮之后的预期效果

### 对问题 1：浏览器四路文件下载慢、速度抖动

预期改善点：

- 不再让下载侧默认用更激进的并发和发送速率；
- 下载不再轻易抢占整机 bulk lane；
- 旧版本里“transfer_id 落成 target URL”的公平分享错桶问题，在新版本也应一起缓解。

### 对问题 2：VLC 单路播放慢、卡顿

预期改善点：

- 播放类 Range 请求更容易落回 playback 策略，而不是 generic download；
- 首包、持续播放和断续恢复的行为会更接近“播放优先”的策略目标。

### 对问题 3：播放和下载同时进行，彼此拖累

预期改善点：

- 下载默认 aggressiveness 下调；
- 播放和下载的分类边界更清楚；
- 下载链不会再那么容易把播放挤到 generic download 的资源竞争态里。

### 对问题 4：8201 登录页无法登录

预期改善点：

- 对这类小 POST 控制请求，控制面压缩后的 SIP 包更容易落回 1300 字节内；
- 登录不再因为 `udp_request_control_oversize` 这种网关前置拒绝而失败。

## 仍需继续关注的隐患

1. 老版本日志里还存在 `sip.listen_port_occupancy` 把本进程监听误报成 error 的噪音；新版本要继续确认这类误报是否真正消失。
2. Windows 上 `mapping-runtime` 曾自动关闭 HTTP keep-alives 作为 workaround；这会增加某些大文件/Range 链路的反复建连成本，需要继续观察。
3. 下载链即使窗口放大到 `1024/384/1800ms` 仍可能出现 `pending>1200` 的头部 gap 堆积；因此仅靠调大窗口并不能根治问题，后续仍应继续推进更早的 gap stall 收敛。

## 验收边界

本轮验收重点是：

- UI 运行面/API 面的一致性收口；
- 现场日志可对上的代码逻辑补强；
- 默认配置与生成模板的一致性。

未在当前环境内宣称完整发布构建通过。
