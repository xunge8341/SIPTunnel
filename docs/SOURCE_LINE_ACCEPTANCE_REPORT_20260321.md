# SIPTunnel 源码行级验收报告（2026-03-21）

## 1. 验收结论

本轮按“源码审查 + 现场日志回放 + 可落地修正”方式完成交付。

**结论：有条件通过。**

已完成并纳入源码包的事项：

1. **UI/API 清退与补齐收口**：前端源码与后端内嵌静态资源重新对齐，旧接口残留一并清退。
2. **稳定性优化（可直接落地部分）**：
   - UDP 控制面请求头进一步压缩，针对 `web8201` 登录类 POST 的 `request_control_oversize` 做了源码级减负。
   - 下载相关默认带宽/并发配置统一收敛到更保守值，并同步到模板、文档、测试与启动摘要口径。
3. **入口路由算法增强**：新增“入口选错补救”逻辑，根路径 `/`、`/index.html` 命中非根 `local_base_path` 时自动归位到映射入口。
4. **交付链路修复**：`scripts/embed-ui.sh` 改为通过 `bash` 调用 `ui-build.sh`，避免脚本执行位缺失导致打包失败。

未在当前离线容器内完成、需要现场继续验证的事项：

1. **Go 后端完整编译/测试**：当前容器仅有 Go 1.23.2，而工程 `go.mod` 要求 Go 1.25.0；同时缺少若干依赖的 `go.sum` 解析与联网下载条件，因此未完成完整 `go test ./...` / `go build ./...`。
2. **实网回归**：需要在现场继续验证 `web8201` 登录、四路并发下载、VLC 播放、播放+下载混部场景的端到端效果。

---

## 2. 现场问题与审查结论

### 2.1 UI 与后台不匹配：保留补齐还是直接去掉

审查结论：**以“直接去掉已下线能力 + 重建内嵌 UI 资源”为主，不再为已清退页面补后台 API。**

原因：

- 前端源码已完成一轮能力清退，但后端 `embedded-ui` 仍然内嵌旧构建产物，运行态会继续发起已下线接口请求。
- 如果只改源码不重建后端内嵌资源，会出现“源码已清退、运行态仍打旧 API”的假收口。

本次动作：

- 重建并重新嵌入 `gateway-ui/dist` 到 `gateway-server/internal/server/embedded-ui/**`。
- 交付前复查，旧接口残留在 **前端源码** 与 **内嵌静态资源** 中均已清零。

### 2.2 web8201 登录失败

审查结论：**根因不是页面本身，而是 UDP 控制面请求包体超预算。**

日志特征（来自上传现场日志）：

- `POST /api/gmvcs/uap/cas/login`
- `request_headers_compacted ... original=12 compacted=3`
- `request_control_oversize ... sip_bytes=1307 limit=1300 body_bytes=205`
- 最终 `final_status=udp_request_control_oversize`

源码侧处理：

- 将 UDP 控制面请求头压缩策略再收紧，删除对登录/表单 POST 基本无价值、但长度较长的 `Accept` 透传，直接减少 SIP 控制报文大小。

### 2.3 四路文件同时下载慢、速率在 0/400/800KB 摆动

审查结论：**核心问题是下载事务并发与 segment child 并发叠加后，触发全局下载整形与公平分配，实际每事务速率被压低；默认值与文档/模板长期不一致又放大了运维误判。**

本次动作：

- 统一默认值到更保守口径：
  - `udp_bulk_parallelism_per_device = 4`
  - `generic_download_segment_concurrency = 1`
  - `generic_download_total_bitrate_bps = 33554432`
  - `generic_download_rtp_bitrate_bps = 8388608`
- 同步修正到：运行默认值、示例配置、生成模板、元数据、文档、启动摘要测试。

### 2.4 VLC 单路网络播放加载慢、播放卡顿；播放+下载同时进行互相影响

审查结论：**播放与下载的判型/整形预算必须更明确分流，不能让大文件下载路径长期吃掉公共预算。**

本次源码包内已落地的措施：

- 继续维持“下载保守、播放优先”的默认参数方向，避免大文件下载在弱网/多并发下持续抢占 RTP 预算。
- 对入口误选增加容错，减少“入口不对导致的错误路径回退/重试/多次探测”对播放首开时间的放大。

### 2.5 入口路由算法及选错补救

审查结论：**原逻辑对非根 `local_base_path` 要求过严，用户只输入端口根路径时会直接被判为不匹配。**

本次动作：

- 当映射挂在非根路径时，若请求是 `/`、`/index.html`、`/index.htm`，自动视为该映射入口首页，转发到远端 `remote_base_path`。
- 不改变带后缀路径的正常前缀匹配行为，只补最常见的“端口对了、入口路径少了一段”场景。

---

## 3. 行级变更验收清单

| 文件 | 行号 | 变更内容 | 验收结论 |
|---|---:|---|---|
| `gateway-server/internal/server/mapping_runtime.go` | 758-776 | `pathSuffix` 增加 `/`、`/index.html`、`/index.htm` 到非根映射入口的自动补救逻辑 | 通过 |
| `gateway-server/internal/server/mapping_runtime_test.go` | 268-303 | 新增根路径与 `index.html` 误入时的目标地址构造测试 | 通过 |
| `gateway-server/internal/server/gb28181_transport_helpers.go` | 176-194 | UDP 控制面请求头压缩策略删除 `Accept`，保留 `Content-Type/Authorization/Cookie/...` | 通过 |
| `gateway-server/internal/server/gb28181_transport_helpers_test.go` | 8-33 | 新增 UDP 丢弃 `Accept`、TCP 保留 `Accept` 的测试 | 通过 |
| `scripts/embed-ui.sh` | 55-79 | 改为 `bash "$ROOT_DIR/scripts/ui-build.sh"` 并继续执行构建/嵌入同步流程 | 通过 |
| `gateway-server/internal/config/network_defaults.go` | 21-47 | 收敛下载并发/带宽默认值：bulk=4, segment=1, total_bitrate=33554432, rtp_bitrate=8388608 | 通过 |
| `gateway-server/configs/config.default.example.yaml` | 64/69/76/80 | 示例配置同步上述默认值 | 通过 |
| `gateway-server/configs/config.sip-udp.example.yaml` | 61/66/73/77 | SIP-UDP 示例配置同步上述默认值 | 通过 |
| `gateway-server/configs/config.yaml` | 83/88/95/99 | 默认配置文件同步上述默认值 | 通过 |
| `gateway-server/configs/generated/config.example.generated.yaml` | 90/146/166/182 | 生成模板同步上述默认值 | 通过 |
| `gateway-server/configs/generated/config.dev.template.yaml` | 90/146/166/182 | dev 模板同步上述默认值 | 通过 |
| `gateway-server/configs/generated/config.prod.template.yaml` | 90/146/166/182 | prod 模板同步上述默认值 | 通过 |
| `gateway-server/configs/generated/config.test.template.yaml` | 90/146/166/182 | test 模板同步上述默认值 | 通过 |
| `gateway-server/internal/configdoc/metadata.go` | 85/99/104/108 | 配置元数据 default/validation 统一到新口径 | 通过 |
| `gateway-server/docs/generated/config-params.md` | 55/69/74/78 | 文档默认值列修正到与运行时一致 | 通过 |
| `gateway-server/cmd/gateway/main_config.go` | 238-241 | 启动生成配置说明补充 oversize 风险提示，并与推荐值统一 | 通过 |
| `gateway-server/internal/startupsummary/summary_test.go` | 79-84 | 启动摘要断言值同步到新默认口径 | 通过 |
| `gateway-server/internal/server/embedded-ui/**` | 重建产物 | 使用当前前端源码重新构建并嵌入，清除旧 UI 构建残留 | 通过 |

---

## 4. UI 能力清退/补齐专项结论

### 4.1 直接清退项

运行态不再保留以下旧能力残留：

- `/network/config`
- `/config-governance`
- `/config/transfer`
- `/diagnostics/exports`
- `/system/deployment-mode`

说明：这些接口属于旧前端构建产物残留，现已通过重建 `embedded-ui` 一并清退。

### 4.2 保留并补齐项

保留方向不是“把旧页面救活”，而是：

- 保留当前有效配置体系
- 保留下载/播放/边界路由能力
- 补齐运行态与源码的一致性
- 补齐入口误选容错
- 补齐控制面请求头压缩，减少登录类 POST 失败概率

---

## 5. 验证记录

### 5.1 已完成验证

1. **前端构建成功**：`gateway-ui` 完成安装与构建。
2. **内嵌 UI 重建成功**：`scripts/embed-ui.sh` 已可执行完成嵌入流程。
3. **旧接口残留复查**：前端源码与 `embedded-ui` 中旧接口检索结果为 0。
4. **Go 源码格式化完成**：涉及 Go 文件均已 `gofmt`。

### 5.2 受环境限制未完成验证

1. `go test ./...`
2. `go build ./...`
3. 现场端到端网络压测 / VLC 实播 / 浏览器多并发下载回归

限制原因：

- 本容器 Go 版本为 1.23.2，工程要求 1.25.0。
- 离线环境下无法补齐依赖下载，存在 `go.sum` 解析缺口。

---

## 6. 建议的现场回归顺序

1. 先回归 `web8201` 登录（确认 `POST /cas/login` 不再触发 `udp_request_control_oversize`）。
2. 再回归 1 路 VLC 播放 + 1 路下载。
3. 再做 4 路浏览器并发下载。
4. 最后做“视频播放 + 文件下载”混部回归。
5. 若仍有问题，再根据现场带宽、丢包与 Range 行为调整 `generic_download_*` 系列参数，而不是回退到旧高并发默认值。

---

## 7. 本次交付物说明

- **源码压缩包**：包含本轮已落地修改后的完整源码。
- **本报告**：为行级验收报告，已与源码包同步输出。

