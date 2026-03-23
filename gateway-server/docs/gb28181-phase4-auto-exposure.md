# GB28181 Phase 4：Catalog 驱动自动暴露

本轮改造把“收到 Catalog”进一步推进为“Catalog 驱动运行态 HTTP 监听生成”。

## 已落地

- 上级域在收到 `NOTIFY(Catalog)` 后，不仅更新 `CatalogRegistry`，还会触发一次运行态重建。
- 运行态会把**手工映射**与**远端 Catalog 自动暴露映射**合并后再同步到本地 HTTP 监听层。
- 自动暴露项使用以下策略：
  - `DeviceID` 作为跨域主键
  - 手工映射优先，若某个 `DeviceID` 已手工配置，则不会再自动生成监听
  - 自动分配 `LocalPort`
  - 避开浏览器危险端口、本地 SIP 端口、本地 RTP 端口范围
  - 尽量复用此前已经分配过的自动端口，避免目录刷新后端口频繁漂移
- 新增 `GET /api/tunnel/catalog`
  - 返回当前目录项
  - 标识每个资源当前是 `MANUAL`、`AUTO` 还是 `UNEXPOSED`
  - 返回自动生成的本地映射视图，便于排障和联调

## 关键文件

- `internal/server/catalog_exposure.go`
- `internal/server/catalog_http.go`
- `internal/server/catalog_registry.go`
- `internal/server/gb28181_tunnel.go`
- `internal/server/http.go`

## 仍未完成

- 自动暴露端口范围目前采用代码内默认策略，尚未做成显式配置项
- 自动暴露项尚未持久化到映射仓库，目前属于运行态生成
- `REGISTER` Digest 鉴权、`SUBSCRIBE` 续订、完整 dialog 生命周期仍需继续补强
