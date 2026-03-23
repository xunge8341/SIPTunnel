# GB28181 Phase1 Strict Mode

当前实现仅保留严格模式。

- 控制面：`MESSAGE + Application/MANSCDP+xml`
- 目录订阅：`SUBSCRIBE / NOTIFY`
- 媒体协商：`INVITE / 200 OK / ACK`，SDP 使用 `m=video` 与 `a=rtpmap:96 PS/90000`
- 密码字段：只写；读接口返回 `register_auth_password_configured`

本阶段不再将私有会话内控制报文作为线上主路径。
