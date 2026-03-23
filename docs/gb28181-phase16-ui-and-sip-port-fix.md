# Phase 16：UI减负与SIP UDP信令口修正

本轮调整包含两类修复：

1. UI 与术语
- 链路监控改成网格布局，避免信息区块纵向堆叠。
- 节点与隧道页移除“本端类型 / 对端类型”选择，仅保留按服务器类型生成 20 位编码。
- 本地资源页将“资源类型”重命名为“编码类型（用于默认生成编码）”，并提供一键生成 20 位资源编码。
- 对 GET /api/link-monitor、/api/mappings 等高频轮询接口降低默认入站日志噪音。

2. SIP UDP 信令
- 之前 REGISTER / MESSAGE / SUBSCRIBE / NOTIFY / INVITE / INFO 在 UDP 模式下错误地使用了 RTP/临时端口发包，导致对端把响应发回临时口，引发 `read udp ... i/o timeout`。
- 现在改为复用本端实际 SIP UDP 监听 socket 发包与收包，并按 `remoteAddr + Call-ID + CSeq` 做事务匹配。
- 这样 REGISTER 等 SIP 控制面请求会真正走在两端约定的 SIP 信令端口上。

说明：
- 这轮对 Windows 上的 `net/http` fatal fault 没有做不负责任的“已彻底根治”承诺；当前代码侧已先收掉 SIP 端口错误与高频日志压力点，这两项是最明确、最该优先修的硬伤。
