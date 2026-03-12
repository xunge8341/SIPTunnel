# 配置参数手册（自动生成）

> 由 `go run ./cmd/configdocgen` 生成，请勿手动编辑。

高风险网络参数会标记为 `⚠️ HIGH-NET`，变更前请执行联调与端口占用检查。

| 参数名 | 类型 | 默认值 | 热更新 | 风险等级 | 说明 |
|---|---|---|---|---|---|
| `server.port` | `int` | `18080` | 否 | MEDIUM | 网关 HTTP 管理端口。 |
| `storage.temp_dir` | `string` | `./data/temp` | 否 | LOW | 文件分片临时目录。 |
| `storage.final_dir` | `string` | `./data/final` | 否 | LOW | 文件组装完成目录。 |
| `storage.audit_dir` | `string` | `./data/audit` | 否 | LOW | 审计日志落盘目录。 |
| `storage.log_dir` | `string` | `./data/logs` | 否 | LOW | 运行日志目录。 |
| `network.sip.enabled` | `bool` | `true` | 是 | MEDIUM | 启用 SIP 控制面。 |
| `network.sip.listen_ip` | `string` | `0.0.0.0` | 否 | ⚠️ HIGH-NET | SIP 监听 IP。 |
| `network.sip.listen_port` | `int` | `5060` | 否 | ⚠️ HIGH-NET | SIP 监听端口。 |
| `network.sip.transport` | `string` | `TCP` | 否 | ⚠️ HIGH-NET | SIP 传输层协议（TCP/UDP/TLS）。 |
| `network.sip.advertise_ip` | `string` | `""` | 否 | MEDIUM | SIP 对端可见地址。 |
| `network.sip.domain` | `string` | `""` | 是 | LOW | SIP 域名。 |
| `network.sip.max_message_bytes` | `int` | `65535` | 是 | ⚠️ HIGH-NET | SIP 最大报文大小（UDP 超 1300 存在分片风险）。 |
| `network.sip.read_timeout_ms` | `int` | `5000` | 是 | MEDIUM | SIP 读超时（毫秒）。 |
| `network.sip.write_timeout_ms` | `int` | `5000` | 是 | MEDIUM | SIP 写超时（毫秒）。 |
| `network.sip.idle_timeout_ms` | `int` | `60000` | 是 | LOW | SIP 空闲连接超时（毫秒）。 |
| `network.rtp.enabled` | `bool` | `true` | 是 | MEDIUM | 启用 RTP 文件面。 |
| `network.rtp.listen_ip` | `string` | `0.0.0.0` | 否 | ⚠️ HIGH-NET | RTP 监听 IP。 |
| `network.rtp.advertise_ip` | `string` | `""` | 否 | MEDIUM | RTP 对端可见地址。 |
| `network.rtp.port_start` | `int` | `20000` | 否 | ⚠️ HIGH-NET | RTP 端口池起始端口。 |
| `network.rtp.port_end` | `int` | `20100` | 否 | ⚠️ HIGH-NET | RTP 端口池结束端口。 |
| `network.rtp.transport` | `string` | `UDP` | 否 | ⚠️ HIGH-NET | RTP 传输协议（当前仅 UDP 正式上线）。 |
| `network.rtp.max_packet_bytes` | `int` | `1400` | 是 | ⚠️ HIGH-NET | RTP 单包大小。 |
| `network.rtp.max_inflight_transfers` | `int` | `64` | 是 | MEDIUM | 并发传输上限。 |
| `network.rtp.receive_buffer_bytes` | `int` | `4194304` | 是 | MEDIUM | RTP 接收缓冲区大小。 |
| `network.rtp.transfer_timeout_ms` | `int` | `30000` | 是 | MEDIUM | 文件传输超时（毫秒）。 |
| `network.rtp.retransmit_max_rounds` | `int` | `3` | 是 | LOW | 重传最大轮次。 |
| `media.port_range.start` | `int` | `20000` | 否 | MEDIUM | 部署规划媒体端口起始值。 |
| `media.port_range.end` | `int` | `20100` | 否 | MEDIUM | 部署规划媒体端口结束值。 |
| `node.role` | `string` | `receiver` | 否 | MEDIUM | 节点角色（receiver/sender）。 |
