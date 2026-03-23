# SIPTunnel 运维 Runbook（值班可直接执行版）

> 目标：故障发生时，值班同学按步骤执行即可完成止血与恢复。每一步都给出“动作 + 判定 + 下一步”。

## 0. 使用前约定

- 服务名：`siptunnel-gateway.service`
- 默认本机地址：`http://127.0.0.1:18080`
- 常用日志命令：`journalctl -u siptunnel-gateway -n 200 --no-pager`
- 标准自检接口：`GET /api/selfcheck`
- 节点网络状态接口：`GET /api/node/network-status`

---

## 1) 服务启动 / 停止

### 1.1 启动

**动作**

```bash
sudo systemctl daemon-reload
sudo systemctl start siptunnel-gateway.service
sudo systemctl status --no-pager siptunnel-gateway.service
curl -fsS http://127.0.0.1:18080/healthz
```

**判定**

- `status` 显示 `active (running)`。
- `/healthz` 返回 `{"code":"OK"...}`。

**失败时下一步**

1. 立即查看日志：`journalctl -u siptunnel-gateway -n 200 --no-pager`。
2. 按本 Runbook 的「端口冲突」「transport 错误」「RTP 端口池」「TCP 连接异常」章节继续处理。

### 1.2 停止

**动作**

```bash
sudo systemctl stop siptunnel-gateway.service
sudo systemctl status --no-pager siptunnel-gateway.service
```

**判定**

- 状态变为 `inactive (dead)`。

**注意**

- 若是生产紧急止血，先在工单中记录停止原因和预计恢复时间，再执行。

---

## 2) 配置发布 / 回滚

### 2.1 配置发布（推荐走脚本）

**动作**

```bash
./deploy/scripts/precheck.sh all
./deploy/scripts/precheck.sh config validate
sudo cp gateway-server/configs/config.yaml /opt/siptunnel/config.yaml
sudo systemctl restart siptunnel-gateway.service
curl -fsS http://127.0.0.1:18080/api/selfcheck
```

**判定**

- 重启成功，且 `selfcheck` 的 `overall` 不为 `error`。

**失败时下一步**

1. 不要反复改配置试错，先执行回滚。
2. 将失败配置和日志打包给研发。

### 2.2 配置回滚

**动作（整包回滚）**

```bash
./deploy/scripts/rollback.sh
sudo systemctl status --no-pager siptunnel-gateway.service
curl -fsS http://127.0.0.1:18080/healthz
```

**动作（仅配置回滚）**

```bash
sudo cp /opt/siptunnel/config.yaml.bak /opt/siptunnel/config.yaml
sudo systemctl restart siptunnel-gateway.service
curl -fsS http://127.0.0.1:18080/api/selfcheck
```

**判定**

- 服务恢复健康，关键接口可用。

---

## 3) 链路自检（上线前 / 故障后必做）

### 3.1 最小自检动作

```bash
curl -fsS http://127.0.0.1:18080/healthz
curl -fsS http://127.0.0.1:18080/api/selfcheck
curl -fsS http://127.0.0.1:18080/api/node/network-status
```

### 3.2 判定标准

- `healthz` 必须 `OK`。
- `selfcheck.overall` 允许 `info/warn`，出现 `error` 必须阻断上线。
- `network-status` 关注：
  - `sip.current_connections` 非异常抖动。
  - `rtp_port_pool_used/rtp_port_pool_total` 不持续高位。
  - `rtp_port_alloc_fail_total` 不持续增长。

---

## 4) 端口冲突处理

### 4.1 典型现象

- 启动日志出现 `bind: address already in use`。
- `selfcheck` 出现 `sip.listen_port_occupancy` 或 `sip_rtp_port_conflict` 的 `error`。

### 4.2 处理动作

```bash
ss -lntup | rg ':5060|:18080|:20[0-9]{3}'
```

1. 找到冲突进程 PID。
2. 判定是否可停：
   - 可停：停止冲突进程，重启网关。
   - 不可停：调整 `sip.listen_port` 或 RTP 端口段，走配置发布流程。
3. 复核：

```bash
sudo systemctl restart siptunnel-gateway.service
curl -fsS http://127.0.0.1:18080/api/selfcheck
```

---

## 5) transport 错误排查（SIP / RTP）

### 5.1 典型现象

- 日志出现 `unsupported transport`、`sip.transport ... unsupported`。
- 链路可达但消息不通，且 transport 与对端不一致。

### 5.2 处理动作

1. 检查配置：`network.sip.transport` 仅允许 `TCP/UDP/TLS`；`network.rtp.transport` 按当前对接方案配置。
2. 校验配置：

```bash
cd gateway-server
go run ./cmd/gatewayctl config validate -f ./configs/config.yaml
```

3. 与对端确认 transport 约定（SIP 与 RTP 分别确认）。
4. 重启后复核：

```bash
curl -fsS http://127.0.0.1:18080/api/selfcheck
curl -fsS http://127.0.0.1:18080/api/node/network-status
```

---

## 6) RTP 端口池耗尽处理

### 6.1 典型现象

- 日志出现 `rtp port pool exhausted`。
- `rtp_port_alloc_fail_total` 持续增长。
- 文件任务失败率上升。

### 6.2 立即止血动作

1. 降低入口并发（限流或上游削峰）。
2. 扩大端口池（例如 `20000-20100` -> `20000-20300`）。
3. 重启服务释放残留占用。

```bash
curl -fsS http://127.0.0.1:18080/api/node/network-status
```

### 6.3 长期治理动作

- 以峰值并发倒推端口池容量（建议留 30% 余量）。
- 对 `rtp_port_pool_used/total` 和 `rtp_port_alloc_fail_total` 建立告警。

---

## 7) TCP 连接异常处理（SIP over TCP / RTP over TCP）

### 7.1 典型现象

- `read_timeout_total`、`write_timeout_total` 快速增长。
- `connection_error_total` 激增。
- 连接反复建立/断开。

### 7.2 处理动作

1. 查看状态：

```bash
curl -fsS http://127.0.0.1:18080/api/node/network-status
ss -tanp | rg '5060|20000|20100'
```

2. 检查是否出现大量 `TIME_WAIT/CLOSE_WAIT`。
3. 临时调参：
   - 增大 `idle_timeout_ms`
   - 打开/收紧 `tcp_keepalive_*`
   - 适当提高 `read_timeout_ms/write_timeout_ms`
4. 若对端异常（短连风暴/半开连接）：先限流，再通知对端修复。

### 7.3 升级条件

- 30 分钟内连接错误率持续不降，或已影响业务成功率（见值班手册阈值）时，升级研发介入。

---

## 8) 压测前准备（必须完成）

### 8.1 环境准备动作

1. 关闭非必要批任务，避免压测相互干扰。
2. 确认日志与审计目录容量充足：

```bash
df -h
df -i
```

3. 固化当前配置快照：保存 `config.yaml` 与 `httpinvoke_routes`。
4. 执行基线检查：

```bash
./deploy/scripts/precheck.sh all
curl -fsS http://127.0.0.1:18080/api/selfcheck
```

### 8.2 压测执行建议

- 使用仓库脚本：`./scripts/loadtest/run.sh`
- 压测期间每 5 分钟采样：`/api/node/network-status`
- 必采指标：成功率、P95、`rtp_port_pool_used`、TCP 超时计数、错误码分布。

### 8.3 压测结束动作

1. 导出诊断快照：

```bash
cd gateway-server
go run ./cmd/gatewayctl diag export --out ./diagnostics.json
```

2. 归档：配置、报告、日志、诊断包。
3. 恢复非压测配置并做一次健康检查。
