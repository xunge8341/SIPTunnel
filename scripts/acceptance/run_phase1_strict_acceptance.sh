#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
REPORT_DIR="${ACCEPTANCE_REPORT_DIR:-$ROOT_DIR/artifacts/acceptance}"
TIMESTAMP="$(date +%Y%m%d-%H%M%S)"
REPORT_BASENAME="phase1-strict-acceptance-${TIMESTAMP}"
REPORT_JSON="$REPORT_DIR/${REPORT_BASENAME}.json"
REPORT_MD="$REPORT_DIR/${REPORT_BASENAME}.md"
mkdir -p "$REPORT_DIR"

SKIP_SOURCE="${ACCEPTANCE_SKIP_SOURCE:-false}"
SKIP_UI="${ACCEPTANCE_SKIP_UI:-false}"
SKIP_SERVER_TESTS="${ACCEPTANCE_SKIP_SERVER_TESTS:-false}"
SKIP_SMOKE="${ACCEPTANCE_SKIP_SMOKE:-false}"
SKIP_REGRESSION="${ACCEPTANCE_SKIP_REGRESSION:-false}"

get_go_mod_compat_version() {
  local go_mod="$ROOT_DIR/gateway-server/go.mod"
  if [[ -f "$go_mod" ]]; then
    local version
    version="$(sed -nE 's/^go[[:space:]]+([0-9]+\.[0-9]+(\.[0-9]+)?).*/\1/p' "$go_mod" | head -n1)"
    if [[ -n "$version" ]]; then
      printf '%s\n' "$version"
      return 0
    fi
  fi
  printf '1.23.0\n'
}

sync_go_module_graph() {
  local compat
  compat="$(get_go_mod_compat_version)"
  ( cd "$ROOT_DIR/gateway-server" && go mod tidy "-compat=$compat" )
}

steps=()
failures=0
SEP=$'\x1f'

run_step() {
  local name="$1"
  shift
  local tmp
  tmp="$(mktemp)"
  local start end elapsed status summary output
  start="$(date +%s)"
  if "$@" >"$tmp" 2>&1; then
    status="PASS"
    summary=""
  else
    status="FAIL"
    failures=$((failures + 1))
    summary="$(tail -n 40 "$tmp")"
  fi
  end="$(date +%s)"
  elapsed=$((end - start))
  output="$(cat "$tmp")"
  rm -f "$tmp"
  printf '[%s] %s (%ss)\n' "$status" "$name" "$elapsed"
  if [[ "$status" == "FAIL" ]]; then
    printf '%s\n' "$summary"
  fi
  local out_b64 sum_b64
  out_b64="$(printf '%s' "$output" | base64 | tr -d '\n')"
  sum_b64="$(printf '%s' "$summary" | base64 | tr -d '\n')"
  steps+=("${name}${SEP}${status}${SEP}${elapsed}${SEP}${*}${SEP}${sum_b64}${SEP}${out_b64}")
}

if [[ "$SKIP_SOURCE" != "true" ]]; then
  run_step source_strict "$ROOT_DIR/scripts/acceptance/verify_phase1_strict_source.sh"
fi

if [[ "$SKIP_UI" != "true" ]]; then
  run_step ui_build_guard "$ROOT_DIR/scripts/test-ui-build-guard.sh"
fi

if [[ "$SKIP_SERVER_TESTS" != "true" ]]; then
  run_step server_targeted bash -lc "cd '$ROOT_DIR/gateway-server' && compat=\$(sed -nE 's/^go[[:space:]]+([0-9]+\.[0-9]+(\.[0-9]+)?).*/\1/p' go.mod | head -n1) && compat=\${compat:-1.23.0} && go mod tidy -compat=\"\$compat\" && go test ./internal/protocol/... ./internal/server ./internal/selfcheck ./internal/config -count=1"
fi

if [[ "$SKIP_SMOKE" != "true" ]]; then
  run_step smoke "$ROOT_DIR/scripts/smoke/run.sh"
fi

if [[ "$SKIP_REGRESSION" != "true" ]]; then
  run_step regression_smoke "$ROOT_DIR/scripts/regression/run.sh" smoke
fi

overall="PASS"
if [[ "$failures" -gt 0 ]]; then
  overall="FAIL"
fi

STEPS_PAYLOAD="$(printf '%s\n' "${steps[@]}")"
export STEPS_PAYLOAD REPORT_JSON REPORT_MD overall failures SEP
python3 - <<'PY'
import base64, json, os
from datetime import datetime, timezone

sep = os.environ['SEP']
raw = os.environ.get('STEPS_PAYLOAD', '')
items = []
for line in raw.splitlines():
    if not line:
        continue
    name, status, duration, command, summary_b64, output_b64 = line.split(sep)
    items.append({
        'name': name,
        'status': status,
        'duration_sec': int(duration),
        'command': command,
        'failure_summary': base64.b64decode(summary_b64).decode('utf-8', errors='replace') if summary_b64 else '',
        'output': base64.b64decode(output_b64).decode('utf-8', errors='replace') if output_b64 else '',
    })
report = {
    'generated_at': datetime.now(timezone.utc).strftime('%Y-%m-%dT%H:%M:%SZ'),
    'overall': os.environ['overall'],
    'summary': {
        'total': len(items),
        'pass': sum(1 for i in items if i['status'] == 'PASS'),
        'fail': sum(1 for i in items if i['status'] == 'FAIL'),
    },
    'items': items,
}
with open(os.environ['REPORT_JSON'], 'w', encoding='utf-8') as f:
    json.dump(report, f, ensure_ascii=False, indent=2)
lines = [
    '# GB28181 第一阶段严格模式验收报告',
    '',
    f"- GeneratedAt(Local): `{report['generated_at']}`",
    f"- Overall: **{report['overall']}**",
    '',
    '| 步骤 | 结果 | 耗时(秒) | 失败摘要 |',
    '|---|---|---:|---|',
]
for item in items:
    summary = item['failure_summary'].replace('\n', '<br>') if item['failure_summary'] else '-'
    lines.append(f"| `{item['name']}` | {item['status']} | {item['duration_sec']} | {summary} |")
lines += [
    '',
    '建议执行顺序：源码验收 → UI 构建守护 → 后端定向测试 → smoke → regression smoke。',
    '如需抓包验收，请结合 `scripts/acceptance/pcap_capture.sh` 与 `scripts/acceptance/verify_gb28181_pcap.sh`。',
]
with open(os.environ['REPORT_MD'], 'w', encoding='utf-8') as f:
    f.write('\n'.join(lines))
print(os.environ['REPORT_MD'])
print(os.environ['REPORT_JSON'])
PY

if [[ "$overall" == "FAIL" ]]; then
  exit 1
fi
