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

## 建议流程

1. 按环境设置 `LISTEN_PORT`、`MEDIA_PORT_START`、`MEDIA_PORT_END`、`NODE_ROLE`。
2. 执行 preflight 确认监听端口/媒体端口范围/接收发送角色配置。
3. 执行构建脚本生成单文件二进制。
4. 按目标环境发布并使用系统服务管理器托管。
