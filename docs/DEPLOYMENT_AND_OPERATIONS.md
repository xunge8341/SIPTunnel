# 隧道控制台部署与运维操作说明

本文将 **部署**、**服务安装**、**首次启动**、**升级回滚**、**运维检查**、**常见操作** 合并在一个文档中，避免说明分散。

## 1. 交付内容

发布包建议包含：

- `bin/windows/amd64/gateway.exe`
- `bin/linux/amd64/gateway`
- `bin/linux/arm64/gateway`
- `configs/config.yaml`
- `deploy/scripts/install.sh`
- `deploy/scripts/uninstall-linux-service.sh`
- `deploy/scripts/install-windows-service.ps1`
- `deploy/scripts/uninstall-windows-service.ps1`
- 本文档 `DEPLOYMENT_AND_OPERATIONS.md`

## 2. 首次启动会自动生成的文件

当以下文件不存在时，程序首次启动会自动生成：

- 配置文件：`configs/config.yaml`
- SQLite 数据库：`data/final/gateway.db`
- 系统设置：`data/final/system_settings.json`
- 安全设置：`data/final/security_settings.json`
- 保护策略：`data/final/protection_settings.json`
- 首次 7 天试用授权：`data/final/.license.lic`
- 授权信息：`data/final/.license_info.json`
- 试用发放标记：`data/final/.trial-issued.json`

> 注意：试用授权只会自动发放一次。删除授权文件不会再次自动生成试用授权。

## 3. Linux 服务安装（systemd）

### 3.1 安装

```bash
RELEASE_FILE=./dist/bin/linux/amd64/gateway \
INSTALL_DIR=/opt/siptunnel \
DATA_DIR=/var/lib/siptunnel \
CONFIG_PATH=/opt/siptunnel/config.yaml \
./deploy/scripts/install.sh
```

说明：

- 自动创建用户/组：`siptunnel`
- 自动安装二进制与配置模板
- 自动写入 systemd unit
- 自动启动并设置开机自启

### 3.2 卸载服务

```bash
SERVICE_NAME=siptunnel-gateway ./deploy/scripts/uninstall-linux-service.sh
```

### 3.3 常用运维命令

```bash
systemctl status siptunnel-gateway --no-pager
systemctl restart siptunnel-gateway
journalctl -u siptunnel-gateway -f
```

## 4. Windows 服务安装

### 4.1 安装为服务

```powershell
Set-Location C:\SIPTunnel
.\deploy\scripts\install-windows-service.ps1 `
  -ServiceName SIPTunnelGateway `
  -InstallDir C:\SIPTunnel `
  -BinaryPath C:\SIPTunnel\gateway.exe `
  -ConfigPath C:\SIPTunnel\configs\config.yaml `
  -DataDir C:\SIPTunnel\data `
  -StartAfterInstall
```

### 4.2 卸载服务

```powershell
.\deploy\scripts\uninstall-windows-service.ps1 -ServiceName SIPTunnelGateway
```

### 4.3 常用运维命令

```powershell
Get-Service SIPTunnelGateway
Restart-Service SIPTunnelGateway
Get-Content .\data\final\logs\gateway.log -Tail 200
```

## 5. 配置变更与生效说明

以下配置修改后 **必须重启服务** 才会生效：

- SIP 监听地址 / 端口 / 传输协议
- RTP 端口范围 / 传输协议
- 节点连接发起方
- 会话网络能力模式
- 对端节点地址 / 端口
- 管理面访问控制（CIDR / MFA）

建议流程：

1. 在 UI 完成配置修改
2. 点击保存并应用
3. 根据提示执行服务重启
4. 回到“节点与级联”页面确认：
   - 注册状态
   - 心跳状态
   - 下一次重试时间
   - 最近失败原因

## 6. 运行时运维检查

### 6.1 节点与级联

重点检查：

- 连接发起方是否正确（本端主动 / 对端主动）
- 会话网络能力模式是否符合当前网络边界
- 注册状态、心跳状态是否正常
- 下一次重试时间是否持续推进
- 对端可达性测试是否通过

### 6.2 隧道映射

重点检查：

- 映射请求是否真正经对端中转
- 对端日志是否能看到目标访问记录
- 映射测试失败原因是否明确
- 本端无法直连目标时，映射是否仍能成功（这是是否真走隧道的关键验证）

### 6.3 访问日志 / 告警与保护 / 安全事件

重点检查：

- 访问日志是否记录成功与失败请求
- “仅失败请求”筛选是否能工作
- 告警与保护是否基于真实访问日志统计
- SIP/RTP 协议拒绝事件是否进入“安全事件”页面
- 安全事件是否同时可从统一审计中查询

## 7. 升级与回滚

### Linux

```bash
RELEASE_FILE=./dist/bin/linux/amd64/gateway ./deploy/scripts/upgrade.sh
./deploy/scripts/rollback.sh
```

### Windows

建议流程：

1. 停止服务
2. 备份旧 `gateway.exe` 与 `configs/`
3. 替换新版本
4. 启动服务
5. 回归检查节点与级联、本地资源、隧道映射、链路监控、日志、授权、安全事件

## 8. 常见故障排查

### 8.1 注册失败但没有自动恢复

检查：

- 当前连接发起方是否正确
- 对端地址 / 端口是否与 UI 配置一致
- 下一次重试时间是否有值
- 服务重启后监听端口是否确实变更

### 8.2 对端可达性测试提示 multiple enabled peer nodes configured

当前设计要求：

- 节点与级联页保存时，当前选中的对端会被保留为启用
- 其他 peer 会自动禁用

若仍出现此报错：

- 检查旧配置文件中是否残留多个启用 peer
- 保存一次节点与级联配置后再重试

### 8.3 删除授权文件后仍希望重新自动发放试用授权

当前已禁止：

- 首次发放试用授权后会写入 `.trial-issued.json`
- 即使删除 `.license.lic`，也不会再自动生成试用授权

### 8.4 映射请求似乎没走隧道

验证方法：

1. 让本端无法直连目标地址
2. 只允许对端访问目标地址
3. 发起映射请求
4. 若仍成功且对端日志有目标访问记录，则说明走了隧道

## 9. 发布包建议同步带出的文件

建议在 release 包中统一附带：

- 本文档 `DEPLOYMENT_AND_OPERATIONS.md`
- `deploy/scripts/install.sh`
- `deploy/scripts/uninstall-linux-service.sh`
- `deploy/scripts/install-windows-service.ps1`
- `deploy/scripts/uninstall-windows-service.ps1`
- `configs/config.default.example.yaml`

这样部署和运维就不需要再去多个文档中来回翻。
