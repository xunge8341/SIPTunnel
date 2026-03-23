# 性能基线（Benchmark Baseline）

本文档定义 SIPTunnel 关键路径的 benchmark 运行方式、结果记录格式与基线对比方法，目标是让每次性能回归都可复现、可追踪。

## 1. 覆盖范围

当前关键路径 benchmark：

- SIP JSON decode/validate：`BenchmarkSIPJSONDecodeValidate`
- 签名与验签：`BenchmarkSignerSign` / `BenchmarkSignerVerify`
- RTP header encode/decode：`BenchmarkRTPHeaderEncode` / `BenchmarkRTPHeaderDecode`
- 文件分片：`BenchmarkFileSplit`
- 文件组装：`BenchmarkFileAssemble`
- HTTP 映射与调用封装：`BenchmarkHTTPMapByTemplate` / `BenchmarkHTTPInvokeWrapper`

对应代码位置：

- `gateway-server/internal/protocol/sip/messages_benchmark_test.go`
- `gateway-server/internal/security/signer_benchmark_test.go`
- `gateway-server/internal/rtp/header_benchmark_test.go`
- `gateway-server/internal/protocol/rtpfile/rtpfile_benchmark_test.go`
- `gateway-server/internal/service/httpinvoke/httpinvoke_benchmark_test.go`

## 2. 运行方式（降低噪音）

推荐本地基线命令：

```bash
cd gateway-server
GOMAXPROCS=1 GODEBUG=asyncpreemptoff=1 go test \
  ./internal/protocol/sip \
  ./internal/security \
  ./internal/rtp \
  ./internal/protocol/rtpfile \
  ./internal/service/httpinvoke \
  -run '^$' \
  -bench 'Benchmark(SIPJSONDecodeValidate|Signer(Sign|Verify)|RTPHeader(Encode|Decode)|File(Split|Assemble)|HTTP(MapByTemplate|InvokeWrapper))$' \
  -benchmem \
  -count=5 \
  -benchtime=1s \
  | tee ../artifacts/bench-$(git rev-parse --short HEAD).txt
```

建议：

- 固定 `GOMAXPROCS=1`，减少调度抖动。
- 固定 benchmark 输入（测试中已使用固定时间戳、固定映射模板、固定文件尺寸/分片参数）。
- 使用 `-count=5` 以上做多次采样，避免单次偶然波动。
- 在同一机器、同一电源策略、低后台负载下执行。

## 3. 结果记录格式

Go benchmark 原生输出即可作为记录文件，推荐按提交存档：

- 文件名：`artifacts/bench-<short_sha>.txt`
- 最小字段：
  - benchmark 名称
  - `ns/op`
  - `B/op`
  - `allocs/op`

示例（节选）：

```text
BenchmarkSIPJSONDecodeValidate-1      120000      9500 ns/op      2200 B/op      47 allocs/op
BenchmarkSignerSign-1                1400000       820 ns/op       352 B/op       6 allocs/op
BenchmarkFileAssemble-1                    80  14500000 ns/op  7500000 B/op    1024 allocs/op
```

## 4. 提交间基线对比

推荐使用 `benchstat` 对比两个提交：

```bash
# 安装（一次即可）
go install golang.org/x/perf/cmd/benchstat@latest

# 生成对比输入
GOMAXPROCS=1 go test ./internal/protocol/sip ./internal/security ./internal/rtp ./internal/protocol/rtpfile ./internal/service/httpinvoke -run '^$' -bench 'Benchmark(SIPJSONDecodeValidate|Signer(Sign|Verify)|RTPHeader(Encode|Decode)|File(Split|Assemble)|HTTP(MapByTemplate|InvokeWrapper))$' -benchmem -count=8 -benchtime=1s > /tmp/old.txt
# 切换到新提交后执行同一命令
GOMAXPROCS=1 go test ./internal/protocol/sip ./internal/security ./internal/rtp ./internal/protocol/rtpfile ./internal/service/httpinvoke -run '^$' -bench 'Benchmark(SIPJSONDecodeValidate|Signer(Sign|Verify)|RTPHeader(Encode|Decode)|File(Split|Assemble)|HTTP(MapByTemplate|InvokeWrapper))$' -benchmem -count=8 -benchtime=1s > /tmp/new.txt

benchstat /tmp/old.txt /tmp/new.txt
```

判定建议：

- 核心指标优先看 `ns/op`，其次看 `B/op` 和 `allocs/op`。
- 若 `ns/op` 回退 > 10% 且统计显著，建议作为性能回归处理。
- 对 `FileSplit`/`FileAssemble` 这类大对象路径，若 `B/op` 或 `allocs/op` 明显上升，也应阻断合并并排查。

## 5. CI smoke 策略

CI 已加入低强度 benchmark smoke（`-benchtime=100ms -count=1`），用于保证：

- 基准函数可编译、可执行。
- 关键路径没有明显功能性退化（例如 panic、不可用）。

说明：CI smoke 不作为严格性能门禁；正式性能评估以本地或专用性能环境的多次采样结果为准。
