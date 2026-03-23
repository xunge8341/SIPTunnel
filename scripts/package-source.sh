#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
OUT_DIR="${1:-$ROOT_DIR/dist/source}"
STAMP="$(date +%Y%m%d-%H%M%S)"
OUT_FILE="$OUT_DIR/siptunnel-source-$STAMP.zip"
mkdir -p "$OUT_DIR"

python3 - "$ROOT_DIR" "$OUT_FILE" <<'PY'
import sys
import zipfile
from pathlib import Path

root = Path(sys.argv[1]).resolve()
out = Path(sys.argv[2]).resolve()
exclude_dirs = {'.git', '.idea', '.vscode', '__pycache__', 'artifacts', 'dist'}
exclude_roots = {'gateway-ui/node_modules', 'gateway-ui/dist'}
exclude_suffixes = {'.zip', '.log', '.pcap'}
exclude_name_tokens = ('验收报告', '复核报告', '决策落地', 'review_optimized_source', 'optimized_source', 'entry_chain_review')

with zipfile.ZipFile(out, 'w', compression=zipfile.ZIP_DEFLATED, compresslevel=9) as zf:
    for path in root.rglob('*'):
        if path.is_dir():
            continue
        rel = path.relative_to(root)
        rel_posix = rel.as_posix()
        if any(part in exclude_dirs for part in rel.parts):
            continue
        if any(rel_posix == prefix or rel_posix.startswith(prefix + '/') for prefix in exclude_roots):
            continue
        if path.suffix.lower() in exclude_suffixes:
            continue
        if rel.parent == Path('.') and any(token in path.name for token in exclude_name_tokens):
            continue
        zf.write(path, rel_posix)
print(out)
PY
