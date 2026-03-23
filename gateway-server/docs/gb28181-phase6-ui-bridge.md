# Phase 6：UI/DOC/Script 回补与联调桥接

## 已完成

- 恢复仓库顶层 `gateway-ui/`、`docs/`、`scripts/`、`deploy/`、`.github/`
- UI TypeScript 合同增加：
  - `register_auth_*`
  - `catalog_subscribe_expires_sec`
  - `TunnelCatalogPayload`
  - `GB28181StatePayload`
- UI API 层增加：
  - `fetchTunnelCatalog()`
  - `fetchGB28181State()`
- Mock 数据增加目录快照和 GB28181 运行态样例
- 后端补最小 `embedded-ui/` 占位资源，避免未嵌入 UI 时 `go:embed` 直接编译失败

## 联调入口

- `GET /api/tunnel/config`
- `POST /api/tunnel/config`
- `GET /api/tunnel/catalog`
- `GET /api/tunnel/gb28181/state`
- `POST /api/tunnel/session/actions`

## 注意

占位 `embedded-ui/` 只用于保证后端源码在未嵌入 UI 时仍可构建。正式包发布前，应使用根目录脚本重新构建并嵌入真实 UI 产物。
