# Phase 19：隧道映射执行时序与回拨地址修正

## 现象
- 上级域 HTTP 请求进入后，前端报：`请求转发到对端失败：wait response start: context deadline exceeded`
- 抓包未看到下级域返回 `HttpResponseStart`
- 之前还出现过 `parse outgoing sip payload: read headers: EOF`

## 本轮处理

### 1. 明确区分主流程时序
当前隧道映射执行主链路应为：
1. 本级域（上级域角色）收到本地 HTTP 请求
2. 根据隧道映射解析到远端资源编码
3. 向下级域发 `INVITE`
4. 收到 `200 OK` 后发 `ACK`
5. 发送 `INFO(HttpInvoke)`
6. 下级域执行真实 HTTP
7. 下级域回调 `INFO(HttpResponseStart)`
8. 小响应继续 `INFO(HttpResponseInline)`，大响应走 RTP
9. 下级域 `BYE`

`wait response start` 说明链路已经过了 3/4/5，卡在第 7 步或其之前的回拨链路。

### 2. 回拨地址修正
原实现中，`Contact / X-Callback-Addr` 在 `SIPListenIP = 0.0.0.0` 时可能退化到 `127.0.0.1`，导致下级域把 `HttpResponseStart` 回给自己或错误地址。

现在改为：
- 优先使用明确配置的 `SIPListenIP`
- 其次尝试使用已注册 SIP UDP socket 的实际本地地址
- 再根据目标对端地址做路由探测，取系统选路后的本地 IP
- 仅在对端本身就是 loopback 时才回退到 `127.0.0.1`

这样 `INVITE / REGISTER / MESSAGE / SUBSCRIBE` 的 `Contact` 和回调地址都更接近真实可达地址。

### 3. 全链路阶段日志
新增日志阶段：
- `stage=invite_prepare`
- `stage=invite_ok / invite_error / invite_rejected`
- `stage=invoke_info_ok / invoke_info_error / invoke_info_rejected`
- `stage=response_start_timeout / response_start_received / response_start_ok`
- `stage=invite_received`
- `stage=invoke_received`
- `stage=response_start_sent / response_start_send_error`
- `stage=inline_body_sent / inline_body_send_error`
- `stage=rtp_body_sent / rtp_body_send_error`
- `stage=callback send`

这样再出问题时，可以从日志直接看出卡点是在：
- `INVITE` 发不出去
- `INFO(HttpInvoke)` 发不出去
- 下级域没收到 `HttpInvoke`
- 下级域收到了但回调地址不可达
- 回调请求发出但上级域未响应

### 4. 隧道映射只基于远端目录资源
本地资源不再出现在“隧道映射”总览中，避免误用本地资源触发错误链路判断。
