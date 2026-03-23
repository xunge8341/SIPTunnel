#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"

export LONGRUN_DURATION="90s"
export LONGRUN_SAMPLE_INTERVAL="5s"

"$ROOT_DIR/scripts/longrun/run.sh" smoke
