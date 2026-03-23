# 网络劣化场景测试（SIP TCP / RTP UDP / RTP TCP）

本文档给出可复现的网络劣化测试框架、执行脚本和报告模板，覆盖以下劣化类型：

- 延迟（delay）
- 抖动（jitter）
- 丢包（loss）
- 乱序（reorder）
- 断连（disconnect）
- 带宽收缩（bandwidth shrink）

## 1. 目录说明

- 场景矩阵：`gateway-server/tests/netem/matrix.json`
- 报告模板：`gateway-server/tests/netem/report_template.md`
- 执行脚本：`scripts/netem/run.sh`
- 报告生成器：`gateway-server/cmd/netemreport`
- 指标聚合库：`gateway-server/internal/testutil/netdegrade`

## 2. 环境准备

> 推荐在 Linux 容器/虚机执行，且具备 `tc`、`iptables`、`go`。

```bash
# 可选：检查依赖
command -v tc
command -v iptables
command -v go
```

## 3. 配置场景矩阵

编辑 `gateway-server/tests/netem/matrix.json`，每个 `case` 可配置：

- `link`: `SIP_TCP` / `RTP_UDP` / `RTP_TCP`
- `interface`: 注入网卡（例如 `eth0`）
- `target_port` + `protocol`: 用于断连规则
- `condition`:
  - `delay_ms`
  - `jitter_ms`
  - `loss_percent`
  - `reorder_percent`
  - `disconnect_ms`
  - `bandwidth_kbps`

## 4. 执行自动化测试

### 4.1 无业务探针（先跑框架）

```bash
./scripts/netem/run.sh
```

该模式会输出占位样本，并在报告中提示需要手工验证。

### 4.2 接入业务探针（推荐）

将现有测试工具复用为 `NETEM_PROBE_COMMAND`，要求每次执行输出一行 JSON（JSONL）。字段如下：

- `link`
- `scenario`
- `condition`
- `attempts`
- `successes`
- `avg_latency_ms`
- `retransmissions`
- `recovery_time_ms`
- `manual_validation`（可选）

示例：

```bash
NETEM_PROBE_COMMAND='go test ./internal/service/filetransfer -run TestProbe -count=1 -v | ./scripts/netem/parse_probe.sh' \
./scripts/netem/run.sh
```

> 提示：探针命令由团队按现有容器/测试链路实现，框架不写死业务逻辑。

## 5. 手动验证步骤（不可自动化场景）

当链路需要人工确认时，在执行每个 case 后补充以下检查：

1. **SIP TCP**：抓取控制面会话，确认断连后连接重建与消息恢复。
2. **RTP UDP**：在丢包+乱序场景下确认补片/重组行为与最终文件一致性。
3. **RTP TCP**：在带宽收缩与断连下确认会话重连、传输恢复时间。
4. 从日志或指标系统提取：
   - 成功率
   - 平均时延
   - 重传率
   - 恢复时间
5. 将人工观察填入输出报告的“手动验证记录”部分。

## 6. 产物与复现

执行后输出在 `gateway-server/tests/netem/output/`：

- `samples.jsonl`：场景原始结果
- `report.md`：报告（由模板自动渲染）

复现实验最小步骤：

```bash
# 1) 按当前环境调整 matrix（网卡/端口）
vim gateway-server/tests/netem/matrix.json

# 2) 执行
./scripts/netem/run.sh

# 3) 查看报告
cat gateway-server/tests/netem/output/report.md
```
