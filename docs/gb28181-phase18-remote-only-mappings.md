# Phase 18 收口

- 修复 `gateway-server/internal/server/tunnel_relay.go` 缺少 `siptext` 导入导致的编译失败。
- 将“隧道映射”明确限定为 **远端目录资源 + 本地监听补充信息**：
  - `CatalogRegistry` 新增 `remoteResources`
  - `currentCatalogExposurePlan()` 与运行态 `syncMappingRuntime()` 仅基于 `RemoteSnapshot()` 生成自动暴露与映射总览
  - `SyncExposureMappings()` 不再为了本地映射补造目录资源占位项
- 这样“本地资源”仍保留在本地资源页，不会再混入“隧道映射”页。
