# 总体清洁回顾与收口（2026-03-21）

本文档用于回顾本轮连续清洁后的**当前工程事实**，帮助后续继续从“全局策略 → 代码结构 → 配置/文档/日志”这条线做增量演进，而不是重新回到局部修补。

## 1. 当前已经完成的主收口

### 1.1 入口层不再维持单体大文件

后端入口层已经完成三轮拆分，当前固定边界如下：

- `gateway-server/internal/server/http.go`：只保留依赖装配、公共响应、通用骨架
- `http_linktest.go`：链路探测/入口探测/自检辅助
- `http_mappings.go`：映射 CRUD / 测试 / 诊断
- `http_tunnel_ops.go`：隧道配置、目录动作、会话动作、对端节点
- `http_audits.go`：审计查询与分页
- `ops_system_settings_http.go`：系统设置与清理策略
- `ops_dashboard_http.go`：访问日志、dashboard 汇总、趋势聚合
- `ops_protection_security_http.go`：保护状态与安全中心
- `ops_workspace_loadtest_http.go`：节点工作区与压测任务

### 1.2 运行态总控与交付算法已经分层

- `mapping_runtime.go`：运行态总控与运行壳层
- `mapping_runtime_delivery.go`：大响应复制、窗口、resume、恢复算法

### 1.3 GB28181 控制流、媒体编解码、发送/接收已经拆开

- `gb28181_tunnel.go`：控制流程主干（REGISTER / MESSAGE / SUBSCRIBE / INVITE / BYE）
- `gb28181_dialog_helpers.go`：URI/对话头/设备标识辅助
- `gb28181_transport_helpers.go`：transport/header/回调压缩辅助
- `gb28181_media.go`：RTP/PS 公共常量与 profile 解析
- `gb28181_media_ps_codec.go`：PS/RTP 编解码细节
- `gb28181_media_sender.go`：RTP 发送侧
- `gb28181_media_receiver.go`：RTP 接收侧

### 1.4 网关启动入口已经按职责收口

- `cmd/gateway/main.go`：主入口壳层
- `cmd/gateway/main_startup.go`：启动主流程、selfcheck、摘要构建
- `cmd/gateway/main_config.go`：config 命令、默认配置生成、配置解析
- `cmd/gateway/main_sip_udp.go`：SIP UDP server 与 peer 放行辅助

### 1.5 loadtest 不再混放“运行主循环 + 诊断 + 协议发送 + 校验/统计”

- `loadtest.go`：模型定义与 Run 主流程
- `loadtest_diagnostics.go`：诊断采集、诊断摘要、Markdown 报告
- `loadtest_ops.go`：HTTP / SIP / RTP 操作与 trace 辅助
- `loadtest_validate.go`：参数校验、预检、错误分类、统计工具

### 1.6 配置/自检/后端接口测试也完成了职责拆分

- `internal/config/network.go`：只保留网络配置模型
- `network_defaults.go`：默认值、YAML 解析、默认回填
- `network_validate.go` / `network_validate_endpoints.go` / `transport_tuning_validate.go`：冲突校验、SIP/RTP 校验、transport tuning 校验
- `internal/selfcheck/selfcheck.go`：Runner/Report/总装配
- `selfcheck_ports.go`：listen_ip、端口占用、端口建议、进程诊断
- `selfcheck_checks.go`：RTP/存储/下游可达性检查
- `internal/netbind/bind.go`：共享绑定地址冲突判定
- `http_test_support_test.go` / `http_mappings_test.go` / `http_nodes_test.go` / `http_ops_test.go`：后端接口测试按责任拆分

这意味着“配置校验 / 自检诊断 / 后端 handler 测试”三块也不再继续依赖单体文件。

### 1.7 运行态支撑域已经按事实源拆分

- `runtime_support.go`：只保留边界说明，不再混放具体实现
- `access_log_store.go`：访问日志过滤、存储、采样丢弃、摘要缓存、聚合分析
- `runtime_support_settings.go`：运维保护默认值、ops 限额默认值与归一化
- `runtime_support_loadtest.go`：压测任务模型、任务存储、压测启动与容量建议
- `runtime_support_security.go`：安全事件记录、审计联动、文件落盘

这意味着访问日志事实源、压测任务、安全事件三块已经不再通过 `runtime_support.go` 这个“运行态胶水大文件”间接承载。

## 2. 本轮进一步确认并清掉的噪音

### 2.1 明确移除的僵尸包装层

- `(*rtpBodySender).SendStream(...)`：当前有效链路只走 `SendStreamWithProfile(...)`，旧包装层已移除
- `sendRTPUDP(...)` / `sendRTPTCP(...)`：loadtest 已直接预构帧并调用 frame-level 发送函数，旧包装层已移除

### 2.2 生成物不再混入源码事实

源码工程根目录不应继续保留本轮交付报告、历史验收输出、构建错误日志等生成物；
这些内容应只存在于交付包或 `artifacts/`，不能继续作为源码树中的“事实源”。

## 3. 当前仍应坚持的全局原则

### 3.1 策略选择入口必须唯一

- 大响应交付家族：只能集中在统一选择器里解析
- 分段 profile：只能集中在统一入口里决定
- RTP 容忍策略：只能集中推导，不能由调用点各自拼字段
- 失败原因分类：必须复用共享字典

### 3.2 入口层文件只描述“编排”，不回灌算法细节

- `http.go` / `main.go` / `mapping_runtime.go` / `gb28181_tunnel.go`
- 这些文件只保留入口装配、编排、分发、阶段控制
- 算法、统计、协议细节、恢复细节都应该下沉到专门文件

### 3.3 日志、控制台、配置、诊断必须说同一套事实

任何新增策略若会影响：

- 启动摘要
- runtime 日志字段
- 控制台展示
- 诊断导出
- 默认配置模板

则必须同步修改，不允许只改一层。

## 4. 下一轮最值得继续压的深水区

从当前形态看，后续继续清洁优先级建议如下：

1. `internal/config/storage.go` / `internal/selfcheck/selfcheck_test.go`
   - 可继续做测试资产和配置族的一致性瘦身
2. `internal/server/ops_observability_service.go` / `ops_dashboard_http.go`
   - 可继续检查 dashboard 趋势聚合与访问日志分析是否还可再分层
3. `internal/server/security_license.go` / `license_support.go`
   - 可继续收口授权/安全中心的共享事实与测试资产

## 5. 当前判断

到这一轮为止，工程已经从“策略越来越多、入口越来越厚、事实源越来越散”收口到：

- 主链路可按职责定位
- 主策略可按单一事实源追踪
- 僵尸包装层开始随策略退场同步删除
- 文档/脚本/源码约束开始自动化校验

后续继续推进时，应优先维持这个方向，而不是再回到“局部修一个点、全局多一层影子规则”。
