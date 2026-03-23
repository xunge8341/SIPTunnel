#!/usr/bin/env bash
set -euo pipefail
REPO_ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
GATEWAY_DIR="$REPO_ROOT/gateway-server"
OUTPUT_ROOT=${OUTPUT_ROOT:-"$REPO_ROOT/artifacts/acceptance/task9_keepalive_ab"}
HTTP_URL=${HTTP_URL:-"http://127.0.0.1:18080/demo/process"}
MAPPING_URL=${MAPPING_URL:-"$HTTP_URL"}
DURATION=${DURATION:-"30s"}
CONCURRENCY_SET=${CONCURRENCY_SET:-"3 5 10"}
GATEWAY_LOG_SOURCE=${GATEWAY_LOG_SOURCE:-""}
mkdir -p "$OUTPUT_ROOT"; MANIFEST="$OUTPUT_ROOT/experiment_manifest.json"; RUNS_JSON="$OUTPUT_ROOT/runs.jsonl"; : > "$RUNS_JSON"
pushd "$GATEWAY_DIR" >/dev/null
for variant in keepalive_workaround_on keepalive_workaround_off; do
  for concurrency in $CONCURRENCY_SET; do
    run_name="${variant}_${concurrency}"; out_dir="$OUTPUT_ROOT/$run_name"; mkdir -p "$out_dir"
    if [[ "$variant" == "keepalive_workaround_on" ]]; then export GATEWAY_DISABLE_HTTP_KEEPALIVES_MAPPING_FORWARD_CLIENT=true; else export GATEWAY_DISABLE_HTTP_KEEPALIVES_MAPPING_FORWARD_CLIENT=false; fi
    go run ./cmd/loadtest -targets mapping-forward -concurrency "$concurrency" -duration "$DURATION" -http-url "$HTTP_URL" -mapping-url "$MAPPING_URL" -output-dir "$out_dir" >/tmp/${run_name}_loadtest.log
    summary_file=$(find "$out_dir" -name summary.json | sort | tail -n 1); gateway_log_file="$out_dir/gateway.log"
    if [[ -n "$GATEWAY_LOG_SOURCE" && -f "$GATEWAY_LOG_SOURCE" ]]; then cp "$GATEWAY_LOG_SOURCE" "$gateway_log_file"; else : > "$gateway_log_file"; fi
    python - <<PY >> "$RUNS_JSON"
import json
print(json.dumps({"name":"$run_name","variant":"$variant","scenario":"keepalive_ab","concurrency":int($concurrency),"summary_file":"$summary_file","gateway_log_file":"$gateway_log_file"}, ensure_ascii=False))
PY
  done
done
python - <<PY
import json, pathlib
runs=[json.loads(line) for line in pathlib.Path("$RUNS_JSON").read_text().splitlines() if line.strip()]
path=pathlib.Path("$MANIFEST"); path.write_text(json.dumps({"kind":"task9_keepalive_ab","runs":runs}, ensure_ascii=False, indent=2))
PY
go run ./cmd/loadtest -analyze-experiment "$MANIFEST" -experiment-output "$OUTPUT_ROOT/task9_keepalive_ab_report.md"
popd >/dev/null
echo "$OUTPUT_ROOT/task9_keepalive_ab_report.md"
