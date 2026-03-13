# deploy

用于存放 SIPTunnel 的部署清单、编排文件与发布脚本。

## 当前交付内容

- 跨平台预检查脚本：
  - Linux/macOS：`scripts/preflight.sh`
  - Windows：`scripts/preflight.ps1`
- 跨平台构建脚本（默认单文件产物）：
  - Linux/macOS：`scripts/build.sh`
  - Windows：`scripts/build.ps1`
- 部署与操作手册：`docs/operations.md`
- 生产交付脚本（Linux/systemd）：
  - 安装：`deploy/scripts/install.sh`
  - 升级：`deploy/scripts/upgrade.sh`
  - 回滚：`deploy/scripts/rollback.sh`
  - 安装前检查：`deploy/scripts/precheck.sh`
- systemd unit 模板：`deploy/systemd/siptunnel-gateway.service`

## 建议流程

1. 按环境设置 `LISTEN_PORT`、`MEDIA_PORT_START`、`MEDIA_PORT_END`、`NODE_ROLE`。
2. 执行 preflight 确认监听端口/媒体端口范围/接收发送角色配置。
3. 执行构建脚本生成单文件二进制。
4. 按目标环境发布并使用系统服务管理器托管。

## 生产脚本快速使用

### 1) 安装前检查

```bash
./deploy/scripts/precheck.sh all
./deploy/scripts/precheck.sh config validate
./deploy/scripts/precheck.sh env inspect
./deploy/scripts/precheck.sh storage check
./deploy/scripts/precheck.sh network check
```

### 2) 安装

```bash
RELEASE_FILE=./dist/gateway-linux-amd64 ./deploy/scripts/install.sh
```

### 3) 升级

```bash
RELEASE_FILE=./dist/gateway-linux-amd64 ./deploy/scripts/upgrade.sh
```

### 4) 回滚

```bash
./deploy/scripts/rollback.sh
```


## UI 部署模式

网关支持：

- `ui.mode=external`：前后端分离部署（默认）。
- `ui.mode=embedded`：将 `gateway-ui` 构建产物嵌入 `gateway-server`，单进程对外提供 `/api/*` 与 SPA 静态资源。

发布 embedded 包前建议执行：

```bash
./scripts/embed-ui.sh
./scripts/verify-embedded-ui.sh
```

