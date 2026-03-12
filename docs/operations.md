# SIPTunnel 部署与操作手册

本文档用于交付部署检查项，覆盖 Linux/macOS/Windows 环境。

## 1. 部署前检查（Preflight）

### 1.1 Bash（Linux/macOS）

```bash
LISTEN_PORT=18080 MEDIA_PORT_START=20000 MEDIA_PORT_END=20100 NODE_ROLE=receiver ./scripts/preflight.sh
```

### 1.2 PowerShell（Windows）

```powershell
$env:LISTEN_PORT = '18080'
$env:MEDIA_PORT_START = '20000'
$env:MEDIA_PORT_END = '20100'
$env:NODE_ROLE = 'receiver'
./scripts/preflight.ps1
```

### 1.3 检查项说明

- 监听端口 `LISTEN_PORT/GATEWAY_PORT`：必须为 `1~65535`，且未被占用。
- 流媒体端口范围 `MEDIA_PORT_START/MEDIA_PORT_END`：必须为 `1~65535`，且 `start <= end`。
- 节点角色 `NODE_ROLE`：仅允许 `receiver` 或 `sender`。

## 2. 默认单文件编译

### 2.1 Bash

```bash
./scripts/build.sh
```

产物位于 `dist/`，默认按宿主平台输出单可执行文件：
- Linux: `gateway-linux-amd64`
- macOS: `gateway-darwin-amd64`
- Windows: `gateway-windows-amd64.exe`

### 2.2 PowerShell

```powershell
./scripts/build.ps1
```

### 2.3 一次构建多平台（可选）

```bash
./scripts/build.sh matrix
```

```powershell
./scripts/build.ps1 -Mode matrix
```

## 3. 服务启动

- 临时启动（开发）：`cd gateway-server && go run ./cmd/gateway`
- 生产运行（推荐）：使用 `dist/` 中单文件二进制并通过系统服务管理器托管（systemd 或 NSSM/Windows Service）。

## 4. 运行参数基线

| 参数 | 默认值 | 说明 |
| --- | --- | --- |
| `GATEWAY_PORT` | `18080` | HTTP 控制与运维接口监听端口 |
| `MEDIA_PORT_START` | `20000` | RTP 接收/发送使用端口范围起始（部署规划参数） |
| `MEDIA_PORT_END` | `20100` | RTP 接收/发送使用端口范围结束（部署规划参数） |
| `NODE_ROLE` | `receiver` | 节点职责，`receiver`（接收端）/`sender`（发送端） |

> 说明：当前服务主进程已使用 `GATEWAY_PORT`，媒体端口范围与角色参数用于部署前检查与运行规划，便于跨环境统一运维口径。

## 5. 运维操作建议

1. 上线前执行 preflight，确保端口和角色配置合法。
2. 灰度期间将 `GATEWAY_PORT` 绑定到专用网段，并开放最小化访问策略。
3. 配置变更后执行 `go test ./...`，并留存审计记录。
4. 故障排查优先检查：端口占用、路由模板映射、重传状态和审计日志。
