# Round 10 迁移说明（术语/配置/网络模式/运行时）

本文档面向本轮重构后的运维与开发迁移，覆盖术语、配置模型、网络模式、注册心跳与映射运行时行为变化。

## 1. 设备编码唯一来源

- `local_device_id` 与 `peer_device_id` 的唯一来源为 **节点配置**（`node config`）。
- `tunnel config` 页面/API 仅只读展示设备编码，不再允许编辑。
- 迁移建议：若历史脚本还在写 `tunnel config` 的设备编码字段，应改为维护本端节点与对端节点配置。

## 2. 新网络模式定义

系统统一使用以下 `NetworkMode`：

- `SENDER_SIP__RECEIVER_RTP`（别名兼容：`A_TO_B_SIP__B_TO_A_RTP`）
- `SENDER_SIP__RECEIVER_SIP_RTP`（别名兼容：`A_B_BIDIR_SIP__B_TO_A_RTP`）
- `SENDER_SIP_RTP__RECEIVER_SIP_RTP`（别名兼容：`A_B_BIDIR_SIP__BIDIR_RTP`）
- `RESERVED_*`（预留模式，默认能力降级）

说明：能力矩阵与 transport plan 由后端按 `NetworkMode` 统一推导，不允许在单条映射规则中覆盖。

## 3. 发送端 / 接收端角色

在 HTTP 映射隧道主线中：

- **接收端**：监听本端入口（`local_bind_ip:local_bind_port`），接收业务 HTTP 请求。
- **发送端**：根据映射配置访问对端目标（`remote_target_ip:remote_target_port`），并回传响应。

角色是“链路职责”而非页面选项，由网络模式与运行时自动确定。

## 4. tunnel config 与 node config 关系

- `node config`：节点身份、设备编码、网络模式兼容性来源。
- `tunnel config`：通道协议、连接发起策略、注册重试与心跳策略。
- 二者关系：
  - `tunnel config` 读取 `node config` 生成只读设备编码展示；
  - `network_mode` 变更会联动刷新 capability 与 transport plan；
  - 对端绑定冲突（无启用对端/多个启用对端）会在 mappings API 返回 `PEER_BINDING_INVALID`。

## 5. 注册 / 心跳状态字段含义

- `registration_status`：`unregistered/registering/registered/failed`
- `heartbeat_status`：`unknown/healthy/timeout/lost`
- `last_register_time`：最近一次注册动作完成时间。
- `last_heartbeat_time`：最近一次心跳上报时间。
- `last_failure_reason`：最近失败原因（用于运维排障）。
- `next_retry_time`：下一次重试计划时间。
- `consecutive_heartbeat_timeout`：连续心跳超时计数（用于判断链路劣化趋势）。

## 6. mapping runtime：监听占位 -> 真实转发

本轮后端运行时从“监听占位”升级为“真实 HTTP 转发”：

1. `enabled=true` 自动监听本端入口；`enabled=false` 或删除映射自动释放监听。
2. 入口收到 HTTP 请求后，转发 `method/path/query/header/body` 到对端目标。
3. 回传真实 `status/header/body`；链路失败返回 `502 Bad Gateway`。
4. 运行时状态回写到映射列表：`disabled/listening/forwarding/degraded/connected/interrupted/start_failed`，并附 `status_reason/failure_reason/suggested_action`。

## 7. README/docs 验证步骤（建议每次改动执行）

1. 前端测试（文案 + 快照 + 页面行为）：
   - `cd gateway-ui && npm test -- src/views/__tests__/TunnelConfigView.spec.ts src/views/__tests__/TunnelMappingsView.spec.ts src/utils/__tests__/capability.spec.ts`
2. 后端关键测试（handler + runtime + network mode）：
   - `cd gateway-server && go test ./internal/service/sipcontrol ./internal/server ./internal/config`
3. 后端全量回归：
   - `cd gateway-server && go test ./...`
4. 联调核验：
   - `GET /api/tunnel/config`：设备编码只读展示与注册/心跳状态字段完整。
   - `GET /api/system/status`：网络模式、能力矩阵、注册/心跳状态可见。
   - `POST/GET /api/mappings`：映射运行时状态回写与异常建议可见。

## 8. 剩余待确认项（建议下轮确认）

- 预留网络模式（`RESERVED_*`）的正式命名与上线开关策略。
- 跨版本脚本是否仍依赖 deprecated `/api/routes` 输出结构。
- 生产环境中 `heartbeat_status=lost` 与 `timeout` 的告警阈值与分级策略。
