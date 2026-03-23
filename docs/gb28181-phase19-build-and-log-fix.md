# GB28181 Phase 19 - 编译依赖同步与生产日志缺陷修复

## 本轮落点

- `run_phase1_strict_acceptance.ps1/.sh` 在 `server_targeted` 之前先执行 `go mod tidy -compat=<go.mod go directive>`。
- `rtpfile.Header.UnmarshalBinary` 只做结构解析，不再把当前时钟窗口与业务必填 ID 校验混进反序列化。
- `ValidateEnvelope/ValidatePacket` 继续承担运行态安全校验，补齐针对 skew / missing IDs 的测试。

## 解决的问题

1. Windows 严格验收在 `server_targeted` 阶段因缺少 `go.sum` 直接失败。
2. RTP 文件协议测试被 `UnmarshalBinary` 中的运行态时间窗/必填字段校验误伤，导致离线样本和 TLV 跳过测试失败。
3. 构建链、测试链、发布链对 `go.mod/go.sum` 的同步职责不一致。
