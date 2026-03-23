# GB28181 Phase 5：联调前收口

这轮补的是 UI 联调前最容易卡住的三块：

1. **REGISTER Digest 鉴权骨架**
   - `TunnelConfigPayload` 新增：
     - `register_auth_enabled`
     - `register_auth_username`
     - `register_auth_password`
     - `register_auth_realm`
     - `register_auth_algorithm`
   - 下级域发起 `REGISTER` 时，若收到 `401 + WWW-Authenticate`，会缓存 challenge，并在下一次注册时按 Digest 方式补 `Authorization`。
   - 上级域接收 `REGISTER` 时，若启用了注册鉴权，会回 `401` 并验证 Digest。

2. **Catalog 续订**
   - `TunnelConfigPayload` 新增：`catalog_subscribe_expires_sec`
   - `SUBSCRIBE` / `NOTIFY` / `REGISTER` 的 `Expires` 与 `Subscription-State` 不再写死 3600。
   - `GB28181TunnelService` 增加续订循环，订阅接近过期时会自动补发 `SUBSCRIBE(Catalog)`。

3. **联调状态接口**
   - 新增：`GET /api/tunnel/gb28181/state`
   - 返回内容包括：
     - 会话管理器状态（注册/心跳/重试）
     - 当前 GB28181 运行配置
     - 对端注册/订阅状态
     - 活跃 pending/inbound 会话
     - 目录资源总数与已暴露数量

## 这轮没有做的事

- 没有补完整的 SIP digest `stale=true` 重试策略
- 没有做 `REGISTER` / `SUBSCRIBE` 的完整 dialog 层事务机
- 没有改 RTP payload 封装格式，仍保持当前 RTP-like chunk 回传
- 没有处理 UI 静态资源缺失导致的编译问题（`internal/server/ui_embedded.go` 依赖 `embedded-ui/*`）

## 适合 UI 联调的接口

- `GET /api/tunnel/config`
- `POST /api/tunnel/config`
- `GET /api/tunnel/catalog`
- `GET /api/tunnel/gb28181/state`
- `POST /api/tunnel/session/actions`
