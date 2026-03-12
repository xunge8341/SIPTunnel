#!/usr/bin/env bash
set -euo pipefail

# 示例解析器：从探针输出提取指标并拼装 JSONL。
# 实际项目可替换为对现有测试工具输出的解析逻辑。

attempts="${ATTEMPTS:-0}"
successes="${SUCCESSES:-0}"
latency="${AVG_LATENCY_MS:-0}"
retrans="${RETRANSMISSIONS:-0}"
recovery="${RECOVERY_TIME_MS:-0}"

cat <<JSON
{"link":"${NETEM_LINK:-UNKNOWN}","scenario":"${NETEM_CASE:-unknown}","condition":{"delay_ms":${NETEM_DELAY_MS:-0},"jitter_ms":${NETEM_JITTER_MS:-0},"loss_percent":${NETEM_LOSS_PERCENT:-0},"reorder_percent":${NETEM_REORDER_PERCENT:-0},"disconnect_ms":${NETEM_DISCONNECT_MS:-0},"bandwidth_kbps":${NETEM_BANDWIDTH_KBPS:-0}},"attempts":${attempts},"successes":${successes},"avg_latency_ms":${latency},"retransmissions":${retrans},"recovery_time_ms":${recovery}}
JSON
