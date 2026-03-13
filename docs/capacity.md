# 容量评估与限流校准（运维可执行版）

本文给出一个**可执行、可解释**的容量评估方案：
- 输入：现有 `loadtest` 的 `summary.json` + 当前运行配置。
- 输出：推荐参数与当前参数对比（命令并发、文件并发、RTP 端口池、`max_connections`、限流阈值）。

> 目标是帮助运维快速做调参决策，不做复杂自动调参。

---

## 1. 一键生成建议

### 1.1 先跑压测

```bash
./scripts/loadtest/run.sh
```

压测产物默认在：`gateway-server/loadtest/results/<run_id>/summary.json`。

### 1.2 用容量脚本输出建议

```bash
./scripts/loadtest/capacity.sh \
  gateway-server/loadtest/results/<run_id>/summary.json \
  120 60 256 220 300 450
```

参数依次为：
1. summary 文件路径
2. 当前最大并发 command 数
3. 当前最大并发 file transfer 数
4. 当前 RTP 端口池大小
5. 当前 max_connections
6. 当前限流 RPS
7. 当前限流 burst

输出为 JSON，包含 `current` 与 `recommendation` 两部分，方便落库与展示。

---

## 2. 启发式规则（可解释）

容量评估由 `gateway-server/loadtest/capacity.go` 实现，核心规则如下：

1. **质量门槛**：优先采用成功率高且延迟可控的数据。
   - 命令链路参考阈值：`P95 <= 200ms`
   - 文件链路参考阈值：`P95 <= 300ms`
2. **推荐并发**：
   - `推荐并发 = 压测并发 × 质量因子 × 0.85`（留 15% 生产余量）
3. **RTP 端口池**：
   - 按 `文件并发 × 2 个端口 × 1.3 缓冲` 计算，且最小值 64
4. **max_connections**：
   - 按 `（命令并发 + 文件并发）× 1.2` 估算
5. **限流阈值**：
   - `RPS = (命令吞吐 + 文件吞吐) × 0.85`
   - `burst = 1.5 × RPS`

> 这组规则偏保守，优先稳定性与可恢复性。

---

## 3. 管理 API：当前配置 vs 推荐配置

新增 API：`POST /api/capacity/recommendation`

请求示例：

```json
{
  "summary_file": "/data/loadtest/summary.json",
  "current": {
    "command_max_concurrent": 120,
    "file_transfer_max_concurrent": 60,
    "rtp_port_pool_size": 256,
    "max_connections": 220,
    "rate_limit_rps": 300,
    "rate_limit_burst": 450
  }
}
```

返回示例（节选）：

```json
{
  "code": "OK",
  "message": "success",
  "data": {
    "assessment": {
      "current": {"...": "..."},
      "recommendation": {
        "recommended_command_max_concurrent": 98,
        "recommended_file_transfer_max_concurrent": 46,
        "recommended_rtp_port_pool_size": 120,
        "recommended_max_connections": 173,
        "recommended_rate_limit_rps": 255,
        "recommended_rate_limit_burst": 383,
        "basis": ["...可解释规则..."]
      }
    }
  }
}
```

该接口可直接用于管理后台展示“当前值 vs 推荐值”。

---

## 4. 运维调参建议

1. 先按推荐值下调到稳态区间，观察 30~60 分钟。
2. 关注以下告警：
   - `rtp_port_alloc_fail_total` 是否上升
   - `connection_error_total`、超时错误是否上升
   - 任务失败率与 P95 延迟是否恶化
3. 若稳定，可每次上调 5%~10% 做小步放量。
4. 出现抖动时优先回滚：
   - 先降 `file transfer 并发`
   - 再降 `command 并发`
   - 最后收紧 `RPS/burst`

