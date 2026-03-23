#!/usr/bin/env bash
# SIPTunnel 部署前检查脚本。
# 提供 4 类可读性强的检查：
#   - config validate
#   - env inspect
#   - storage check
#   - network check
#
# 用法：
#   ./deploy/scripts/precheck.sh all
#   ./deploy/scripts/precheck.sh config validate
#   ./deploy/scripts/precheck.sh env inspect
#   ./deploy/scripts/precheck.sh storage check
#   ./deploy/scripts/precheck.sh network check

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
APP_USER="${APP_USER:-siptunnel}"
APP_GROUP="${APP_GROUP:-siptunnel}"
INSTALL_DIR="${INSTALL_DIR:-/opt/siptunnel}"
DATA_DIR="${DATA_DIR:-/var/lib/siptunnel}"
CONFIG_PATH="${CONFIG_PATH:-${INSTALL_DIR}/config.yaml}"
BINARY_PATH="${BINARY_PATH:-${INSTALL_DIR}/gateway}"
SERVICE_NAME="${SERVICE_NAME:-siptunnel-gateway}"
LISTEN_PORT="${LISTEN_PORT:-${GATEWAY_PORT:-18080}}"
MEDIA_PORT_START="${MEDIA_PORT_START:-20000}"
MEDIA_PORT_END="${MEDIA_PORT_END:-20100}"
NODE_ROLE="${NODE_ROLE:-receiver}"

green='\033[0;32m'
yellow='\033[1;33m'
red='\033[0;31m'
blue='\033[0;34m'
reset='\033[0m'

log_info() { printf "%b[INFO]%b %s\n" "$blue" "$reset" "$*"; }
log_warn() { printf "%b[WARN]%b %s\n" "$yellow" "$reset" "$*"; }
log_ok() { printf "%b[ OK ]%b %s\n" "$green" "$reset" "$*"; }
log_err() { printf "%b[ERR ]%b %s\n" "$red" "$reset" "$*"; }

is_integer() {
  [[ "$1" =~ ^[0-9]+$ ]]
}

check_port_value() {
  local name="$1"
  local value="$2"
  if ! is_integer "$value"; then
    log_err "${name} 必须是整数，当前值: ${value}"
    return 1
  fi
  if (( value < 1 || value > 65535 )); then
    log_err "${name} 必须在 [1,65535]，当前值: ${value}"
    return 1
  fi
  return 0
}

port_in_use() {
  local port="$1"
  if command -v ss >/dev/null 2>&1; then
    ss -ltn "sport = :$port" | tail -n +2 | grep -q .
    return $?
  fi

  if command -v lsof >/dev/null 2>&1; then
    lsof -iTCP:"$port" -sTCP:LISTEN -Pn | tail -n +2 | grep -q .
    return $?
  fi

  if command -v netstat >/dev/null 2>&1; then
    netstat -lnt 2>/dev/null | awk '{print $4}' | grep -Eq "(^|:)$port$"
    return $?
  fi

  return 2
}

check_env_inspect() {
  log_info "执行 env inspect"
  cat <<REPORT
---------------- ENV ----------------
SERVICE_NAME      : ${SERVICE_NAME}
APP_USER/APP_GROUP: ${APP_USER}/${APP_GROUP}
INSTALL_DIR       : ${INSTALL_DIR}
DATA_DIR          : ${DATA_DIR}
BINARY_PATH       : ${BINARY_PATH}
CONFIG_PATH       : ${CONFIG_PATH}
LISTEN_PORT       : ${LISTEN_PORT}
MEDIA_PORT_RANGE  : ${MEDIA_PORT_START}-${MEDIA_PORT_END}
NODE_ROLE         : ${NODE_ROLE}
-------------------------------------
REPORT
  log_ok "env inspect 完成"
}

check_config_validate() {
  log_info "执行 config validate"

  if [[ ! -f "${CONFIG_PATH}" ]]; then
    log_err "未找到配置文件: ${CONFIG_PATH}"
    return 1
  fi

  local required_keys=(
    "server:"
    "storage:"
    "media:"
    "node:"
  )

  local key
  for key in "${required_keys[@]}"; do
    if ! grep -Eq "^[[:space:]]*${key}[[:space:]]*$" "${CONFIG_PATH}"; then
      log_warn "未在配置中定位段落: ${key}（建议人工复核）"
    fi
  done

  if [[ ! -s "${CONFIG_PATH}" ]]; then
    log_err "配置文件为空: ${CONFIG_PATH}"
    return 1
  fi

  if grep -Eq "^[[:space:]]*listen_addr:[[:space:]]*$" "${CONFIG_PATH}"; then
    log_err "存在空 listen_addr 配置，请修复后重试"
    return 1
  fi

  log_ok "配置基础校验通过（存在性/非空/关键字段）"
}

check_storage_check() {
  log_info "执行 storage check"

  local dirs=(
    "${INSTALL_DIR}"
    "${DATA_DIR}"
    "${DATA_DIR}/temp"
    "${DATA_DIR}/final"
    "${DATA_DIR}/audit"
    "${DATA_DIR}/logs"
  )

  local d
  for d in "${dirs[@]}"; do
    if mkdir -p "${d}"; then
      log_ok "目录存在且可创建: ${d}"
    else
      log_err "目录创建失败: ${d}"
      return 1
    fi

    if [[ -w "${d}" ]]; then
      log_ok "目录可写: ${d}"
    else
      log_err "目录不可写: ${d}"
      return 1
    fi
  done
}

check_network_check() {
  log_info "执行 network check"

  check_port_value "LISTEN_PORT" "${LISTEN_PORT}" || return 1
  check_port_value "MEDIA_PORT_START" "${MEDIA_PORT_START}" || return 1
  check_port_value "MEDIA_PORT_END" "${MEDIA_PORT_END}" || return 1

  if (( MEDIA_PORT_START > MEDIA_PORT_END )); then
    log_err "MEDIA_PORT_START 不能大于 MEDIA_PORT_END"
    return 1
  fi

  if [[ "${NODE_ROLE}" != "receiver" && "${NODE_ROLE}" != "sender" ]]; then
    log_err "NODE_ROLE 仅允许 receiver/sender，当前值: ${NODE_ROLE}"
    return 1
  fi

  if port_in_use "${LISTEN_PORT}"; then
    log_err "监听端口已被占用: ${LISTEN_PORT}"
    return 1
  else
    case "$?" in
      1) log_ok "监听端口可用: ${LISTEN_PORT}" ;;
      2) log_warn "未找到 ss/lsof/netstat，无法检测端口占用" ;;
    esac
  fi

  log_ok "network check 完成"
}

run_all() {
  check_env_inspect
  check_config_validate
  check_storage_check
  check_network_check
  log_ok "全部预检查通过"
}

main() {
  local cmd1="${1:-all}"
  local cmd2="${2:-}"

  case "${cmd1} ${cmd2}" in
    "all "|"all all") run_all ;;
    "config validate") check_config_validate ;;
    "env inspect") check_env_inspect ;;
    "storage check") check_storage_check ;;
    "network check") check_network_check ;;
    *)
      cat <<USAGE
用法:
  $0 all
  $0 config validate
  $0 env inspect
  $0 storage check
  $0 network check
USAGE
      return 1
      ;;
  esac
}

main "$@"
