# Phase 22：健壮性增强与差距复盘

## 本轮重点

围绕“访问不可达地址、IP 变更、端口拒绝连接时程序容易被拖崩”的问题，本轮新增：

1. **上游 HTTP 失败分类**
   - 连接拒绝（connection refused / actively refused）
   - DNS 解析失败（no such host）
   - 网络不可达（network unreachable / no route to host）
   - 超时（timeout / deadline exceeded）
   - 连接中断（connection reset / broken pipe / EOF）

2. **短路退避（Circuit Breaker）**
   - 对同一 MappingID + TargetURL Host 的连续网络失败做指数退避。
   - 退避窗口内直接快速失败，不再持续轰炸同一不可达目标。
   - 适用于：目标服务未启动、端口未监听、IP 已变更、DNS 配置错误等场景。

3. **HTTP 服务恢复保护**
   - gateway 主 HTTP 服务增加 panic recovery 包装。
   - 本地映射监听服务增加 panic recovery 包装。
   - 主服务与映射监听服务补充 Read/Write/Idle/Header 等超时配置。

4. **RTP 资源退化策略**
   - AUTO 模式在 RTP 端口池耗尽时，自动退化为 INLINE-only，会话继续建立但不再抢占 RTP 端口。
   - 显式 `RTP` 模式仍保持严格语义，端口池耗尽时直接报错。

## 对照最初目标的完成度

### 已完成/基本对齐

- 下级域主动 `REGISTER`
- 上级域 `SUBSCRIBE/NOTIFY Catalog`
- 上级域本地监听 HTTP 映射端口
- 上级域按资源编码发 `INVITE`
- 会话内 `INFO(HttpInvoke XML)`
- 下级域执行真实 HTTP 请求
- 下级域先回 `HttpResponseStart`
- 小响应可 `INLINE`
- 大响应/流式响应可 `RTP`
- 会话末尾 `BYE`

### 本轮新增的稳定性能力

- 不可达 TargetURL 失败分类与更友好的错误消息
- 不可达目标自动退避，避免全链路资源被重复消耗
- 浏览器附带请求（如 `/favicon.ico`）抑制
- RTP 端口池紧张时的 AUTO 模式退化
- HTTP 服务 panic recovery / timeout 防护

### 仍存在差距（后续建议）

1. **RTP 负载仍是“字节块拟态”**
   - 现在可以工作，但离“更像 GB28181 媒体流”还有距离。

2. **事务/对话恢复策略仍偏轻量**
   - 缺少更强的 SIP 事务重试、丢包重发、CSeq/dialog 级别治理。

3. **Windows 下 net/http 崩溃仍需继续专项排查**
   - 本轮已经通过 recovery、超时、噪音抑制、退避减压来缓解。
   - 但若仍出现 `net/http.(*connReader).lock` 类 fault，需要继续结合 Go 版本、轮询频率、日志 IO 与连接复用做专项治理。

4. **UI 侧自检仍可继续增强**
   - 建议把“目标拒绝连接 / DNS 失败 / 退避中 / RTP 池紧张”拆成更明确的检查项与修复动作。

## 建议下一轮重点

1. 为 RTP 端口池增加 **实时占用监控 + 会话级清理告警**。
2. 为映射执行增加 **并发限流 / 单映射熔断状态展示**。
3. 对 Windows 运行态补做 **长稳压测 / 高频轮询 / 浏览器打开页面回归测试**。
4. 对 `INVITE / INFO / RTP / BYE` 增加更细粒度的时序自检结果。
