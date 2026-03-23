# GB/T 28181 拟态改造（Phase 2 主流程接线）

这组修改把主链路从原先的单次 JSON 隧道，推进到“真 SIP 文本 + 会话内 XML + RTP 回传”的可接线骨架：

## 已接上的主流程

### 1. 双栈信令入口
- `cmd/gateway/main.go`
- `internal/server/gb28181_tunnel.go`

SIP TCP/UDP 收包后先尝试按真 SIP 文本报文解析：
- 能解析：走 GB28181 拟态路由
- 不能解析：回退到原有 JSON dispatcher

这样不会一次性打断现网 JSON 链路，但新主流程已经优先进入 SIP 文本栈。

### 2. 上级域 HTTP -> INVITE/INFO/BYE
- `internal/server/tunnel_relay.go`
- `internal/server/mapping_runtime.go`

当 `GB28181TunnelService` 已配置时，HTTP 映射转发优先改走：
1. `INVITE + application/sdp + Subject`
2. `ACK`
3. `INFO(HttpInvoke XML)`
4. 等待下级域主动回 `INFO(HttpResponseStart XML)`
5. `INLINE` 走会话内 `INFO(HttpResponseInline XML)`
6. `RTP` 走 UDP 分片回传
7. `BYE`

### 3. 下级域执行真实 HTTP 请求
收到 `INVITE` 后会按 `DeviceID` 建立入站会话；
收到 `INFO(HttpInvoke)` 后异步执行真实 HTTP 请求，并回推：
- `INFO(HttpResponseStart)`
- `INFO(HttpResponseInline)` 或 RTP body
- `BYE`

### 4. RTP chunk 回传
当前 RTP body 回传是 **Phase 2 的工程骨架**：
- SDP 中协商上级域 RTP 接收地址
- 下级域通过 UDP 发送分片
- 负载使用现有 `internal/rtp` 头格式承载 chunk

这一步已经满足“响应体优先走 RTP 回传”的主流程要求，但还没有做更强的媒体拟态封装。

## 同步并入的 Phase 1 基础层
- `internal/protocol/siptext`
- `internal/protocol/manscdp`
- `internal/server/catalog_registry.go`
- `internal/tunnelmapping/model.go`

## 目前仍是骨架、还没补满的点
1. `REGISTER / SUBSCRIBE / NOTIFY` 目前只接了基础 200 应答，目录订阅通知还没接到运行态同步。
2. `ACK` 目前按无响应发送，尚未补更完整的事务层状态。
3. RTP 仍是“chunk over UDP + 自定义 RTP-like 头”，还没升级成更强的 28181 媒体拟态。
4. 回调地址目前优先使用本地 SIP 监听地址；当节点配置里是 `0.0.0.0` 时，仍需要按部署环境补“对外可达地址”策略。
5. 响应体当前仍是整包读完后决定 INLINE/RTP；还没做真正的流式边读边发。

## 建议的下一步（Phase 3）
1. 把 `REGISTER / SUBSCRIBE / NOTIFY(Catalog)` 真正接到 `CatalogRegistry`
2. 给 SIP dialog/cseq/route-set 补更完整的事务管理
3. 把 RTP body 从当前 chunk 头提升成更像 28181 的媒体拟态封装
4. 补回调地址/外网可达地址协商
5. 做大响应流式转发与并发治理
