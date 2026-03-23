# 工程固定原则（不得随意改动）

本文档定义 SIPTunnel 当前主线的**固定原则**。后续 UI、代码、文档、脚本的新增或重构，若与本文冲突，必须先更新本文并在变更说明中解释原因，否则视为不合规改动。

## 1. 产品主线与术语

当前主线是：

> **HTTP 映射隧道模式**
>
> 网络能力模式 → 能力矩阵 → 本地资源 → 隧道映射 → 会话/日志/审计

统一主术语：

- 本端节点
- 对端节点
- 网络能力模式
- 能力矩阵
- 本地资源
- 隧道映射
- 本端入口
- 对端目标
- 链路监控

禁止重新引入为主术语：

- 节点与隧道
- `route`
- `api_code`
- `template`
- `server.port`

## 2. 页面职责边界（UI 固定信息架构）

一级菜单固定为：

- 总览监控
- 节点与级联
- 本地资源
- 隧道映射
- 链路监控
- 访问日志
- 运维审计
- 告警与保护
- 系统设置
- 诊断与压测
- 授权管理
- 安全事件

页面职责固定如下：

### 2.1 节点与级联

只负责：

- 本端/对端节点
- SIP/RTP
- 会话/安全
- 本地隧道映射端口范围

不得承载：

- 本地资源 CRUD
- 隧道映射 CRUD
- 运行态动作（注册/重注册/手动拉取目录/手动推送目录）

### 2.2 本地资源

只负责：

- 资源编码/名称/目标 URL/方法/响应模式
- 目录发布源
- 手动推送目录

不得承载：

- 本地监听端口
- 本地监听路径
- 隐式生成映射
- 隐式分配端口

### 2.3 隧道映射

只负责：

- 远端目录资源选择结果
- 本地监听端口/路径补充
- 手动拉取目录
- 单条映射测试/删除/启停

不得承载：

- 本地资源定义
- 对端节点主配置
- RTP 端口配置

### 2.4 链路监控

只负责：

- REGISTER / SUBSCRIBE / 会话 / 心跳状态
- 手动重注册/链路动作
- 运行态观察

不得承载：

- 资源定义
- 映射编辑

## 3. 配置职责边界

### 3.1 `config.yaml`

只承载：

- 基础运行壳层配置
- UI / SIP / RTP / storage / observability / ops

禁止在 `config.yaml` 中承载：

- 本地资源
- 隧道映射
- 运行态 peer 目录
- UI 业务表单数据

### 3.2 运行态数据

运行态业务数据统一由 API 和持久化文件/数据库维护：

- 节点与级联工作区：`/api/node-tunnel/workspace`
- 本地资源：`/api/resources/local`
- 隧道映射：`/api/mappings`

### 3.3 配置唯一来源

以下字段只允许有一个配置来源：

- 管理面端口：`ui.listen_port`
- 本地资源：本地资源存储
- 本地监听端口：隧道映射存储
- 本地隧道映射端口范围：节点与级联工作区

禁止双写或“兼容影子字段”。

## 4. 传输与承载原则

### 4.1 控制面与数据面分离

- 小元数据、会话控制：SIP MESSAGE / SUBSCRIBE / NOTIFY
- 大响应体：RTP + PS

### 4.2 RTP 模式禁止整包缓存

RTP 模式下禁止：

- `io.ReadAll(upstream.Body)` 后再整体发送
- 接收端先整包收完再统一回放

必须采用：

- 流式读 upstream body
- 流式封装 PS/RTP
- 流式接收并回写

### 4.3 结束条件固定

RTP 响应完成应优先根据：

- `ResponseEnd.content_length`
- 或已知 `Content-Length`
- 或 `BYE`

不得把 RTP Marker 位当作“应用层消息结束”的唯一依据。

### 4.4 播放与下载策略必须分开治理

- 视频播放优先首包快、平滑连续，不得因为下载弱网补强而把主播放路径改成重分段重缓存。
- 非音视频大文件下载优先完整性与可恢复性；开区间 Range（如 `bytes=0-`）默认走更保守的分段下载主路径。
- WEB 页面 / WEBAPI 的小响应与 JSON 主路径继续按轻量 HTTP / INLINE / 正常 RTP 判定，不得被下载策略误伤。

## 5. 日志与观测原则

### 5.1 访问日志按入口视角记录

访问日志的主口径是：

- 入口路径（entry path）
- 入口状态
- 入口时延
- 入口传输模式（INLINE / RTP）

浏览器同源子请求：

- 成功且不慢时默认不单独刷访问日志
- 失败、慢请求、异常请求保留

### 5.2 传输级日志必须结构化

RTP / SIP / 目录同步日志必须包含：

- `call_id`
- `device_id`
- `mode`
- `content_length`
- `body_bytes`
- `ps_bytes`
- `rtp_packets`
- `duration_ms`
- `completion`

### 5.3 运行态支撑域必须按事实源拆分

- 访问日志存储/采样/摘要：独立文件承载
- 压测任务模型与启动：独立文件承载
- 安全事件记录与审计联动：独立文件承载
- `runtime_support.go` 只允许保留边界说明或极薄壳层

禁止重新把 access log、loadtest job、安全事件、默认值归一化混回单一运行态大文件。

### 5.4 性能分析必须有阶段指标

至少要能区分：

- INVITE 建链耗时
- 上游 HTTP 耗时
- ResponseStart 到 ResponseEnd 耗时
- RTP 接收完成到本地响应完成耗时

## 5.5 后端按职责分文件，禁止继续堆积“总控大文件”

后台 HTTP/运维接口必须遵循按职责分文件：

- `http.go`：只保留 handler 依赖装配、启动骨架、通用中间件、公共响应工具
- `http_linktest.go`：链路探测、自检辅助、入口探测口径
- `http_mappings.go`：映射 CRUD / 映射测试 / 映射诊断
- `http_tunnel_ops.go`：隧道配置、目录动作、会话动作、对端节点
- `http_audits.go`：审计查询、审计分页、操作人读取
- `ops_system_settings_http.go`：系统设置与清理配置
- `ops_dashboard_http.go`：访问日志、dashboard 汇总、趋势聚合
- `ops_protection_security_http.go`：保护状态、安全中心
- `ops_workspace_loadtest_http.go`：节点工作区与压测任务

禁止再把新 handler 或聚合逻辑回灌到：

- `http.go`
- `ops_settings_logs.go`

新链路若需要新增 handler，必须先确定职责归属，再放入对应文件。

## 5.6 配置、自检、后端测试也必须按职责分层，禁止重新回堆单体文件

- `internal/config/network.go`：只保留网络配置模型/字段定义
- `internal/config/network_defaults.go`：默认值、YAML 解析、默认回填
- `internal/config/network_validate*.go` / `transport_tuning_validate.go`：配置校验与冲突校验
- `internal/selfcheck/selfcheck.go`：Runner、Report、总装配
- `internal/selfcheck/selfcheck_ports.go`：listen_ip / 端口占用 / 端口建议 / 进程诊断
- `internal/selfcheck/selfcheck_checks.go`：RTP/存储/下游可达性/一致性检查
- `internal/server/http_test_support_test.go`：测试装配和共用 helper
- `internal/server/http_mappings_test.go`：映射域测试
- `internal/server/http_nodes_test.go`：节点/隧道/链路域测试
- `internal/server/http_ops_test.go`：metrics/tasks/audits/selfcheck/system-status 域测试

禁止再把：

- 网络配置默认值、校验、推荐值重新堆回一个文件
- selfcheck 的端口诊断、目录写入、下游探测重新堆回 `selfcheck.go`
- 后端接口测试重新回堆到单体 `http_test.go`

## 5.7 绑定地址冲突判定必须复用共享能力

`config`、`selfcheck`、`repository` 里涉及 `0.0.0.0` / `::` / 显式 IP 的绑定冲突判定，必须统一复用共享 helper；
禁止再各写一份 `sameBindAddress(...)`，避免配置校验、运行时自检、持久化冲突检测给出不一致结论。

## 6. 文档治理原则

### 6.1 文档分层

- `README.md`：仓库级总览
- `docs/README.md`：当前有效文档索引
- `docs/ENGINEERING_GUARDRAILS.md`：固定原则
- `docs/REVIEW_AND_CLEANUP_20260318.md`：本轮 Review 结论
- `docs/gb28181-phase*.md`：归档/阶段性文档

### 6.2 发生冲突时的优先级

1. 代码行为
2. `docs/ENGINEERING_GUARDRAILS.md`
3. `README.md` / `docs/README.md`
4. 其他当前有效文档
5. 阶段性归档文档

## 7. 脚本治理原则

### 7.1 脚本层级

- `scripts/`：仓库级公共脚本
- `scripts/ci/`：CI / 质量门禁
- `scripts/acceptance/`：阶段验收与抓包回归
- `scripts/smoke/`、`scripts/regression/`：回归验证

### 7.2 一致性检查必须自动化

必须存在静态检查，至少覆盖：

- 菜单名/文档主术语是否仍使用当前口径
- 禁止重新引入 `server.port`
- 当前有效文档是否仍引用“节点与隧道”这类旧名称
- README / docs/README / UI 菜单是否一致

## 8. 变更约束

任何改动如触及以下内容，必须同步修改 UI、代码、文档、脚本四处中的至少相关项：

- 菜单名称
- 页面职责
- 配置字段
- 运行态数据来源
- 日志口径
- 验收标准

否则视为“局部修复导致全局漂移”，不允许合入。

## 7. 策略治理原则

### 7.1 同一策略只能有一个选择入口

像“大响应交付策略”“分段 profile 选择”“目录资源暴露状态”这类全局策略，代码里只能保留一个统一解析入口；
禁止在 runtime、lane、UI 适配层各自复制一份 if/else 规则。

### 7.2 已退场策略不得继续冒充运行态事实

已经退场的自动暴露/自动映射、历史 fallback 口径、旧日志字段等，必须从控制台、返回体、mock、测试里同步清理；
不允许出现“代码不再生效，但 UI 仍展示 AUTO / 自动暴露”的伪事实。

### 7.3 僵尸函数必须随策略退场一并删除

策略已经收口后，同步删除只剩定义、不再被运行链路调用的辅助函数、包装层和测试样例；
禁止为了“以后也许有用”长期保留未接线的函数。

### 7.4 启动摘要必须可直接回答“当前策略到底是什么”

只输出 tuning 数值不够；
只要运行时存在策略家族（大响应交付、分段 profile、RTP 发送/容忍、generic breaker），启动摘要就必须输出统一快照，
让一线不用翻多份代码和文档去拼当前生效策略。

### 7.5 失败原因字典必须共享

`timeout / connection_reset / broken_pipe / unexpected_eof / rtp_sequence_gap / rtp_gap_timeout` 这类跨链路失败原因，
必须集中到共享分类工具；
禁止 generic、resume、breaker、penalty、日志汇总各自硬编码一套字符串匹配。

### 7.6 基础网络判断不得重复实现

像“address already in use / only one usage of each socket address”这类跨平台基础判断，
必须沉到共享基础层；
禁止启动入口、媒体入口、脚本诊断各自复制一份文本匹配逻辑。


## 本轮继续清洁（2026-03-21, round-continued）

- 后端 `http.go` 只保留共享骨架与公共类型；任务/节点/状态类 handler 必须拆分到独立文件，避免继续堆成单体入口。
- 超时、连接拒绝、连接中断、本地地址耗尽、UDP 报文过大等网络失败词典，统一收敛到 `gateway-server/internal/netdiag`；运行时链路、loadtest、诊断脚本不得各自复制近义词判断。
- 观测、控制台、诊断包引用的失败原因必须复用共享词典，避免“日志会说一套、压测会判一套、运维建议再说一套”。
- `mapping_runtime.go` 不得再次回灌大响应复制 / 分段窗口 / resume 恢复的实现细节；这些交付链路必须留在独立文件（如 `mapping_runtime_delivery.go`），让“运行态管理”和“交付算法”分层清晰。
- `gb28181_tunnel.go` 不得再次回灌 URI/对话头部/请求头压缩/UDP 回调压缩这类低层辅助实现；这些能力必须继续下沉到独立 helper 文件，避免控制流程文件重新膨胀。
- 清理僵尸函数时，优先保留“当前真实链路 + 当前有效测试”需要的最小集合；不为了名义上的扩展性保留未接线辅助层。


## 本轮彻底清洁补充（2026-03-21, round-finalize）

- `cmd/gateway/main.go` 只允许保留主入口壳层；启动流程、自检/摘要、config 命令、SIP UDP server 必须分别留在独立文件，不得重新堆回单体入口。
- `gb28181_media.go` 只允许保留 RTP/PS 公共常量、公共类型与 profile 解析；PS 编解码、发送侧、接收侧必须拆分到独立文件。
- `loadtest.go` 只允许保留模型定义与 Run 主流程；诊断采集/报告、协议操作、校验与统计工具不得重新回灌。
- 已经证明不再被真实链路调用的包装层（如 frame-level 调用之上的薄壳）应直接删除，不为了“可能以后有用”继续保留。
- 源码仓根目录不得继续沉淀交付报告、验收结果、历史构建输出；这些只能作为交付产物或 artifacts 保存，不再作为源码事实的一部分。
