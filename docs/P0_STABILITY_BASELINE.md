# P0 稳态基线（可控环境）

本阶段只追求**可控环境下的链路稳态**，不扩展到开放异构互通。

## 阶段目标

1. **黄金链路可观测**：注册、心跳、INVITE、INFO、响应开始、INLINE/RTP、BYE 均能看到阶段状态。
2. **异常不拖垮**：入口流量有硬限流和最大并发保护；上游临时错误进入退避保护。
3. **探针分层**：
   - `/healthz`：仅表示进程存活
   - `/readyz`：表示本地监听、自检与映射运行态是否满足接流条件
   - `/api/selfcheck`：人工排障视图
4. **UI/脚本/文档同步**：链路监控页、保护页、smoke 检查与运维文档同时收敛。

## 本阶段落地项

### 1. 链路阶段状态机

- `tunnelSessionRuntimeState` 新增：
  - `phase`
  - `phase_updated_at`
- GB/T 28181 运行态新增：
  - pending session `stage / last_stage_at / last_error`
  - inbound session `stage / last_stage_at / last_error`

### 2. 入口保护硬执行

映射运行时不再只展示限流配置，而是在**真实入口**执行：

- 全局 RPS/Burst 令牌桶
- 全局 MaxConcurrent 并发闸门
- 保护触发时：
  - 记录访问日志
  - 写运行态原因
  - 返回 `429`（限流）或 `503`（并发闸门）

### 3. 熔断/退避可见化

上游 HTTP 临时错误仍使用现有退避逻辑，同时将以下信息暴露到保护页：

- 当前打开的 circuit 数
- 最近打开到期时间
- 最近打开原因

### 4. 探针与 smoke

- `cmd/opssmoke` 新增 `readyz` 检查
- `Link Monitor` 页新增 live/ready 状态与不就绪原因展示

## 本轮收口补充

- 探针路径 `/healthz`、`/readyz`、审计路径 `/audit/events` 已从嵌入式 UI fallback 中保留，不再被 `index.html` 覆盖。
- 在未启用 mappings、也未指定 peer 的受控空闲环境下，`/api/selfcheck` 会将 `mapping_peer_binding` 记为 `info`，不会把系统判为阻塞错误。
- Windows smoke 脚本已拆分 stdout/stderr 日志文件，便于首启排障。

## 验收口径

在可控环境下，至少满足：

1. `/healthz` 返回 200 且 `status=ok`
2. `/readyz` 返回 200 且 `status=ready`
3. `/api/selfcheck` 不出现 `overall=error`
4. 链路监控页可看到：
   - session phase
   - pending/inbound stage
   - readiness reasons（若不就绪）
5. 保护页可看到：
   - 当前活跃请求
   - 限流命中累计
   - 并发拒绝累计
   - 熔断打开数与最近原因

## 已知边界

- 本阶段没有把策略扩展到多租户/按 peer 精细限流。
- `/readyz` 主要面向**本地接流条件**，不是第三方互通验收结论。
- UI 构建与 Go 模块测试仍依赖本地完整依赖环境；离线容器内无法拉取外部依赖时，只能做源码级静态收口。


## Smoke 自检隔离说明

- `scripts/smoke.ps1` / `scripts/smoke.sh` 现在会为 smoke 运行生成临时配置，自动分配独立的 HTTP/SIP/RTP 端口，避免被本机常驻服务或旧进程污染。
- smoke 运行同时会设置独立的 `GATEWAY_DATA_DIR`，不再复用工作区 `data/` 下的历史 node/mapping 状态。
- 就绪等待不再只看 HTTP 200，而是要求 `/healthz` 与 `/readyz` 返回期望 JSON，避免把旧 UI 页面误判为健康服务。

- smoke 脚本会在启动前执行 `go mod tidy -compat=1.23.0`，避免 `go run` 因模块图未同步而中断。
