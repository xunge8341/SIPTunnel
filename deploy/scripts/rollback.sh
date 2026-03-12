#!/usr/bin/env bash
# SIPTunnel 回滚脚本。
#
# 用法：
#   ./deploy/scripts/rollback.sh                # 回滚到最近备份
#   TARGET_BACKUP=/opt/siptunnel/backups/gateway-xxx.bak ./deploy/scripts/rollback.sh

set -euo pipefail

SERVICE_NAME="${SERVICE_NAME:-siptunnel-gateway}"
INSTALL_DIR="${INSTALL_DIR:-/opt/siptunnel}"
BINARY_PATH="${BINARY_PATH:-${INSTALL_DIR}/gateway}"
BACKUP_DIR="${BACKUP_DIR:-${INSTALL_DIR}/backups}"
TARGET_BACKUP="${TARGET_BACKUP:-}"

log() { echo "[rollback] $*"; }
err() { echo "[rollback][ERROR] $*" >&2; }

main() {
  if [[ -z "${TARGET_BACKUP}" ]]; then
    TARGET_BACKUP="$(ls -1t "${BACKUP_DIR}"/gateway-*.bak 2>/dev/null | head -n 1 || true)"
  fi

  if [[ -z "${TARGET_BACKUP}" || ! -f "${TARGET_BACKUP}" ]]; then
    err "未找到有效备份文件，请设置 TARGET_BACKUP"
    exit 1
  fi

  log "准备回滚到: ${TARGET_BACKUP}"
  systemctl stop "${SERVICE_NAME}.service"
  install -m 0755 "${TARGET_BACKUP}" "${BINARY_PATH}"
  systemctl start "${SERVICE_NAME}.service"
  systemctl status --no-pager "${SERVICE_NAME}.service" || true
  log "回滚完成"
}

main "$@"
