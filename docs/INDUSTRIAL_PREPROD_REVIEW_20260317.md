# 工业生产级 / 专网部署代码审查（2026-03-17）

本轮审查基于当前预发源码，重点从专网交付、工业现场可运维性、管理面安全、敏感信息保护四个方向检查。

## 已直接调整

- 放宽了几个 20 位国标编码输入框的可视宽度，避免桌面端录入和核对时被按钮挤压：
  - `gateway-ui/src/views/NodesAndTunnelsView.vue`
  - `gateway-ui/src/views/LocalResourcesView.vue`
  - `gateway-ui/src/views/TunnelMappingsView.vue`
  - `gateway-ui/src/views/NodeConfigView.vue`

## P0：建议在工业生产环境上线前完成

### 1. 隧道签名密钥存在内置默认值回退
- 文件：`gateway-server/internal/server/tunnel_relay.go`
- 现状：`GATEWAY_TUNNEL_SIGNER_SECRET` 为空时，会回退到固定值 `siptunnel-boundary-secret`。
- 风险：不同环境会共享同一默认签名口令，无法满足工业生产环境下的环境隔离与密钥轮换要求。
- 建议：生产模式下改为**缺失即启动失败**；测试环境可通过显式 `ALLOW_INSECURE_DEFAULT_SIGNER=true` 一类开关放行，但不能默认回退。

### 2. 管理面 CIDR 判断直接信任 `X-Forwarded-For`
- 文件：`gateway-server/internal/server/management_security.go`
- 现状：`requestClientIP()` 优先取 `X-Forwarded-For`，没有“可信代理”白名单或 `RemoteAddr` 约束。
- 风险：客户端可以自行伪造请求头，绕过 `admin_allow_cidr` 判断。
- 建议：
  1. 默认只信任 `RemoteAddr`；
  2. 仅当来源地址命中“可信反向代理 CIDR”时，才解析 `X-Forwarded-For`；
  3. 最好统一支持 `X-Real-IP` / RFC 7239 `Forwarded`，并写入审计日志。

### 3. 管理面当前仅 HTTP 明文监听，没有原生 TLS / mTLS 能力
- 文件：`gateway-server/cmd/gateway/main.go`
- 现状：管理面通过 `httpServer.ListenAndServe()` 启动，没有 `ListenAndServeTLS()` 或 `tls.Config`。
- 风险：在专网中虽然常由前置网关兜底，但对于工业现场跨网段、跨厂站访问，明文管理面仍然偏弱。
- 建议：至少提供两种模式：
  - 原生 TLS（证书文件 / 热更新）；
  - 由反向代理终止 TLS 时，服务端强制要求来自可信代理网段。

### 4. “管理面未加固”目前只做告警，不做 fail-close
- 文件：`gateway-server/internal/server/http.go`
- 现状：`/api/selfcheck`、链路检查等只提示“建议开启管理令牌 / CIDR / MFA”，服务本身仍可继续以未加固状态运行。
- 风险：生产环境误配置后可直接裸奔上线。
- 建议：增加 `run_mode=prod` 或 `deployment_profile=industrial` 下的启动闸门，至少检查：
  - `GATEWAY_ADMIN_TOKEN` 已配置；
  - `admin_allow_cidr` 非空且非全开放；
  - 若启用 MFA，则 `GATEWAY_ADMIN_MFA_CODE` 必须存在；
  - `GATEWAY_TUNNEL_SIGNER_SECRET` 不允许回退默认值。

### 5. 敏感配置仍可能以明文方式返回或落库
- 文件：
  - `gateway-server/internal/server/http.go`
  - `gateway-server/internal/persistence/sqlite_store.go`
  - `gateway-server/internal/server/secure_config.go`
- 现状：
  - `TunnelConfigPayload` 含 `register_auth_password`；
  - `/api/tunnel/config` 读接口直接返回整个配置结构；
  - `SaveSystemConfig()` 直接 `json.Marshal(payload)` 写入 SQLite，未走 `marshalSecureJSON()`。
- 风险：Digest 密码等敏感字段可能被前端、浏览器调试工具、SQLite 备份、故障包直接带出；而系统中虽然已经实现了 `secure_config.go`，但并未用于 `system_configs`。
- 建议：
  1. 将 `register_auth_password` 改为**只写字段**；读接口只返回 `register_auth_password_configured=true/false`；
  2. 更新接口增加 `replace_secret` 语义，避免“回读旧密码再带回去”；
  3. `system_configs` 写库统一改为加密封装，或至少对敏感 key 分类加密。

## P1：建议在交付前一并补强

### 6. 生产模板默认监听地址过宽
- 文件：`gateway-server/configs/*.yaml`
- 现状：多个模板默认使用 `0.0.0.0`。
- 风险：一旦部署人员未按现场网卡收敛，服务会暴露到所有接口。
- 建议：
  - 开发模板保留 `0.0.0.0`；
  - 生产模板改为显式占位符，如 `__REQUIRED_BIND_IP__`；
  - 自检中对 `0.0.0.0` 给出更高等级告警。

### 7. Windows 交付件仍容易触发 PowerShell 安全警告
- 文件：`scripts/*.ps1`
- 现状：用户侧执行压缩包中的脚本仍需 `Unblock-File`，会触发 MOTW / 执行确认。
- 风险：现场运维体验差，且容易让非熟悉 PowerShell 的实施人员误判为恶意脚本。
- 建议：
  - 发布流程里增加解压后自动 `Unblock-File` 指引；
  - 正式交付包考虑 Authenticode 签名；
  - 或提供单文件安装器 / MSI。

## P2：运维与交付体验优化

### 8. 管理面安全策略建议从“配置项”升级为“基线策略”
- 当前已有 `admin_allow_cidr`、`admin_require_mfa`、`GATEWAY_ADMIN_TOKEN` 等能力，基础不错。
- 下一步建议增加：
  - 管理令牌轮换时间 / 最近轮换时间；
  - 连续鉴权失败熔断；
  - 管理面只允许跳板机 / 堡垒机来源；
  - 关键配置变更双人复核或变更窗口机制。

## 审查结论

当前版本已经具备专网交付雏形：有管理面鉴权、CIDR、MFA、pprof 守卫、自检和运维面板，这是明显加分项。

但如果按“工业生产级”标准评估，至少还有以下四项需要优先补齐后再正式落地：

1. 去掉隧道签名默认密钥回退；
2. 修正 `X-Forwarded-For` 信任边界；
3. 管理面增加 TLS / 可信代理约束，并在生产模式 fail-close；
4. 敏感配置读接口与 SQLite 落库改为脱敏 / 加密。
