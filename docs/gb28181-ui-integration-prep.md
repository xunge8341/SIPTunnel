# GB28181 UI 联调前准备（Phase 6）

本轮在 Phase 5 基础上完成了两类工作：

1. **仓库内容补齐**：从 `SIPTunnel-main-ui-backend-aligned-v21.4-slim-updated-v7.zip` 恢复了顶层 `gateway-ui/`、`docs/`、`scripts/`、`deploy/`、`.github/`、`README.md` 等内容，避免当前分支只剩后端代码。
2. **联调前接口补齐**：后端继续保留 `GET/POST /api/tunnel/config`，并新增/稳定了：
   - `GET /api/tunnel/catalog`
   - `GET /api/tunnel/gb28181/state`

## UI 联调建议顺序

1. 先调用 `GET /api/tunnel/config` 获取 GB28181 参数：
   - `register_auth_enabled`
   - `register_auth_username`
   - `register_auth_password`
   - `register_auth_realm`
   - `register_auth_algorithm`
   - `catalog_subscribe_expires_sec`
2. 再调用 `GET /api/tunnel/catalog` 展示目录资源和自动暴露结果。
3. 最后调用 `GET /api/tunnel/gb28181/state` 展示会话、注册、订阅和 RTP 会话状态。

## 嵌入式 UI 说明

为了避免 `go:embed embedded-ui/* embedded-ui/assets/* embedded-ui/errors/*` 在未构建 UI 时直接编译失败，本轮补了一个最小占位的 `gateway-server/internal/server/embedded-ui/`。

正式联调前请执行：

```bash
./scripts/ui-build.sh
./scripts/embed-ui.sh
```

执行后会用真实前端构建产物覆盖占位文件。
