# SIPTunnel 部署与操作手册

本文档面向一线运维与交付人员，提供可重复执行的安装、升级、回滚与排障流程。


关联文档：
- Runbook（现场动作清单）：`docs/runbook.md`
- 值班手册（告警与升级）：`docs/oncall-handbook.md`
- 故障类型排查：`docs/troubleshooting.md`

## 1. 安装前检查（Precheck）

生产环境建议先执行统一预检查脚本，输出包含：
- `config validate`
- `env inspect`
- `storage check`
- `network check`

### 1.1 一次执行全部检查

```bash
./deploy/scripts/precheck.sh all
```

### 1.2 按检查项单独执行

```bash
./deploy/scripts/precheck.sh config validate
./deploy/scripts/precheck.sh env inspect
./deploy/scripts/precheck.sh storage check
./deploy/scripts/precheck.sh network check
```

### 1.3 检查项说明

- `config validate`：检查配置文件存在性、关键段落和基础字段完整性。
- `env inspect`：打印关键环境变量，方便变更单核对。
- `storage check`：检查安装目录和数据目录可创建、可写。
- `network check`：检查监听端口、媒体端口范围、节点角色、端口占用。

## 2. 安装步骤（Install）

> 适用：首次部署或重建节点。脚本默认幂等，重复执行会尽量复用已有用户、组与配置文件。

### 2.1 准备发布文件

先通过构建流程生成 Linux 可执行文件（示例）：

```bash
./scripts/build.sh
```

默认发布文件可使用 `./dist/gateway-linux-amd64`。

### 2.2 执行安装脚本

```bash
RELEASE_FILE=./dist/gateway-linux-amd64 ./deploy/scripts/install.sh
```

脚本将执行：
1. 预检查（可用 `SKIP_PREFLIGHT=true` 跳过）。
2. 创建系统用户/组（默认 `siptunnel`）。
3. 创建目录并安装二进制到 `/opt/siptunnel/gateway`。
4. 初始化配置文件（若不存在）到 `/opt/siptunnel/config.yaml`。
5. 生成并安装 systemd unit。
6. 启动并设置开机自启。

### 2.3 验证安装结果

```bash
systemctl status --no-pager siptunnel-gateway.service
curl -fsS http://127.0.0.1:18080/healthz
```

## 3. 升级步骤（Upgrade）

> 适用：平滑替换版本。升级脚本会先做备份，失败自动回滚。

### 3.1 执行升级

```bash
RELEASE_FILE=./dist/gateway-linux-amd64 ./deploy/scripts/upgrade.sh
```

升级动作：
1. 备份当前二进制到 `/opt/siptunnel/backups/`。
2. 停止服务并替换二进制。
3. 启动服务并执行健康检查。
4. 若健康检查失败，自动回滚到最近备份。

### 3.2 升级后核查

```bash
systemctl status --no-pager siptunnel-gateway.service
ls -lh /opt/siptunnel/backups
```

## 4. 回滚步骤（Rollback）

### 4.1 回滚到最近版本

```bash
./deploy/scripts/rollback.sh
```

### 4.2 指定备份文件回滚

```bash
TARGET_BACKUP=/opt/siptunnel/backups/gateway-20260101-120000.bak ./deploy/scripts/rollback.sh
```

回滚完成后建议立即检查：

```bash
systemctl status --no-pager siptunnel-gateway.service
curl -fsS http://127.0.0.1:18080/healthz
```

## 5. systemd unit 模板

仓库提供模板：`deploy/systemd/siptunnel-gateway.service`。

安装脚本会自动将模板渲染为：
- `/etc/systemd/system/siptunnel-gateway.service`（默认服务名）

如果手工修改 unit，请执行：

```bash
systemctl daemon-reload
systemctl restart siptunnel-gateway.service
```

## 6. 运行参数基线

| 参数 | 默认值 | 说明 |
| --- | --- | --- |
| `GATEWAY_PORT` | `18080` | HTTP 控制与运维接口监听端口 |
| `MEDIA_PORT_START` | `20000` | RTP 接收/发送端口范围起始 |
| `MEDIA_PORT_END` | `20100` | RTP 接收/发送端口范围结束 |
| `NODE_ROLE` | `receiver` | 节点职责（`receiver`/`sender`） |
| `GATEWAY_DATA_DIR` | `/var/lib/siptunnel` | 数据根目录 |

## 7. 常见失败场景

### 场景 1：安装时报权限错误（目录不可写）
- 现象：`storage check` 报目录创建失败或不可写。
- 处理：确认脚本以 root 执行，或修正 `INSTALL_DIR/DATA_DIR` 权限后重试。

### 场景 2：服务启动失败，日志提示端口占用
- 现象：`network check` 或 systemd 日志提示 `LISTEN_PORT` 被占用。
- 处理：先执行诊断命令定位占用进程，再决定释放端口或变更 `GATEWAY_PORT`，最后 `systemctl restart`。
  - Linux：`ss -ltnp`、`lsof -i :<port>`
  - Windows：`Get-NetTCPConnection -LocalPort <port>`、`netstat -ano | findstr :<port>`、`tasklist /fi "PID eq <pid>"`

### 场景 3：升级后健康检查失败
- 现象：`upgrade.sh` 报错并触发自动回滚。
- 处理：
  1. 检查新版本配置兼容性。
  2. 查看 `journalctl -u siptunnel-gateway -n 200 --no-pager`。
  3. 修复后重新执行升级。

### 场景 4：配置校验失败
- 现象：`config validate` 提示缺失字段或加载失败。
- 处理：核对配置路径、YAML 格式与关键字段；可先恢复到上一个有效配置。

## 8. 运维操作建议

1. 所有变更先运行 `precheck.sh all` 并归档输出。
2. 升级前确认备份目录可用并记录变更单号。
3. 升级完成后保留至少一个可回滚版本。
4. 故障排查优先检查：端口占用、配置加载、目录权限、系统服务日志。
5. 针对一线值班排障，优先参考 `docs/troubleshooting.md`（按错误类型给出可执行动作）。


## 9. gateway-server 环境自检（面向运维）

### 9.1 自检覆盖项

启动阶段自动执行以下检查并输出统一报告：

1. `listen_ip` 存在性（SIP/RTP）
2. SIP 监听端口占用
3. RTP 端口范围合法性
4. SIP 与 RTP 端口冲突
5. `temp/final/audit` 目录可写性
6. 下游 HTTP 基础可达性（按 `target_host:target_port` 做 TCP 连通）

报告分级：`info / warn / error`，每项均包含 `message` 与 `suggestion`，适用于 API、CLI、日志三端复用。

当检测到监听端口冲突时，自检会增强输出：
- 明确冲突地址与 transport；
- 在可安全识别时附带 `进程名(pid=PID)`；
- Linux 给出 `ss -ltnp`、`lsof -i :<port>`；
- Windows 给出 `Get-NetTCPConnection -LocalPort <port>`、`netstat -ano | findstr :<port>`、`tasklist /fi "PID eq <pid>"`；
- 生产模式默认不自动改端口，仅输出诊断与人工调整建议；
- 开发模式可选输出建议空闲端口（`GATEWAY_SELFCHECK_SUGGEST_FREE_PORT=true` 或默认 dev/test）。

### 9.2 配置入口

网络配置文件按以下优先级查找（命中即停止）：

1. CLI 参数：`--config <path>`
2. 环境变量：`GATEWAY_CONFIG`
3. `exe_dir/configs/config.yaml`
4. `exe_dir/config.yaml`
5. `cwd/configs/config.yaml`
6. `cwd/config.yaml`

若以上路径都不存在，服务会明确记录“进入默认配置生成逻辑（default_generated）”并使用内置默认网络配置。

启动摘要（`startup summary` / `/api/startup-summary`）会额外展示：
- `run_mode`（dev/test/prod）
- `auto_generated_config`（是否首启自动生成配置）
- `config_candidates`（配置自动发现顺序，便于定位来源）

- `GATEWAY_HTTPINVOKE_CONFIG`：下游路由配置（YAML，含 routes 列表）。

示例（Linux）：

```bash
cd gateway-server
GATEWAY_CONFIG=./configs/config.yaml \
GATEWAY_HTTPINVOKE_CONFIG=./configs/httpinvoke_routes.example.yaml \
go run ./cmd/gateway
```

### 9.3 首启 smoke test 回归项

`opssmoke` 套件新增了“首启摘要”检查，要求 `/api/startup-summary` 至少返回 `run_mode/config_path/config_source`，用于保障首启可观测性不回退。

示例（显式 CLI，优先级最高）：

```bash
cd gateway-server
go run ./cmd/gateway --config ./configs/config.yaml
```

### 9.3 运维 API

服务启动后可通过：

```bash
curl -fsS http://127.0.0.1:18080/api/selfcheck
```

按级别筛选（支持逗号分隔多级别）：

```bash
curl -fsS 'http://127.0.0.1:18080/api/selfcheck?level=warn,error'
```

返回项字段统一为：`name`、`level`、`message`、`suggestion`、`action_hint`、`doc_link`（可选）。

### 9.4 看到某个自检项时怎么办（值班动作版）

> 原则：先执行 `action_hint`，再根据 `suggestion` 做配置修复，最后打开 `doc_link` 对照完整手册复盘。

1. **`sip.listen_port_occupancy`（端口冲突，error）**
   - 立即动作：在目标主机执行 `ss -ltnp` / `lsof -i :<port>` 找占用进程。
   - 建议动作：停止冲突进程或调整 `sip.listen_port`，重启后复核 `/api/selfcheck`。
2. **`sip.listen_ip` / `rtp.listen_ip`（0.0.0.0 通配地址，warn）**
   - 立即动作：确认这是临时联调还是生产配置。
   - 建议动作：生产建议替换为明确网卡 IP，避免错误网段接入。
3. **`downstream.http_base_reachability`（下游 HTTP 未配置或不可达，warn/error）**
   - 立即动作：核对 `httpinvoke` 路由配置是否有 `target_host/target_port`。
   - 建议动作：补齐 `api_code -> 下游地址` 映射并在网关主机发起连通性验证（`curl`/`telnet`）。
4. **`storage.*_dir_writable`（目录不可写，error）**
   - 立即动作：以服务用户执行 `touch` 验证写权限。
   - 建议动作：修复目录属主/权限并检查磁盘容量、inode 后重启服务。
5. **`rtp.transport_plan` / `sip_rtp_port_conflict`（transport 不匹配或冲突，warn/error）**
   - 立即动作：核对 SIP/RTP transport 是否符合当前环境（生产通常 RTP=UDP）。
   - 建议动作：按发布策略回归默认 transport，避免“临时排障配置”长期残留。

若启动阶段自检存在 `error` 级结果，进程会直接退出，避免带病上线。

## 10. SIP over TCP 生产运维补充

### 10.1 连接生命周期与参数建议

- 建连上限：通过 `network.sip.max_connections` 限制并发 TCP 会话，防止突发连接耗尽 FD。
- 空闲超时：`network.sip.idle_timeout_ms`，建议大于业务心跳周期。
- 读/写超时：`network.sip.read_timeout_ms` / `network.sip.write_timeout_ms`，超时会进入连接级计数器。
- Keepalive：`network.sip.tcp_keepalive_enabled=true`，并使用 `network.sip.tcp_keepalive_interval_ms` 加速僵尸连接回收。
- 套接字缓冲：`network.sip.tcp_read_buffer_bytes` / `network.sip.tcp_write_buffer_bytes`，大报文场景建议 >= 64KiB。

### 10.2 观测点与巡检

调用 `GET /api/node/network-status`，重点关注：

- `sip.current_connections`
- `sip.accepted_connections_total`
- `sip.closed_connections_total`
- `sip.read_timeout_total`
- `sip.write_timeout_total`
- `sip.connection_error_total`

排障建议：

1. `accepted_connections_total` 上升但 `current_connections` 长期为 0：检查对端短连策略或握手失败。
2. `read_timeout_total` 快速增长：检查链路抖动、上游发送节奏或空闲超时过短。
3. `write_timeout_total` 快速增长：检查下游接收阻塞和发送窗口。
4. `connection_error_total` 与日志中的 `connection_id/remote_addr/local_addr/transport=tcp` 关联定位异常连接。
