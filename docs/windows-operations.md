# Windows 打包与运维手册

本文面向 PowerShell/CMD 运维同学，覆盖 SIPTunnel 在 Windows 的交付目录、首次启动、常见排障和服务托管预留。

## 1. 推荐交付目录结构

```text
SIPTunnel/
├─ gateway.exe
├─ configs/
│  └─ config.yaml
├─ data/
├─ logs/
├─ docs/
│  ├─ README.md
│  └─ windows-operations.md
├─ scripts/
│  └─ service-skeleton.ps1
└─ start-gateway.ps1
```

说明：
- `configs/`：配置文件。
- `data/`：运行时数据（temp/final/audit/logs 子目录会自动创建）。
- `logs/`：额外日志落盘目录（若你用外部日志采集，也可保留为空）。
- `docs/`：交付文档与排障命令。

## 2. 首次启动（PowerShell）

```powershell
Set-Location C:\SIPTunnel
.\gateway.exe --config .\configs\config.yaml
```

如果未显式传 `--config`，程序会按优先级查找：`CLI > GATEWAY_CONFIG > exe目录\configs\config.yaml > exe目录\config.yaml > 当前目录`。

首启端口策略：未设置 `GATEWAY_PORT` 时，Windows dev/test 模式优先尝试 `18180`，若占用则自动回退到 `18080/18081/8080`。

如果找不到配置，将自动生成默认配置并创建所需目录（dev/test 模式）。

> 首启优化（已修复）：Windows 自动生成配置时，会优先选择可用的 SIP 端口（按 `5060 -> 15060 -> 25060 -> 35060` 顺序探测），避免大量机器上首启即因 `5060` 被占用而被 self-check 拦截。

推荐首启步骤（避免你截图里那类问题）：

```powershell
# 1) 进入安装目录（非常关键）
Set-Location C:\SIPTunnel

# 2) 主动生成配置（若已存在则不会覆盖）
.\gateway.exe init-config --config .\configs\config.yaml

# 3) 校验配置
.\gateway.exe validate-config -f .\configs\config.yaml

# 4) 启动
.\gateway.exe --config .\configs\config.yaml
```

若仍提示 SIP 端口占用，请改 `configs\config.yaml` 的 `sip.listen_port` 为其他空闲端口（如 `15060`），再重启。

## 3. 配置修改

1. 编辑 `configs\config.yaml`。
2. 先执行：

```powershell
.\gateway.exe validate-config -f .\configs\config.yaml
```


> 说明：`init-config` / `print-default-config` / `validate-config` 都是纯工具命令，执行后会直接退出，不会进入完整启动流程（不会加载网络服务、不会初始化 SIP/RTP、不会执行 environment self-check）。即使命令参数中前置了 `--config` 等启动参数，也会优先识别工具命令并直接退出。

3. 再重启服务。

## 4. 端口排查

### PowerShell

```powershell
Get-NetTCPConnection -LocalPort 18080 | Select-Object LocalAddress,LocalPort,State,OwningProcess
Get-Process -Id <PID> | Format-Table Id,ProcessName,Path
```

### CMD

```cmd
netstat -ano | findstr :18080
tasklist /FI "PID eq <PID>"
```

## 5. 服务托管建议（预留）

当前仓库提供 `scripts/service-skeleton.ps1` 作为骨架，用于快速演示 `New-Service` 安装方式：

```powershell
.\scripts\service-skeleton.ps1 -Action install -ServiceName SIPTunnelGateway -ConfigPath .\configs\config.yaml
```

> 说明：该脚本为预留骨架，不包含账号权限、故障恢复策略、日志轮转等生产参数，请按企业标准补全。

## 6. 常见错误

- **找不到配置文件**：确认工作目录是否在 exe 所在目录，或直接传 `--config .\configs\config.yaml`。
- **目录无写权限**：用管理员权限启动，或调整 `configs/data/logs` ACL。
- **端口占用**：使用上文 PowerShell/CMD 命令定位 PID 后释放端口，或修改监听端口。
- **快捷方式启动失败**：检查快捷方式“起始位置（Start in）”是否为安装目录。

## 7. 一键组包脚本

在仓库根目录运行：

```powershell
.\scripts\package-windows.ps1 -Version v0.1.0
```

输出：`dist/windows/SIPTunnel-v0.1.0-windows-amd64.zip`。
