# HTTP over 28181 映射网关下载慢问题整改核对报告（2026-03-23）

## 一、整改目标

围绕 `http over 28181` 映射网关下载慢问题，落实以下整改目标：

1. 识别并修复“外层下载事务 ID 在 28181 请求头压缩链路中丢失”的实现缺陷。
2. 确保 generic download 限速按“外层下载事务”而不是按 `target URL` 错误并桶。
3. 补齐针对 UDP 正常压缩、UDP budget、UDP severe budget 三条路径的回归用例。
4. 输出可复核的源码证据、日志证据、核对结果与交付件。

---

## 二、问题证据闭环

### 1. 生产日志证据：下载限速按错误桶统计

日志中反复出现以下现象：

- `transfer_id` 直接等于文件下载 URL，而不是外层下载事务 ID。
- `active_segments_transfer=11`
- `effective_transfer_bitrate_bps=8388608`
- `effective_bitrate_bps=762600`

样例证据：

- `SP0323_3.txt:9`
- `SP0323_3.txt:65`
- `SP0323_3.txt:101`
- `SP0323_3.txt:117`

这说明 8Mbps 总传输速率已经进入执行面，但又被 `active_segments_transfer=11` 继续平分，单段落到了约 0.76Mbps。

### 2. 生产日志证据：28181 请求头压缩后只剩 3 个头

日志中多次出现：

- `request_headers_compacted ... original=13 compacted=3`

样例证据：

- `GA0323_1.txt:5`
- `GA0323_1.txt:26`
- `GA0323_2.txt:...`
- `GA0323_3.txt:...`

这说明进入 GB28181/UDP 中继后的请求头经过了强压缩，关键内部控制头很可能在这里被裁掉。

### 3. 生产日志证据：媒体面真实吞吐只有 0.63~0.64Mbps 左右

样例证据：

- `GA0323_2.txt:12` -> `body_bitrate_bps=639880`
- `GA0323_2.txt:1026` -> `body_bitrate_bps=640296`
- `GA0323_2.txt:2317` -> `body_bitrate_bps=631080`

这与前述 `effective_bitrate_bps=762600` 同量级，符合“按错误桶做持续限速”的故障表现。

### 4. 源码证据：下载事务 ID 原本就要求透传

`gateway-server/internal/server/mapping_runtime.go:293-295`

源码注释已明确说明：

- `X-SIPTunnel-Download-Transfer-ID` 必须沿上级 -> 下级 HTTP 继续透传。
- 如果这里丢失，下级会退回 `target URL` 做兜底。
- 多个外层下载会被错误并桶。

### 5. 源码证据：分段下载时已经设置了下载事务 ID

`gateway-server/internal/server/mapping_runtime_delivery.go:825-826`

分段请求构造时，已经把 `downloadTransferIDHeader` 写入 `segmentPrepared.Headers`。

### 6. 源码证据：下游发送面依赖该 header 建立 generic download context

`gateway-server/internal/server/gb28181_tunnel.go:1717-1718`

下游 RTP 发送前，会从 `prepared.Headers.Get(downloadTransferIDHeader)` 读取事务 ID。

### 7. 源码证据：丢失后会退回 target URL 并按活跃分段平分

- `gateway-server/internal/server/generic_download_control.go:152-157`
- `gateway-server/internal/server/generic_download_control.go:290-295`

逻辑含义：

- `transferID` 为空时退回 `target`。
- `ActiveSegmentsTransfer > 1` 时将总速率继续按分段数平分。

---

## 三、整改实施情况

### 整改项 1：在 28181 请求头压缩保留名单中加入下载事务 ID

**修改文件**

- `gateway-server/internal/server/gb28181_transport_helpers.go`

**修改位置**

- `181-196`

**整改前**

`compactTunnelRequestHeaders()` 保留名单没有 `downloadTransferIDHeader`。

**整改后**

在 `preserveOrder` 中新增：

- `downloadTransferIDHeader`

**代码证据**

- `gateway-server/internal/server/gb28181_transport_helpers.go:193-195`

**整改结论**

UDP 请求头压缩路径已不再无条件裁掉下载事务 ID。

---

### 整改项 2：在 UDP severe budget 路径中显式回灌下载事务 ID

**修改文件**

- `gateway-server/internal/server/gb28181_transport_helpers.go`

**修改位置**

- `287-295`

**整改前**

`compactTunnelRequestHeadersForUDPSevereBudget()` 会重新构造 `severe` header，仅保留 `Content-Type / Cookie / Authorization`，存在再次丢失下载事务 ID 的风险。

**整改后**

新增：

- `if transferID := strings.TrimSpace(budgeted.Get(downloadTransferIDHeader)); transferID != "" { severe.Set(downloadTransferIDHeader, transferID) }`

**代码证据**

- `gateway-server/internal/server/gb28181_transport_helpers.go:292-294`

**整改结论**

即便进入 severe budget 强裁剪路径，下载事务 ID 仍会被显式回灌并继续透传。

---

### 整改项 3：补齐三条回归用例

**修改文件**

- `gateway-server/internal/server/gb28181_transport_helpers_test.go`

**新增用例**

1. `TestCompactTunnelRequestHeaders_UDPPreservesDownloadTransferID`
2. `TestCompactTunnelRequestHeadersForUDPBudget_PreservesDownloadTransferID`
3. `TestCompactTunnelRequestHeadersForUDPSevereBudget_PreservesDownloadTransferID`

**代码证据**

- `gateway-server/internal/server/gb28181_transport_helpers_test.go:27-34`
- `gateway-server/internal/server/gb28181_transport_helpers_test.go:112-124`
- `gateway-server/internal/server/gb28181_transport_helpers_test.go:157-171`

**已有链路用例继续保留**

- `gateway-server/internal/server/mapping_runtime_test.go` 中已有 `TestMappingRuntimeManager_PreservesDownloadTransferHeader`

**整改结论**

正常 UDP 压缩、budget 压缩、severe budget 压缩三条路径均已纳入回归覆盖范围。

---

## 四、核对结果

### 1. 静态源码核对

核对项：

- UDP compact preserve list 包含 `downloadTransferIDHeader`
- UDP severe budget 显式回灌 `downloadTransferIDHeader`
- 三个新增回归测试存在
- 既有映射运行时透传测试存在

**结果：通过**

### 2. 代码格式化核对

执行：

- `gofmt -w internal/server/gb28181_transport_helpers.go internal/server/gb28181_transport_helpers_test.go`

**结果：通过**

### 3. 原生 Go 单元测试执行核对

尝试执行：

```bash
go test ./internal/server -run 'TestCompactTunnelRequestHeaders|TestMappingRuntimeManager_PreservesDownloadTransferHeader' -count=1
```

实际结果：

- 仓库 `go.mod` 声明 `go 1.25.0`
- 当前容器仅有本地 `go1.23.2`
- 由于容器无外网，无法从 `proxy.golang.org` 拉取 `go1.25.0` toolchain
- 因此本次环境内**无法完成原生 Go 测试执行**

对应失败输出已单独保存：

- `go_test_attempt.txt`

**结果：环境受限，未执行通过；不是代码逻辑失败。**

---

## 五、最终整改判定

### 判定结论

**源码整改已落实到位，关键实现缺陷已修复，回归用例已补齐，源码证据充分。**

本次可以判定为：

- **整改完成（源码层）**：通过
- **整改完成（静态核对层）**：通过
- **整改完成（原生单测执行层）**：受当前离线工具链环境限制，未完成现场执行

### 风险提示

当前未在本容器内完成原生 `go test` 不是因为源码失败，而是因为：

1. `go.mod` 需要 `go1.25.0`
2. 当前容器仅有 `go1.23.2`
3. 容器无法联网下载新 toolchain

因此，**上线前仍建议在具备 Go 1.25.0 工具链的构建环境中补跑：**

```bash
go test ./internal/server -run 'TestCompactTunnelRequestHeaders|TestMappingRuntimeManager_PreservesDownloadTransferHeader' -count=1
```

以及建议增加一次端到端回归：

1. 单文件下载，校验 `transfer_id` 不再退回 URL。
2. 两个外层下载同时命中同一资源，校验不会错误并桶。
3. 观察 `active_segments_transfer` 是否回归到按“外层下载事务”统计。

---

## 六、交付件

1. 修复后的源码包
2. 本核对报告
3. `go_test_attempt.txt`（环境受限的测试尝试输出）
4. `static_validation.json`（静态核对结果）

