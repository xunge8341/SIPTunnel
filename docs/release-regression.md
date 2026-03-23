# 发布前回归测试套件

SIPTunnel 提供统一回归入口：

```bash
./scripts/regression.sh [local|smoke|full]
```

## Profile 说明

- `local`：本地快速回归（开发自测，分钟级）。
- `smoke`：CI 冒烟回归（覆盖主链路，稳定/可重复）。
- `full`：发布机全量回归（包含 `go test ./...`）。

## 覆盖矩阵

回归套件统一覆盖以下测试项：

1. `command_chain`：基础 command 链路。
2. `file_chain`：基础 file 链路。
3. `sip_tcp`：SIP TCP。
4. `rtp_udp`：RTP UDP。
5. `rtp_tcp`：RTP TCP（若项目存在对应测试则自动纳入）。
6. `config_validate`：配置校验。
7. `selfcheck`：自检能力。
8. `api_smoke`：关键 API smoke。

`full` 额外增加：

9. `repo_full`：`go test ./...`。

## 报告输出

每次执行会在 `artifacts/regression/` 下生成两份报告：

- Markdown：`regression-<profile>-<timestamp>.md`
- JSON：`regression-<profile>-<timestamp>.json`

报告字段包括：

- 测试项
- 结果（PASS/FAIL）
- 耗时（秒）
- 失败摘要（失败时展示尾部日志）

## CI 接入示例

```bash
./scripts/regression.sh smoke
```

如需保留报告到 CI Artifact，可归档目录：

```bash
artifacts/regression/
```
