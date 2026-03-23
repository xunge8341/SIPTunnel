# GB28181 拟态链路 Phase 7：UI/后台闭环与 Go 1.23 对齐

本轮收口目标：在不推翻现有运行态的前提下，把 **UI、后台、目录/映射语义、构建约束** 串成一个自洽闭环，便于联调前统一验收。

## 1. 语义收口

- **目录不是另一套平行模型**。
  - 对下：下级域本地仍维护真实 HTTP 映射。
  - 对上：发布/订阅看到的是以 `DeviceID` 为主键的“目录化映射视图”。
- **上级域本地暴露也不是另一种业务类型**。
  - 手工映射：运维显式配置。
  - 自动暴露：Catalog 驱动生成临时 mapping。
  - 两者在 UI 中统一收敛为“有效映射视图”。

## 2. UI 收口

### 节点与隧道页
- 新增并回读：
  - `register_auth_enabled`
  - `register_auth_username`
  - `register_auth_password`
  - `register_auth_realm`
  - `register_auth_algorithm`
  - `catalog_subscribe_expires_sec`
- 新增运行态展示：
  - 已注册 peer
  - pending session
  - inbound session
  - catalog 资源数 / 已暴露数

### 隧道映射页
- 手工映射编辑器补齐：
  - `device_id`
  - `response_mode`
  - `max_inline_response_body`
  - `allowed_methods`
- 页面增加两层视图：
  - **有效映射视图**：手工映射 + 自动暴露 mapping 合并展示
  - **目录资源视图**：展示目录项的暴露方式（MANUAL/AUTO/UNEXPOSED）

## 3. API 合同收口

### `/api/tunnel/catalog`
后端返回字段为：
- `method_list`
- `local_ports`
- `mapping_ids`

前端已新增归一化映射，转换为：
- `methods`
- `local_port` / `local_ports`
- `mapping_ids`

避免再出现“API 已加了，但页面没法直接吃”的合同层断点。

### `/api/tunnel/gb28181/state`
前端现在直接消费：
- `session`
- `config`
- `gb28181.peers`
- `gb28181.pending_sessions`
- `gb28181.inbound_sessions`
- `gb28181.catalog`

## 4. 构建约束收口

- `gateway-server/go.mod` 已调整为 `go 1.23.0`
- `gateway-server/Dockerfile` builder 基础镜像已调整为 `golang:1.23-alpine`
- `Dockerfile` 补拷贝 `go.sum`

## 5. 联调前建议验收顺序

1. 打开“节点与隧道”页，保存一次 REGISTER 鉴权和 Catalog 续订配置。
2. 触发“手动注册 / 重新注册 / 发送一次心跳”，观察运行态表格是否变化。
3. 打开“隧道映射”页，确认：
   - 手工映射可编辑
   - 自动暴露项可见但不可误编辑
   - Catalog 未暴露项可识别
4. 访问 `/api/tunnel/catalog` 与 `/api/tunnel/gb28181/state`，确认返回值与页面一致。
5. 在 Go 1.23 环境里完成后端 build；在 Node 环境里完成 UI build/embed。

