# 第三批整改：非现场验证方案与 RTP 乱序/轻微丢包容忍说明

## 1. 这批可以在非现场先验证什么

并不是所有内容都必须到现场才能验证。下面这些项可以先在实验室、预发或 CI 环境完成：

### 1.1 配置有效性与默认值
- 验证默认端口规划：
  - SIP：`5060`
  - RTP：`20000-20999`
  - 节点映射：`21000-21999`
  - UI：`18080`
- 验证 `transport_tuning` 新增参数具备默认值、边界检查和中文注释。
- 验证 INLINE 预算和 RTP 缓冲预算的计算公式输出正确。

### 1.2 代码级单元测试
- `response_mode_decision`：小响应仍走 INLINE；大响应仍走 RTP。
- `rtp_reorder`：8 包窗口内乱序可恢复；超出窗口但落在 `loss_tolerance` 内时先缓存等待；超过总容忍后再触发恢复。
- `mapping_runtime`：`rtp pending gap timeout`、`unexpected eof` 等错误可以被归类为可恢复。

### 1.3 可控网络劣化试验
可直接使用仓库内已有脚本：
- `scripts/netem/run.sh`
- `gateway-server/tests/netem/matrix.json`

建议新增或手工追加以下场景：
1. `rtp-udp-reorder-window-8-loss-2`
   - `loss_percent=0.5`
   - `reorder_percent=10`
2. `rtp-udp-reorder-window-12-loss-4`
   - `loss_percent=1`
   - `reorder_percent=20`
3. `rtp-udp-gap-timeout-mp4`
   - `loss_percent=0.2`
   - `delay_ms=120`
   - `jitter_ms=40`

### 1.4 Web MP4 播放专项验证
主场景是 Web 页面播放 HTTP MP4，可在非现场先做：
- 浏览器发起首个 `GET /video.mp4`
- 浏览器后续发 `Range: bytes=...`
- 观察日志中是否出现：
  - `stage=segment_plan`
  - `stage=resume_plan`
  - `resume_reason=rtp_gap_timeout|rtp_sequence_gap|unexpected_eof`
- 成功标准：
  - 页面不是整段报错退出
  - 允许轻微卡顿后自动续传恢复
  - 不出现超窗 copy

## 2. 哪些必须现场验证

以下项仍必须现场验证：
- 安全边界设备是否对 UDP 分片、迟到包、Range 请求有额外处理。
- 实际海康/大华/SRS 对端组合下的 RTP/SIP 交互差异。
- 真实专网带宽、QOS、状态防火墙、NAT/会话老化对长流的影响。

## 3. 新增配置项说明（现场尽量只改这三项）

### 3.1 `boundary_rtp_reorder_window_packets`
- 含义：允许缓存并等待恢复的乱序包窗口。
- 默认：`8`
- 建议范围：
  - 一般专网：`8`
  - 弱网明显：`12`
  - 极端弱网：`16`
- 风险：越大，等待迟到包的时间和内存占用越高。

### 3.2 `boundary_rtp_loss_tolerance_packets`
- 含义：在 `reorder_window` 之外，再额外给多少个包的等待空间。
- 默认：`2`
- 建议范围：
  - 一般专网：`2`
  - Web MP4 主场景：`4`
- 注意：这不是“允许丢字节”，而是“允许等待迟到包，再不行就触发恢复”。

### 3.3 `boundary_rtp_gap_timeout_ms`
- 含义：出现缺口后，最多等待迟到包多久；超过后触发恢复。
- 默认：`1500`
- 建议范围：
  - 一般专网：`1200-1500`
  - 弱网 + MP4：`1800-2500`

## 4. 计算口径

以默认值为例：
- `boundary_rtp_payload_bytes = 640`
- `boundary_rtp_reorder_window_packets = 8`
- `boundary_rtp_loss_tolerance_packets = 2`

则接收侧的 RTP 缓冲预算约为：

`(8 + 2) * 640 = 6400 bytes`

这个预算只用于：
- 等待乱序包
- 等待少量迟到包
- 给恢复逻辑争取时间

不用于：
- 静默丢弃数据并继续拼 HTTP 文件

## 5. 现场建议的最少改法

现场人员能力有限时，优先只改以下三项，其他保持默认：

```yaml
transport_tuning:
  boundary_rtp_reorder_window_packets: 12
  boundary_rtp_loss_tolerance_packets: 4
  boundary_rtp_gap_timeout_ms: 2000
```

适用于：
- Web 页面播放 HTTP MP4
- 文件较大（数百 MB）
- 偶发 1~2 个 RTP 包丢失/迟到导致播放中断

## 6. 判定改进到位的证据

### 非现场
- 单元测试通过
- `netem` 报告中轻微 `loss/reorder` 下成功率提升
- `resume_reason=rtp_gap_timeout` 能触发恢复而不是整段硬失败

### 现场
- 3 路以上不再立即卡壳
- Web MP4 播放从“少量丢包即中断”变成“轻微卡顿后恢复”
- 日志中不再出现超窗 copy
