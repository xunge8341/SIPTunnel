# SIPTunnel

SIPTunnel 当前主线是 **HTTP 映射隧道模式**：在受限网络模式下，把控制面（SIP）、大载荷回传（RTP）与运维面（UI / API / 审计 / 观测）收敛成一套可治理、可追踪、可回放的工程体系。

## 当前有效事实源

发生冲突时，以以下文档为准：

- `docs/README.md`：文档索引与术语入口
- `docs/design.md`：总体设计与当前有效策略
- `docs/BACKEND_CHAIN_SEQUENCE_AND_OPTIMIZATION_20260321.md`：后台真实执行链路、时序图与可继续优化项
- `docs/ENGINEERING_GUARDRAILS.md`：工程守则与禁止项
- `docs/DEPLOYMENT_AND_OPERATIONS.md`：部署、升级、回滚与运维动作
- `docs/CONFIGURATION_CLEANUP.md`：配置职责边界与唯一事实来源

## 当前后台主链路

- **控制面**：SIP 注册、目录查询、Invite、MESSAGE/NOTIFY 回调
- **映射面**：本端入口 → 映射准备 → 上游 HTTP → INLINE/RTP/分段策略选择
- **大响应交付**：`stream_primary / range_primary / adaptive_segmented_primary / fallback_segmented`
- **RTP 弱网增强**：更大重排窗口、gap 容忍、FEC、socket buffer、恢复链路
- **运维观测**：访问日志、审计、诊断导出、保护与熔断、清理任务


## 控制台当前主菜单

- 节点与级联
- 本地资源
- 隧道映射
- 链路监控
- 授权管理
- 安全事件

## 工程边界

- 交付包只应包含**源码、配置模板、文档、脚本**。
- `node_modules`、`dist`、临时日志、抓包、构建错误输出、实验性 scratch 文件不得作为事实来源进入源码工程。
- 历史阶段文档（`phase*`、`reviews/`）用于追溯，不作为当前运行态首要事实来源。

## 推荐先读

1. `docs/README.md`
2. `docs/design.md`
3. `docs/BACKEND_CHAIN_SEQUENCE_AND_OPTIMIZATION_20260321.md`
4. `docs/operations.md`
