#!/usr/bin/env bash
# SIPTunnel 安装脚本（可重复执行）。
#
# 功能：
# 1) 创建用户/目录
# 2) 安装二进制、配置模板、systemd unit
# 3) 执行安装前检查（可禁用）
# 4) 启动并启用 systemd 服务
#
# 用法示例：
#   RELEASE_FILE=./dist/gateway-linux-amd64 ./deploy/scripts/install.sh
#   SKIP_PREFLIGHT=true ./deploy/scripts/install.sh

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"

SERVICE_NAME="${SERVICE_NAME:-siptunnel-gateway}"
APP_USER="${APP_USER:-siptunnel}"
APP_GROUP="${APP_GROUP:-siptunnel}"
INSTALL_DIR="${INSTALL_DIR:-/opt/siptunnel}"
DATA_DIR="${DATA_DIR:-/var/lib/siptunnel}"
CONFIG_PATH="${CONFIG_PATH:-${INSTALL_DIR}/config.yaml}"
BINARY_PATH="${BINARY_PATH:-${INSTALL_DIR}/gateway}"
RELEASE_FILE="${RELEASE_FILE:-${ROOT_DIR}/dist/gateway-linux-amd64}"
SKIP_PREFLIGHT="${SKIP_PREFLIGHT:-false}"

log() { echo "[install] $*"; }
err() { echo "[install][ERROR] $*" >&2; }

need_cmd() {
  command -v "$1" >/dev/null 2>&1 || { err "缺少命令: $1"; exit 1; }
}

ensure_group() {
  if getent group "${APP_GROUP}" >/dev/null 2>&1; then
    log "组已存在: ${APP_GROUP}"
  else
    groupadd --system "${APP_GROUP}"
    log "已创建组: ${APP_GROUP}"
  fi
}

ensure_user() {
  if id -u "${APP_USER}" >/dev/null 2>&1; then
    log "用户已存在: ${APP_USER}"
  else
    useradd --system --gid "${APP_GROUP}" --home-dir "${INSTALL_DIR}" --shell /usr/sbin/nologin "${APP_USER}"
    log "已创建用户: ${APP_USER}"
  fi
}

install_binary() {
  if [[ ! -f "${RELEASE_FILE}" ]]; then
    err "未找到发布文件: ${RELEASE_FILE}"
    exit 1
  fi

  install -d -o "${APP_USER}" -g "${APP_GROUP}" -m 0755 "${INSTALL_DIR}" "${DATA_DIR}" \
    "${DATA_DIR}/temp" "${DATA_DIR}/final" "${DATA_DIR}/audit" "${DATA_DIR}/logs"

  install -o "${APP_USER}" -g "${APP_GROUP}" -m 0755 "${RELEASE_FILE}" "${BINARY_PATH}"
  log "已安装二进制: ${BINARY_PATH}"
}

install_config_template() {
  local template="${ROOT_DIR}/gateway-server/configs/config.yaml"
  if [[ ! -f "${CONFIG_PATH}" ]]; then
    install -D -o "${APP_USER}" -g "${APP_GROUP}" -m 0640 "${template}" "${CONFIG_PATH}"
    log "已初始化配置文件: ${CONFIG_PATH}"
  else
    log "配置文件已存在，保留原样: ${CONFIG_PATH}"
  fi
}

install_unit_template() {
  local unit_template="${ROOT_DIR}/deploy/systemd/siptunnel-gateway.service"
  local target="/etc/systemd/system/${SERVICE_NAME}.service"

  sed \
    -e "s#__APP_USER__#${APP_USER}#g" \
    -e "s#__APP_GROUP__#${APP_GROUP}#g" \
    -e "s#__BINARY_PATH__#${BINARY_PATH}#g" \
    -e "s#__CONFIG_PATH__#${CONFIG_PATH}#g" \
    -e "s#__DATA_DIR__#${DATA_DIR}#g" \
    "${unit_template}" > "${target}"

  chmod 0644 "${target}"
  log "已安装 systemd unit: ${target}"
}

main() {
  need_cmd install
  need_cmd systemctl
  need_cmd sed

  if [[ "${SKIP_PREFLIGHT}" != "true" ]]; then
    "${ROOT_DIR}/deploy/scripts/precheck.sh" all
  else
    log "已跳过预检查（SKIP_PREFLIGHT=true）"
  fi

  ensure_group
  ensure_user
  install_binary
  install_config_template
  install_unit_template

  systemctl daemon-reload
  systemctl enable --now "${SERVICE_NAME}.service"
  systemctl status --no-pager "${SERVICE_NAME}.service" || true

  log "安装完成"
}

main "$@"
