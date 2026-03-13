#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<USAGE
用法:
  $(basename "$0") --base-url <http://127.0.0.1:6060> --token <pprof_token> [options]

选项:
  --base-url URL            pprof 服务地址（默认: http://127.0.0.1:6060）
  --token TOKEN             访问 token（必填）
  --duration SEC            CPU profile 时长，单位秒（默认: 30）
  --out-dir DIR             输出目录（默认: ./artifacts/pprof/<timestamp>）
  --insecure                跳过 TLS 证书校验（仅测试环境）
  -h, --help                显示帮助

输出文件:
  cpu.pb.gz
  heap.pb.gz
  goroutine.pb.gz
  block.pb.gz
  mutex.pb.gz
USAGE
}

BASE_URL="http://127.0.0.1:6060"
TOKEN=""
DURATION=30
OUT_DIR=""
CURL_ARGS=()

while [[ $# -gt 0 ]]; do
  case "$1" in
    --base-url)
      BASE_URL="$2"; shift 2 ;;
    --token)
      TOKEN="$2"; shift 2 ;;
    --duration)
      DURATION="$2"; shift 2 ;;
    --out-dir)
      OUT_DIR="$2"; shift 2 ;;
    --insecure)
      CURL_ARGS+=("-k"); shift ;;
    -h|--help)
      usage; exit 0 ;;
    *)
      echo "未知参数: $1" >&2
      usage
      exit 1 ;;
  esac
done

if [[ -z "$TOKEN" ]]; then
  echo "错误: --token 必填" >&2
  exit 1
fi
if ! [[ "$DURATION" =~ ^[0-9]+$ ]] || [[ "$DURATION" -le 0 ]]; then
  echo "错误: --duration 需为正整数秒" >&2
  exit 1
fi

if [[ -z "$OUT_DIR" ]]; then
  OUT_DIR="./artifacts/pprof/$(date +%Y%m%d-%H%M%S)"
fi
mkdir -p "$OUT_DIR"

fetch() {
  local path="$1"
  local output="$2"
  local url="${BASE_URL}${path}"
  echo "[collect] ${url} -> ${output}"
  curl -fsSL "${CURL_ARGS[@]}" \
    -H "Authorization: Bearer ${TOKEN}" \
    "$url" -o "$output"
}

fetch "/debug/pprof/profile?seconds=${DURATION}" "$OUT_DIR/cpu.pb.gz"
fetch "/debug/pprof/heap" "$OUT_DIR/heap.pb.gz"
fetch "/debug/pprof/goroutine" "$OUT_DIR/goroutine.pb.gz"
fetch "/debug/pprof/block" "$OUT_DIR/block.pb.gz"
fetch "/debug/pprof/mutex" "$OUT_DIR/mutex.pb.gz"

echo "采集完成: $OUT_DIR"
