# P1 阶段闭环说明

本阶段目标是在 P0“可控环境先跑稳”的基础上，补齐**可观测性、启动健壮性、前端运维体验和边界表述**。

## 本阶段已落实事项

### 1. 启动/构造路径去 panic 化

- `gateway-server/internal/server/http.go`
  - `NewHandler()` 不再直接 `panic(err)`。
  - 当 handler 初始化失败时，返回结构化 500 响应，包含：
    - 结论（summary）
    - 建议（suggestion）
    - 详情（detail）
    - 动作提示（action_hint）

### 2. 观测/告警闭环

- 新增 `/metrics`，输出 Prometheus 文本格式指标。
- 指标覆盖当前值班最常用的一组运行态信号：
  - SIP TCP 连接数 / 连接错误 / 读写超时
  - RTP 端口池总量 / 已用 / 分配失败
  - HTTP mapping 请求总数 / 失败数 / 慢请求数
  - 入口保护：active requests / rate limit hits / concurrency rejects / allowed total
  - 熔断状态：open count
  - transport recovery failed total
  - self-check item 数与 overall 状态
  - task total（按 status 标签输出）
  - go_goroutines
- `deploy/observability/prometheus/alerts.yaml` 中使用的核心指标名称，已在 `/metrics` 中对齐。

### 3. UI 信息架构与细节收口

- 保持主导航收敛到“当前运维主链路”页面，不再新增分散入口。
- 网络配置页自检结果已改为“结论 + 建议 + 详情 + 打开修复页”。
- 链路监控页 readiness / SIP 发送链路提示改为可执行提示块。
- 告警与保护页明确区分“运行态命中计数”与“配置值”。
- 系统设置页新增 `/metrics`、`/readyz`、`/api/selfcheck`、`/api/startup-summary` 入口展示。
- 长编码显示优化：
  - 项目编码 / 机器码改为等宽字体
  - 支持自动换行
  - 支持复制
  - 不再因容器宽度不足被截断

### 4. 自检结果与修复动作联动

- 自检项展示改为：
  - 结论
  - 建议
  - 详情（动作提示）
  - 文档链接
  - 打开修复页
- 当前联动规则：
  - peer / binding 问题 → 节点与隧道
  - mapping / capability 问题 → 隧道映射
  - SIP / RTP / storage 问题 → 节点与隧道
  - 其余运行态问题 → 链路监控

### 5. GB/T 28181 边界说明

文档表述统一为：

- 本系统当前主线是**受控环境下的 HTTP 映射隧道网关**。
- GB/T 28181 在本产品中主要承担**信令骨架和受控承载模型**角色。
- 不对外宣称“面向任意第三方生态的完全互通型 GB/T 28181 通用网关”。

## 本阶段验收建议

1. `build-release.ps1` 必须通过。
2. `/metrics` 可抓取，并能看到核心指标。
3. `/readyz`、`/api/selfcheck`、`/api/startup-summary` 与 UI 展示一致。
4. UI 中长编码、长告警说明、长自检项不出现被截断的情况。
5. 文档与发布包包含本文件与 `docs/observability.md`。

## 当前仍需注意

- Windows smoke 脚本仍在收口中，不能作为 P1 观测/UI 验收的唯一判断依据。
- `/metrics` 当前以“值班常用运行态指标”为主，不替代未来更完整的业务埋点体系。
