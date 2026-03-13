# SIPTunnel 值班手册（On-call Handbook）

> 面向 7x24 值班同学：先止血、再定位、最后复盘。

## 1. 常见告警与首个动作

| 告警 | 典型信号 | 首个动作（1 分钟内） |
| --- | --- | --- |
| 服务不可用 | `/healthz` 非 200 或超时 | `systemctl status` + `journalctl -n 200` |
| 自检失败 | `/api/selfcheck` `overall=error` | 按错误项 suggestion 逐条执行 |
| 端口冲突 | `bind: address already in use` | `ss -lntup` 查冲突进程并释放/改端口 |
| RTP 端口池耗尽 | `rtp port pool exhausted` / `alloc_fail_total` 增长 | 降并发 + 扩端口池 + 重启 |
| TCP 连接异常 | `read/write timeout`、`connection_error_total` 上升 | 查 `network-status` + `ss -tanp` |
| 下游不可达 | `UPSTREAM_TIMEOUT`、拒绝连接 | `nc -vz target_host target_port` |

---

## 2. 标准排查顺序（必须按顺序）

1. **确认影响面**：单节点还是全链路（先看监控面板与告警聚合）。
2. **确认服务状态**：
   ```bash
   systemctl status --no-pager siptunnel-gateway.service
   ```
3. **确认健康与自检**：
   ```bash
   curl -fsS http://127.0.0.1:18080/healthz
   curl -fsS http://127.0.0.1:18080/api/selfcheck
   ```
4. **确认网络状态**：
   ```bash
   curl -fsS http://127.0.0.1:18080/api/node/network-status
   ```
5. **确认最近日志**：
   ```bash
   journalctl -u siptunnel-gateway -n 200 --no-pager
   ```
6. **执行止血动作**（降并发/回滚/重启/端口调整）。
7. **记录事件时间线**（首次告警、止血动作、恢复时间、待办项）。

---

## 3. 升级路径（Escalation）

### 3.1 L1（值班运维）

- 执行标准排查顺序。
- 可执行动作：重启、配置回滚、端口调整、限流、通知网络团队放通。
- 目标：10 分钟内完成首轮止血判断。

### 3.2 L2（SRE / 平台）

触发条件（任一满足）：

- L1 10 分钟内无法止血。
- 多节点同时异常。
- 需要跨团队网络策略变更。

动作：

- 协调流量切换/灰度回退。
- 校验主机资源、系统参数、网络策略。
- 指导 L1 执行临时绕行方案。

### 3.3 L3（研发）

触发条件见第 4 节阈值。升级时必须附带：

- `diagnostics.json`（`gatewayctl diag export`）
- 最近 200 行日志
- 当前配置与最近一次变更记录
- 影响面（请求量、失败率、受影响系统）

---

## 4. 需要研发介入的阈值

满足任一条即升级研发：

1. **服务不可用持续 > 10 分钟**，且回滚/重启无效。
2. **`rtp_port_alloc_fail_total` 连续 10 分钟增长**，并导致任务失败率 > 5%。
3. **`connection_error_total` 或 `read/write_timeout_total` 连续 15 分钟上升**，且成功率 < 99%。
4. **`/api/selfcheck` 出现无法通过运维动作消除的 `error`**（例如代码缺陷导致资源泄漏）。
5. **同类告警 24 小时内重复触发 ≥ 3 次**。
6. **疑似协议兼容问题**（transport 一致但仍无法收发，或升级后出现序列化/解析异常）。

---

## 5. 值班动作清单（复制即可执行）

### 5.1 一键采样（故障现场）

```bash
systemctl status --no-pager siptunnel-gateway.service
journalctl -u siptunnel-gateway -n 200 --no-pager
curl -fsS http://127.0.0.1:18080/healthz
curl -fsS http://127.0.0.1:18080/api/selfcheck
curl -fsS http://127.0.0.1:18080/api/node/network-status
ss -lntup | rg ':5060|:18080|:20[0-9]{3}'
```

### 5.2 导出诊断包（升级前）

```bash
cd gateway-server
go run ./cmd/gatewayctl diag export --out ./diagnostics.json
```

---

## 6. 文档导航（值班必读）

- Runbook：`docs/runbook.md`
- 故障类型手册：`docs/troubleshooting.md`
- 部署与发布：`docs/operations.md`
- 可观测告警基线：`docs/observability.md`
- 压测工具与方法：`docs/loadtest-toolkit.md`
