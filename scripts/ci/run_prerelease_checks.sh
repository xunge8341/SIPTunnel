#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
source "$ROOT_DIR/scripts/ci/common.sh"

run_gate "regression_full" bash -lc 'cd scripts && ./regression/run.sh full'
run_gate "selfcheck_suite" bash -lc 'cd gateway-server && go test ./internal/selfcheck ./cmd/gatewayctl -run "Test(RunnerRun_|CollectDiagnostics|ValidateConfig)" -count=1'
run_gate "diagnostics_sampling" bash -lc 'cd gateway-server && go test ./internal/diagnostics ./internal/observability -count=1'

LOG_DIR_PATH="$LOG_DIR" python3 - <<'PY' > "$REPORT_DIR/log-summary.md"
from pathlib import Path
import os

log_dir = Path(os.environ['LOG_DIR_PATH'])
lines = ["# 预发布日志摘要", ""]
for f in sorted(log_dir.glob("*.log")):
    txt = f.read_text(encoding="utf-8", errors="replace").splitlines()
    lines.append(f"## {f.name}")
    lines.append("```text")
    lines.extend(txt[-30:] if txt else ["<empty>"])
    lines.append("```")
    lines.append("")
print("\n".join(lines))
PY

# 回归脚本已输出 JSON/Markdown 报告，复制到统一 artifacts 目录，便于上传。
if [[ -d "$ROOT_DIR/artifacts/regression" ]]; then
  cp -r "$ROOT_DIR/artifacts/regression" "$REPORT_DIR/" || true
fi

write_report "prerelease-gates"

if [[ "$FAILURES" -gt 0 ]]; then
  exit 1
fi
