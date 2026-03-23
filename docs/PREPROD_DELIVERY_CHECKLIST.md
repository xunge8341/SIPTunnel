# 投产前交付清单（yyyy-MM-dd HH:mm:sss）

本清单用于专网/受控环境上线前的最终确认。当前代码已补齐：

- 管理面令牌认证（`GATEWAY_ADMIN_TOKEN`）
- 可选 MFA 校验（`GATEWAY_ADMIN_MFA_CODE`）
- 管理面 CIDR 收敛（`admin_allow_cidr`）
- 配置落盘加密（`GATEWAY_CONFIG_KEY`）
- 隧道签名密钥外置（`GATEWAY_TUNNEL_SIGNER_SECRET`）
- 三层保护运行态（全局 / 映射 / 来源 IP）
- 熔断显式状态与手工恢复
- 统一时间格式 `yyyy-MM-dd HH:mm:sss`（Go 等价 `2006-01-02 15:04:05.000`）

## 仍需由环境侧完成的事项

以下事项不是代码缺陷，但在专网投产前必须由交付/运维侧完成：

1. **环境变量注入与保管**
   - `GATEWAY_ADMIN_TOKEN`
   - `GATEWAY_ADMIN_MFA_CODE`（启用 MFA 时）
   - `GATEWAY_CONFIG_KEY`
   - `GATEWAY_TUNNEL_SIGNER_SECRET`

2. **管理网访问路径收敛**
   - 跳板机 / 反向代理 / 专用管理网卡
   - `admin_allow_cidr` 与实际来源地址一致

3. **时间同步**
   - 节点统一 NTP/北斗授时
   - 允许日志与审计使用 `yyyy-MM-dd HH:mm:sss` 统一格式

4. **专网联调验收**
   - 对端 SIP/GB28181 平台白名单
   - DNS/hosts 静态解析策略
   - 防火墙端口放通（SIP / RTP / HTTP 管理面）

5. **应急流程**
   - 熔断恢复、重启、诊断导出操作人清单
   - 审计导出与值班电话

## 建议上线前命令

```powershell
Get-ChildItem .\scripts\*.ps1 | Unblock-File
.\scripts\build-release.ps1
cd .\gateway-server
go test ./internal/server/...
```
