# Phase 13：UI 信息架构减负重构

本轮改造把 UI 从“协议角色视角”切换到“资源来源 + 映射 + 链路状态视角”。

## 页面结构

一级菜单调整为：

- 节点与隧道：只保留配置
- 本地资源：本机发布资源定义
- 隧道映射：远端资源与本地入口绑定总览
- 链路监控：注册状态、GB28181 运行态、会话链路摘要

## 后端新增接口

- `GET /api/resources/local`
  - 以“本地资源”视角返回现有手工映射仓库内容
  - 当前版本仍是从兼容映射模型投影而来

- `GET /api/tunnel/mappings`
  - 聚合远端 Catalog 资源与本地绑定结果
  - 输出更轻量的单表视图模型

- `GET /api/link-monitor`
  - 聚合注册状态、GB28181 运行态、映射摘要
  - 供链路监控页统一消费

## 兼容性策略

- 旧接口 `/api/mappings`、`/api/tunnel/catalog`、`/api/tunnel/gb28181/state` 继续保留
- 本地资源页当前保存动作仍复用 `/api/mappings`，以避免一次性重做底层仓库
- 后续可继续把 `PublishedResource` 从 `TunnelMapping` 中拆出
