#!/usr/bin/env bash
# SIPTunnel 升级脚本（支持失败自动回滚）。
#
# 用法：
#   RELEASE_FILE=./dist/gateway-linux-amd64 ./deploy/scripts/upgrade.sh
#
# 行为：
# 1) 为当前版本创建备份
# 2) 停服务 -> 替换二进制 -> 启服务
# 3) 健康检查失败时恢复备份

set -euo pipefail

SERVICE_NAME="${SERVICE_NAME:-siptunnel-gateway}"
INSTALL_DIR="${INSTALL_DIR:-/opt/siptunnel}"
BINARY_PATH="${BINARY_PATH:-${INSTALL_DIR}/gateway}"
BACKUP_DIR="${BACKUP_DIR:-${INSTALL_DIR}/backups}"
RELEASE_FILE="${RELEASE_FILE:-}"
HEALTH_URL="${HEALTH_URL:-http://127.0.0.1:18080/healthz}"

log() { echo "[upgrade] $*"; }
err() { echo "[upgrade][ERROR] $*" >&2; }

timestamp="$(date +%Y%m%d-%H%M%S)"
backup_file="${BACKUP_DIR}/gateway-${timestamp}.bak"

rollback_latest() {
  local latest
  latest="$(ls -1t "${BACKUP_DIR}"/gateway-*.bak 2>/dev/null | head -n 1 || true)"
  if [[ -z "${latest}" ]]; then
    err "未找到可回滚备份"
    return 1
  fi

  log "开始回滚到: ${latest}"
  systemctl stop "${SERVICE_NAME}.service"
  install -m 0755 "${latest}" "${BINARY_PATH}"
  systemctl start "${SERVICE_NAME}.service"
  log "回滚完成"
}

need_cmd() {
  command -v "$1" >/dev/null 2>&1 || { err "缺少命令: $1"; exit 1; }
}

main() {
  need_cmd install
  need_cmd systemctl
  need_cmd curl

  if [[ -z "${RELEASE_FILE}" || ! -f "${RELEASE_FILE}" ]]; then
    err "请通过 RELEASE_FILE 指定新版本二进制"
    exit 1
  fi

  mkdir -p "${BACKUP_DIR}"

  if [[ -f "${BINARY_PATH}" ]]; then
    cp -a "${BINARY_PATH}" "${backup_file}"
    log "已备份当前版本: ${backup_file}"
  else
    log "当前不存在历史二进制，按首次升级处理"
  fi

  systemctl stop "${SERVICE_NAME}.service"
  install -m 0755 "${RELEASE_FILE}" "${BINARY_PATH}"
  systemctl start "${SERVICE_NAME}.service"

  if curl -fsS --max-time 5 "${HEALTH_URL}" >/dev/null; then
    log "升级成功，健康检查通过"
  else
    err "升级后健康检查失败，执行自动回滚"
    rollback_latest
    exit 1
  fi
}

main "$@"
