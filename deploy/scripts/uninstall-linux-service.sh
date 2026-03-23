#!/usr/bin/env bash
set -euo pipefail

SERVICE_NAME="${SERVICE_NAME:-siptunnel-gateway}"
INSTALL_DIR="${INSTALL_DIR:-/opt/siptunnel}"
DATA_DIR="${DATA_DIR:-/var/lib/siptunnel}"

log() { echo "[uninstall-linux-service] $*"; }

if command -v systemctl >/dev/null 2>&1; then
  systemctl disable --now "${SERVICE_NAME}.service" >/dev/null 2>&1 || true
  rm -f "/etc/systemd/system/${SERVICE_NAME}.service"
  systemctl daemon-reload || true
fi

log "服务已卸载: ${SERVICE_NAME}"
log "安装目录保留: ${INSTALL_DIR}"
log "数据目录保留: ${DATA_DIR}"
log "如需彻底清理，请手工删除以上目录"
