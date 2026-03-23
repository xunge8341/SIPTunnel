# Phase 21：RTP 端口池耗尽与浏览器附带请求治理

本轮修复聚焦两个问题：

1. 浏览器在打开本地映射入口时，会自动追加 `/favicon.ico`、`/robots.txt`、`/apple-touch-icon.png` 等附带请求；这些请求不应和真实业务请求一样触发一整套 `INVITE/INFO/RTP/BYE` 会话。
2. 对端返回的小体积失败响应（如 502/404 的短文本 body）不应强制走 RTP；否则既浪费端口池，也让排障噪声更大。

## 代码调整

### 1）本地映射入口拦截浏览器附带探测
在 `mapping_runtime.go` 中新增 `isBrowserAncillaryRequest()`：

- 仅对 `GET/HEAD`
- 仅对常见浏览器附带路径：
  - `/favicon.ico`
  - `/favicon.svg`
  - `/robots.txt`
  - `/apple-touch-icon.png`
  - `/apple-touch-icon-precomposed.png`
  - `/site.webmanifest`
  - `/browserconfig.xml`
- 且需同时满足浏览器特征（`User-Agent: Mozilla`、`Accept: image/*`、`Sec-Fetch-Dest=image/empty` 之一）

命中后：

- 本地直接返回 `204 No Content`
- 不再进入 GB28181 会话建链
- 访问日志标记为 `browser_probe_suppressed`

### 2）小体积失败响应强制 INLINE
在 `gb28181_tunnel.go` 中新增 `shouldForceInlineResponse()`：

- 若下级回调 RTP 端点无效，强制 `INLINE`
- 若 `status >= 400` 且 `Content-Length <= 4KB`，强制 `INLINE`

这样像 `502` 且 body 只有几十字节的错误，就不会再占用 RTP 回传链。

### 3）自检提示补充 RTP 池耗尽诊断
在 `http.go` 的 `suggestMappingForwardAction()` 中新增：

- 命中 `rtp port pool exhausted` 时，提示检查：
  - 浏览器附带请求风暴
  - 会话并发过高
  - RTP 端口范围过小

## 预期效果

修复后，打开映射入口时：

- `/favicon.ico` 等附带请求不再消耗 RTP 端口池
- 即使真实业务失败，小错误响应优先走 INLINE，不再把 74 字节错误也包装成 RTP
- `mapping test / 自检` 的建议能直接指出 RTP 池耗尽问题
