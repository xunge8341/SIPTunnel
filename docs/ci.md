# CI/CD 质量门禁说明

本文档说明 SIPTunnel 的 CI / 预发布门禁拆分策略、产物与维护方式。

## 目标与原则

- **PR/PUSH 快速反馈**：覆盖单元测试、协议编解码、e2e smoke、网络配置校验、benchmark smoke。
- **发布前强化校验**：补充回归套件、自检、诊断采样。
- **失败可追踪**：统一输出测试报告、日志摘要、指标快照。
- **控制时长**：将高负载测试放到手动触发或夜间计划任务。

## 工作流总览

### 1) `.github/workflows/ci.yml`

主线 CI（push/pull_request）执行：

- `backend-quality-gates`
  - 调用 `scripts/ci/run_quality_gates.sh`
  - 包含：
    - 单元测试（`go test ./... -short`）
    - 协议编解码测试（SIP/RTP/rtpfile/control）
    - e2e smoke（`scripts/regression/run.sh smoke`）
    - 网络配置校验（config + selfcheck + `scripts/preflight.sh`）
    - benchmark smoke（关键路径低强度）
  - 上传产物：`artifacts/ci`
- `frontend`
  - npm lint + build
- `docker-buildx-example`
  - 多架构镜像构建示例

### 2) `.github/workflows/pre-release.yml`

用于发布前与夜间任务：

- 触发：`workflow_dispatch` + `schedule`（每日 02:00 UTC）
- `prerelease-gates`
  - 调用 `scripts/ci/run_prerelease_checks.sh`
  - 包含：
    - 回归套件（`scripts/regression/run.sh full`）
    - 自检相关测试（selfcheck + gatewayctl）
    - 诊断采样相关测试（diagnostics + observability）
  - 上传产物：`artifacts/ci`
- `nightly-heavy`
  - 长稳 smoke（`scripts/longrun/smoke.sh`）
  - 全量 `go test ./...`
  - 用于手动触发/夜间运行，不阻塞主线 PR

## 产物规范

统一落盘目录：`artifacts/ci`

- `logs/*.log`：每个 gate 的完整日志。
- `reports/*-report.md|json`：结构化测试报告。
- `reports/log-summary.md`：日志尾部摘要，便于快速定位失败。
- `metrics/benchmark-smoke-snapshot.json`：benchmark 关键指标快照（ns/op、B/op、allocs/op）。

## 可维护性设计

- 通用执行/报表逻辑下沉到：`scripts/ci/common.sh`。
- 门禁职责拆分：
  - 快速质量门禁：`scripts/ci/run_quality_gates.sh`
  - 发布前门禁：`scripts/ci/run_prerelease_checks.sh`
- 新增检查项时仅需：
  1. 在对应脚本新增 `run_gate "name" <command>`。
  2. 在本文档补充该 gate 描述。

## 时长控制建议

- PR 默认保持在 25 分钟内（`ci.yml` 已设置 `timeout-minutes`）。
- benchmark 仅保留关键路径、低 `benchtime`。
- 重压测试（长稳、全量回归）放在手动或夜间任务，不阻断开发迭代。
