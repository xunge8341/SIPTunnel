#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<USAGE
用法:
  $(basename "$0") --profile <profile.pb.gz> [--out flame.folded]

说明:
  1) 若安装了 inferno-collapse-pprof，导出标准 folded stack（flamegraph.pl 可直接消费）
  2) 若未安装 inferno-collapse-pprof，导出 pprof protobuf（.pb）作为火焰图中间输入

依赖:
  - go tool pprof
  - 可选: inferno-collapse-pprof
USAGE
}

PROFILE=""
OUT=""

while [[ $# -gt 0 ]]; do
  case "$1" in
    --profile)
      PROFILE="$2"; shift 2 ;;
    --out)
      OUT="$2"; shift 2 ;;
    -h|--help)
      usage; exit 0 ;;
    *)
      echo "未知参数: $1" >&2
      usage
      exit 1 ;;
  esac
done

if [[ -z "$PROFILE" ]]; then
  echo "错误: --profile 必填" >&2
  exit 1
fi
if [[ ! -f "$PROFILE" ]]; then
  echo "错误: profile 文件不存在: $PROFILE" >&2
  exit 1
fi

if ! go tool pprof -top "$PROFILE" >/dev/null 2>&1; then
  echo "错误: go tool pprof 无法解析 profile: $PROFILE" >&2
  exit 1
fi

if command -v inferno-collapse-pprof >/dev/null 2>&1; then
  if [[ -z "$OUT" ]]; then
    OUT="${PROFILE%.pb.gz}.folded"
  fi
  inferno-collapse-pprof < "$PROFILE" > "$OUT"
  echo "导出完成(标准 folded): $OUT"
  echo "可继续执行: flamegraph.pl $OUT > ${OUT}.svg"
  exit 0
fi

if [[ -z "$OUT" ]]; then
  OUT="${PROFILE%.pb.gz}.pb"
fi
go tool pprof -proto "$PROFILE" > "$OUT"
echo "导出完成(pprof protobuf): $OUT"
echo "提示: 未检测到 inferno-collapse-pprof，未生成 folded 栈。"
echo "可安装后重试: cargo install inferno"
