# GB/T 28181 拟态改造（Phase 3 注册与目录同步）

这组修改继续沿着“上级域订阅目录、下级域主动注册”的目标推进，把此前只返回 `200 OK` 的信令壳真正接到运行态：

## 这次补上的部分

### 1. 下级域主动注册不再是空探测
- 新增 `internal/server/gb28181_registrar.go`
- `tunnelSessionManager` 现在通过 `gb28181Registrar` 实际发送：
  - `REGISTER`
  - `MESSAGE(Application/MANSCDP+xml Keepalive)`

这意味着当前“注册/心跳状态”不再只是 TCP 探活，而是走真实 SIP 文本报文。

### 2. 上级域收到 REGISTER 后自动发 Catalog 订阅
- `internal/server/gb28181_tunnel.go`

收到下级域 `REGISTER` 后，服务会：
1. 记录对端注册信息（设备标识、回调地址、传输方式、最近注册时间）
2. 异步向该对端发起 `SUBSCRIBE(Event: Catalog)`

这一步把“下级域主动注册 → 上级域目录订阅”的背景链路接上了。

### 3. 下级域收到 SUBSCRIBE 后回真实 Catalog NOTIFY
- `internal/server/gb28181_tunnel.go`

收到 `SUBSCRIBE(Event: Catalog)` 后，会：
1. 先同步回 `200 OK`
2. 再异步发送 `NOTIFY(Event: Catalog)`
3. `NOTIFY` body 使用 `Application/MANSCDP+xml`
4. 目录内容来自本地下级域映射定义，按 `DeviceID` / `Name` / `MethodList` / `ResponseMode` 等字段组织

### 4. 上级域收到 Catalog NOTIFY 后写入 CatalogRegistry
- `internal/server/catalog_registry.go`
- `internal/server/gb28181_tunnel.go`

`CatalogRegistry` 新增了两类同步能力：
- `SyncRemoteCatalog(...)`：把对端目录 XML 落成本地可查询虚拟资源目录
- `SyncExposureMappings(...)`：把本地暴露端口策略（`LocalPort -> DeviceID`）与目录分离维护

这样可以避免后续本地映射更新时把远端目录快照直接覆盖掉。

## 这次顺手收掉的工程问题

### 1. 目录快照和本地暴露策略解耦
之前 `CatalogRegistry` 只有一个“从 mapping 全量覆盖”的同步入口。现在拆成：
- **本地资源发布视角**：`SyncMappings(...)`
- **上级域接收远端目录视角**：`SyncRemoteCatalog(...)`
- **本地端口暴露策略更新**：`SyncExposureMappings(...)`

### 2. 映射变更不再直接抹掉远端 Catalog
`handlerDeps.syncMappingRuntime()` 现在只更新暴露端口绑定，不再在每次映射变更时把 `CatalogRegistry` 回退成“仅本地 mapping 快照”。

## 目前仍未补满的地方

1. `SUBSCRIBE/NOTIFY` 还没有做完整的订阅续订、失效和 route-set/dialog 维护。
2. `REGISTER` 还没有补鉴权（Digest/401 challenge）和更细的失效/刷新策略。
3. `CatalogRegistry` 已经接到运行态，但尚未把目录快照暴露成专门的运维 API 页面。
4. 上级域本地 HTTP 监听仍然主要依赖现有 mapping 配置创建监听器；还没有做到“仅凭 Catalog 自动分配本地端口”的全动态编排。
5. 回调地址仍优先来自本地监听配置和 Contact，尚未引入更稳的对外可达地址发现策略。

## 下一步最值得做的事

1. 把 Catalog 快照暴露成 `/api/tunnel/catalog` 一类的只读运维接口。
2. 让上级域根据 Catalog + 本地策略自动生成/恢复 `LocalPort -> DeviceID` 监听绑定。
3. 给 `REGISTER / SUBSCRIBE / NOTIFY / INVITE / INFO / BYE` 补完整 dialog/cseq 状态机。
4. 把当前 UDP chunk body 回传继续升级成更像 28181 媒体面的封装。
