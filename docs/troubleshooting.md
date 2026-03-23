# SIPTunnel 一线运维故障排查手册（Troubleshooting）

> 适用对象：值班运维 / SRE / 现场实施工程师。  
> 目标：遇到告警或报错后，**快速定位、快速止血、可复盘沉淀**。


配套文档：
- 日常操作 Runbook：`docs/runbook.md`
- 值班升级与阈值：`docs/oncall-handbook.md`

---

## 1. 使用方法（先做什么）

建议按以下顺序排查，避免“东一榔头西一棒子”：

1. **先看服务是否存活**：`systemctl status siptunnel-gateway -n 50`
2. **再看最近错误日志**：`journalctl -u siptunnel-gateway -n 200 --no-pager`
3. **看启动自检**：`curl -fsS http://127.0.0.1:18080/api/selfcheck`
4. **看网络与端口**：`ss -lntup | rg '5060|18080|20[0-9]{3}'`
5. **看配置文件版本/变更**：核对 `gateway-server/configs/config.yaml` 与变更记录

> 经验法则：先确认“能不能启动 + 端口是否可监听 + 依赖是否可达”，再做业务层细查。

---

## 2. 术语与信号速查

### 2.1 `result_code`（任务结果码）

`result_code` 主要用于描述下游 HTTP 执行结果，常见值：

- `OK`
- `UPSTREAM_TIMEOUT`
- `UPSTREAM_RATE_LIMIT`
- `UPSTREAM_SERVER_ERROR`
- `UPSTREAM_CLIENT_ERROR`
- `UPSTREAM_UNEXPECTED`

> 这些值用于快速判断“失败责任面”：网络超时、下游限流、下游 5xx、请求参数问题等。

### 2.2 自检分级

- `info`：正常信息
- `warn`：有风险，建议整改但未必阻断启动
- `error`：严重错误，通常会导致启动失败或功能不可用

---

## 3. 故障类型排查（可扩展模板）

> 每一类都按同一结构：**现象 → 可能原因 → 排查步骤 → 修复建议**。后续新增错误类型时可直接复制章节模板扩展。

---

### 3.1 `result_code` 异常（HTTP 调用结果异常）

#### 现象

- 任务状态失败，`result_code` 非 `OK`。
- 日志出现 `invoke upstream`、`UPSTREAM_TIMEOUT`、`UPSTREAM_SERVER_ERROR` 等关键字。

#### 可能原因

- 下游服务处理超时（业务慢、线程池满、数据库慢查询）。
- 下游触发限流（429）。
- 下游服务本身故障（5xx）。
- 请求参数映射错误导致 4xx。

#### 排查步骤

1. 在任务详情或数据库中确认 `result_code` 与 HTTP 状态码（若有）。
2. 按 `result_code` 分类定位：
   - `UPSTREAM_TIMEOUT`：优先检查网络延迟、下游耗时、`timeout_ms`。
   - `UPSTREAM_RATE_LIMIT`：核对下游限流阈值和当前 QPS。
   - `UPSTREAM_SERVER_ERROR`：查看下游服务日志（5xx 栈信息）。
   - `UPSTREAM_CLIENT_ERROR`：重点检查请求体字段映射、Header 映射、api_code 路由模板。
3. 回看 `httpinvoke` 路由配置（`target_host/target_port/http_path/http_method/retry_times/timeout_ms`）。

#### 修复建议

- 对超时：临时调大 `timeout_ms` + 下游扩容，长期做慢请求治理。
- 对限流：做流量削峰、重试退避或扩容下游限流桶。
- 对 4xx：修正映射模板，避免错误字段透传。
- 对 5xx：先恢复下游可用性，再补充熔断/降级策略。

---

### 3.2 配置校验错误

#### 现象

- 启动日志出现 `validate network config ... failed`。
- `config validate` 失败，提示字段缺失、格式错误或取值越界。

#### 可能原因

- YAML 语法错误（缩进、冒号、数组格式）。
- `sip.listen_port`、`rtp.port_start/end` 超出范围。
- `sip.listen_ip` / `rtp.listen_ip` 非法 IP。
- `sip.transport` / `rtp.transport` 填写了不支持值。

#### 排查步骤

1. 先做语法检查：
   - `python - <<'PY'\nimport yaml,sys\nyaml.safe_load(open('gateway-server/configs/config.yaml','r',encoding='utf-8'))\nprint('yaml ok')\nPY`
2. 执行预检：`./deploy/scripts/precheck.sh config validate`
3. 对照参数手册核对字段：`gateway-server/docs/generated/config-params.md`
4. 若近期变更较大，回滚到上一份已验证配置并做二分定位。

#### 修复建议

- 先恢复“可启动配置”，再逐项引入变更。
- 配置变更走双人复核（字段名、端口范围、transport）。
- 变更后固定执行 `go test ./...` 与启动自检。

---

### 3.3 端口冲突错误

#### 现象

- 启动失败，日志提示端口被占用（`bind: address already in use`）。
- 自检中出现 `sip.listen_port_occupancy` 或 `sip_rtp_port_conflict` 的 `error`。

#### 可能原因

- SIP 监听端口被其他进程占用。
- SIP 端口落入 RTP 端口池范围且 transport 相同。
- 同机多实例误用相同端口配置。

#### 排查步骤

> 先澄清：`validate-config` 通过仅代表配置字段合法；端口占用属于运行时检查，只会在真实启动（`gateway --config ...`）阶段暴露。

1. 查占用进程：`ss -lntup | rg ':5060|:18080|:20[0-9]{3}'`（Windows 可用 `Get-NetTCPConnection -LocalPort <port>`）。
2. 查配置冲突：确认 `sip.listen_port` 不在 `[rtp.port_start, rtp.port_end]`。
3. 查实例重复：`systemctl list-units | rg siptunnel`（Windows 同机多开可查任务管理器/服务列表）。

#### 修复建议

- 为同机实例分配独立 SIP/RTP 端口区间。
- 固定一套端口规划文档，避免现场口头分配。
- 修改配置后重启并立即执行 `/api/selfcheck` 复核。

---

### 3.4 目录不可写错误

#### 现象

- 启动失败，日志出现 `startup directory validation failed`。
- 自检中 `storage.*_dir_writable` 报 `error`。

#### 可能原因

- 目录不存在且进程无创建权限。
- 目录属主/权限不匹配运行用户。
- 磁盘已满或 inode 耗尽。

#### 排查步骤

1. 核对目录配置：`GATEWAY_DATA_DIR/GATEWAY_TEMP_DIR/GATEWAY_FINAL_DIR/GATEWAY_AUDIT_DIR`。
2. 检查权限与属主：`namei -om /path/to/dir`。
3. 检查容量：`df -h`、`df -i`。
4. 以运行用户做写入测试：`sudo -u <service_user> touch <dir>/.write_test`。

#### 修复建议

- 统一使用服务用户作为目录属主（例如 `chown -R siptunnel:siptunnel <data_dir>`）。
- 目录权限建议最小可用原则（常见 `750/755`）。
- 建立磁盘容量告警阈值（容量与 inode）。

---

### 3.5 SIP transport 配置错误

#### 现象

- 配置校验报错：`sip.transport "..." is unsupported`。
- 自检报错：`不支持的 SIP transport=...`。

#### 可能原因

- 填写了非 `TCP/UDP/TLS` 值。
- 手工编辑时大小写或拼写错误（如 `Tcp`、`udpv6`）。

#### 排查步骤

1. 打开网络配置检查 `network.sip.transport`。
2. 与变更单核对：是否误把 RTP transport 值复制到 SIP。
3. 若使用 UDP，额外核查 `sip.max_message_bytes` 是否过大（可能触发分片风险告警）。

#### 修复建议

- 生产优先 `TCP`（降低 UDP 分片风险）。
- 必须 UDP 时，将 `max_message_bytes` 控制在建议范围（如 `<=1300`）。
- 配置变更后执行自检并确认 `sip.listen_port_occupancy` 为 `info`。

---

### 3.6 RTP 端口池错误

#### 现象

- 启动失败：`init rtp port pool failed`（端口范围非法）。
- 运行期任务失败，出现 `rtp port pool exhausted`。
- 监控中 `rtp_port_alloc_fail_total` 持续增长。

#### 可能原因

- `rtp.port_start/end` 配置非法或范围过小。
- 并发传输高峰超过端口池容量。
- 传输结束后端口未及时释放（上游流量异常或任务状态滞留）。

#### 排查步骤

1. 核对配置合法性：`port_start <= port_end` 且都在 `1~65535`。
2. 查看 `/api/node/network-status` 中端口池使用率（`used/total`）。
3. 结合任务并发数与 `max_inflight_transfers` 判断是否容量不足。
4. 检查是否存在长时间未完成任务导致端口占用。

#### 修复建议

- 临时止血：扩大 RTP 端口范围、降低并发入口流量。
- 长期治理：按峰值并发容量规划端口池，建立 `alloc_fail_total` 告警。
- 排查异常任务释放链路，避免端口泄漏。

---

### 3.7 下游 HTTP 不可用错误

#### 现象

- 自检 `downstream.http_base_reachability` 为 `error`。
- 任务调用失败并伴随连接错误（拒绝连接、超时、无路由）。

#### 可能原因

- 下游服务未启动或端口未监听。
- 网络 ACL/防火墙策略阻断。
- `target_host/target_port` 配置错误。

#### 排查步骤

1. 从网关节点直连测试：`nc -vz <target_host> <target_port>`。
2. 查看路由配置中的目标地址是否正确。
3. 检查跨网段路由、ACL、安全组、防火墙策略变更。
4. 若节点容器化部署，确认容器网络策略与 DNS 解析。

#### 修复建议

- 修正路由配置并重载服务。
- 与网络团队确认放通策略（源/目的 IP、端口、协议）。
- 为关键下游配置健康检查和冗余实例。

---

### 3.8 通配地址 0.0.0.0 风险

#### 现象

- 自检出现 `sip.listen_ip` 或 `rtp.listen_ip` 的 `warn`，提示 `listen_ip=0.0.0.0`。

#### 可能原因

- 联调阶段使用通配地址，后续未回收。
- 希望“自动适配网卡”，但生产需要固定绑定业务网卡。

#### 排查步骤

1. 查看当前配置是否显式写为 `0.0.0.0`。
2. 在主机执行 `ip addr`（Linux）确认业务网卡 IP。
3. 核对安全边界策略，确认允许监听的网段。

#### 修复建议

- 生产环境建议绑定明确网卡 IP，减少误接入与排障复杂度。
- 若暂时保留 `0.0.0.0`，至少配套 ACL 与防火墙收敛暴露面。
- 调整后执行 `/api/selfcheck`，确认告警消失。

---

### 3.9 transport 不匹配

#### 现象

- 自检中 `rtp.transport_plan` 为 `warn`（例如 RTP=TCP）。
- 现场配置中 SIP/RTP transport 组合与发布基线不一致。

#### 可能原因

- 排障时临时切换 transport，变更后未恢复。
- 复制配置时将测试环境参数带入生产。

#### 排查步骤

1. 对照变更单确认目标 transport 组合。
2. 检查 `network.sip.transport` 与 `network.rtp.transport` 当前值。
3. 结合链路质量数据确认是否仍需要非默认 transport。

#### 修复建议

- 非排障窗口建议恢复发布基线（生产一般 RTP=UDP）。
- 将 transport 变更纳入变更审核，避免“长期临时配置”。

---

### 3.10 下游 HTTP 未配置

#### 现象

- 自检 `downstream.http_base_reachability` 为 `warn`，提示未配置路由。
- 业务请求到达网关后无法匹配下游执行目标。

#### 可能原因

- `httpinvoke` 配置文件未加载。
- 路由缺失 `target_host/target_port`。
- 新增 `api_code` 未同步下游模板映射。

#### 排查步骤

1. 检查 `GATEWAY_HTTPINVOKE_CONFIG` 指向文件是否存在。
2. 逐条检查路由是否包含 `api_code/target_host/target_port/http_path/http_method`。
3. 用 `curl -fsS http://127.0.0.1:18080/api/selfcheck` 复核告警是否消失。

#### 修复建议

- 补齐路由模板并重启服务。
- 变更后执行最小业务回归，确认关键 `api_code` 可正常下发。

---

## 4. FAQ（常见问题）

### Q1：看到 `warn` 级别是否必须立刻处理？
- 建议分级处理：
  - 与容量/稳定性相关（如 UDP 报文过大、RTP TCP 预留）建议尽快整改。
  - 纯提示类可纳入变更窗口处理。

### Q2：为什么配置看起来正确，但启动仍失败？
- 先确认不是**环境问题**：端口占用、目录权限、磁盘满、下游不可达。
- 再确认是否加载了预期配置文件（尤其是环境变量覆盖路径）。

### Q3：`UPSTREAM_CLIENT_ERROR` 一定是网关问题吗？
- 不一定。它表示 4xx，可能是网关映射错误，也可能是下游接口契约变更或鉴权策略变化。

### Q4：如何快速判断“是下游挂了”还是“网络不通”？
- 先看自检 `downstream.http_base_reachability`：
  - 若不可达，多为网络/目标未监听问题。
  - 若可达但仍大量 5xx，多半是下游业务故障。

### Q5：RTP 端口池经常打满怎么办？
- 三步并行：扩端口池、降并发、查未释放任务。
- 同时把 `rtp_port_pool_used/total`、`rtp_port_alloc_fail_total` 纳入容量告警。

### Q6：是否可以把 SIP 和 RTP 端口复用在同一端口？
- 不建议。即使部分场景传输层不同也“看起来能工作”，运维复杂度和风险会明显上升。

---

## 5. 附录：值班排障最小命令集

```bash
# 1) 服务状态
systemctl status siptunnel-gateway -n 50

# 2) 最近日志
journalctl -u siptunnel-gateway -n 200 --no-pager

# 3) 启动自检
curl -fsS http://127.0.0.1:18080/api/selfcheck

# 4) 端口监听
ss -lntup | rg '5060|18080|20[0-9]{3}'

# 5) 预检（部署脚本）
./deploy/scripts/precheck.sh all
```


## P0 稳态补充

- `GET /readyz`：检查本地监听、自检与映射运行态是否满足接流条件。
- `GET /healthz`：仅表示进程存活，不代表已满足业务接流条件。
- 现场排障建议顺序：`/healthz` → `/readyz` → `/api/selfcheck` → 链路监控页/诊断导出。


## Smoke 自检隔离说明

- `scripts/smoke.ps1` / `scripts/smoke.sh` 现在会为 smoke 运行生成临时配置，自动分配独立的 HTTP/SIP/RTP 端口，避免被本机常驻服务或旧进程污染。
- smoke 运行同时会设置独立的 `GATEWAY_DATA_DIR`，不再复用工作区 `data/` 下的历史 node/mapping 状态。
- 就绪等待不再只看 HTTP 200，而是要求 `/healthz` 与 `/readyz` 返回期望 JSON，避免把旧 UI 页面误判为健康服务。
