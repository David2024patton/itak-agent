#!/bin/bash
# code-server management script — start, stop, status, configure
set -euo pipefail

CONFIG_FILE="${CODE_SERVER_CONFIG:-$HOME/.config/code-server/config.yaml}"
BINARY=$(command -v code-server 2>/dev/null || echo "$HOME/.local/bin/code-server")

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
CYAN='\033[0;36m'
NC='\033[0m'

log() { echo -e "${GREEN}[code-server]${NC} $1"; }
warn() { echo -e "${YELLOW}[code-server]${NC} $1"; }
err() { echo -e "${RED}[code-server]${NC} $1" >&2; }
info() { echo -e "${CYAN}[code-server]${NC} $1"; }

check_binary() {
  if [[ ! -x "$BINARY" ]]; then
    err "code-server not found. Run: bash scripts/install.sh"
    exit 1
  fi
}

check_config() {
  if [[ ! -f "$CONFIG_FILE" ]]; then
    err "Config not found at $CONFIG_FILE. Run: bash scripts/install.sh"
    exit 1
  fi
}

get_config_value() {
  local key=$1
  grep "^${key}:" "$CONFIG_FILE" 2>/dev/null | sed "s/^${key}: *//" | tr -d '"' || echo ""
}

set_config_value() {
  local key=$1
  local value=$2
  if grep -q "^${key}:" "$CONFIG_FILE" 2>/dev/null; then
    sed -i "s|^${key}:.*|${key}: ${value}|" "$CONFIG_FILE"
  else
    echo "${key}: ${value}" >> "$CONFIG_FILE"
  fi
  log "Set ${key} = ${value}"
}

use_systemd() {
  command -v systemctl &>/dev/null && systemctl --user status code-server &>/dev/null 2>&1
}

case "${1:-help}" in
  start)
    check_binary
    check_config
    if use_systemd; then
      systemctl --user start code-server
      log "✅ code-server started (systemd)"
    else
      # Direct start (background)
      PORT="${2:-}"
      WORKSPACE="${4:-}"
      EXTRA_ARGS=""
      shift || true
      while [[ $# -gt 0 ]]; do
        case $1 in
          --port) EXTRA_ARGS+=" --bind-addr 0.0.0.0:$2"; shift 2 ;;
          --workspace) EXTRA_ARGS+=" $2"; shift 2 ;;
          *) shift ;;
        esac
      done
      nohup "$BINARY" --config "$CONFIG_FILE" $EXTRA_ARGS > /tmp/code-server.log 2>&1 &
      PID=$!
      sleep 2
      if kill -0 $PID 2>/dev/null; then
        log "✅ code-server started (PID: $PID)"
        BIND=$(get_config_value "bind-addr")
        log "   URL: http://${BIND:-localhost:8443}"
      else
        err "Failed to start. Check /tmp/code-server.log"
        tail -5 /tmp/code-server.log 2>/dev/null || true
        exit 1
      fi
    fi
    ;;

  stop)
    if use_systemd; then
      systemctl --user stop code-server
      log "⏹ code-server stopped (systemd)"
    else
      pkill -f "code-server" 2>/dev/null && log "⏹ code-server stopped" || warn "code-server is not running"
    fi
    ;;

  restart)
    if use_systemd; then
      systemctl --user restart code-server
      log "🔄 code-server restarted (systemd)"
    else
      pkill -f "code-server" 2>/dev/null || true
      sleep 1
      bash "$0" start
    fi
    ;;

  status)
    check_binary
    VERSION=$("$BINARY" --version 2>/dev/null | head -1 || echo "unknown")
    echo ""
    info "code-server Status"
    info "===================="
    echo "  Version:  ${VERSION}"
    
    if use_systemd; then
      STATUS=$(systemctl --user is-active code-server 2>/dev/null || echo "inactive")
      echo "  Service:  ${STATUS}"
      if [[ "$STATUS" == "active" ]]; then
        PID=$(systemctl --user show code-server -p MainPID --value 2>/dev/null || echo "?")
        echo "  PID:      ${PID}"
      fi
    else
      PID=$(pgrep -f "code-server" 2>/dev/null | head -1 || echo "")
      if [[ -n "$PID" ]]; then
        echo -e "  Status:   ${GREEN}running${NC}"
        echo "  PID:      ${PID}"
        UPTIME=$(ps -p "$PID" -o etime= 2>/dev/null | tr -d ' ' || echo "?")
        echo "  Uptime:   ${UPTIME}"
      else
        echo -e "  Status:   ${RED}stopped${NC}"
      fi
    fi
    
    if [[ -f "$CONFIG_FILE" ]]; then
      BIND=$(get_config_value "bind-addr")
      AUTH=$(get_config_value "auth")
      CERT=$(get_config_value "cert")
      PROTO="http"
      [[ "$CERT" == "true" ]] && PROTO="https"
      echo "  URL:      ${PROTO}://${BIND:-localhost:8443}"
      echo "  Auth:     ${AUTH:-password}"
      echo "  Config:   ${CONFIG_FILE}"
    fi
    echo ""
    ;;

  enable)
    if command -v systemctl &>/dev/null; then
      systemctl --user enable code-server 2>/dev/null
      # Enable lingering so service runs without login
      loginctl enable-linger "$(whoami)" 2>/dev/null || true
      log "✅ code-server enabled (will start on boot)"
    else
      warn "systemd not available. Use crontab instead:"
      echo "  @reboot $BINARY --config $CONFIG_FILE"
    fi
    ;;

  disable)
    if command -v systemctl &>/dev/null; then
      systemctl --user disable code-server 2>/dev/null
      log "code-server disabled (won't start on boot)"
    fi
    ;;

  enable-user)
    USER="${2:?Usage: manage.sh enable-user USERNAME}"
    if [[ $EUID -ne 0 ]]; then
      err "Requires root: sudo bash manage.sh enable-user $USER"
      exit 1
    fi
    systemctl enable "code-server@${USER}"
    log "✅ code-server enabled for user ${USER}"
    ;;

  version)
    check_binary
    "$BINARY" --version 2>/dev/null
    ;;

  set-password)
    PASSWORD="${2:?Usage: manage.sh set-password PASSWORD}"
    check_config
    set_config_value "password" "$PASSWORD"
    warn "Restart required: bash scripts/manage.sh restart"
    ;;

  set-bind)
    BIND="${2:?Usage: manage.sh set-bind ADDRESS:PORT (e.g. 0.0.0.0:8443)}"
    check_config
    set_config_value "bind-addr" "$BIND"
    warn "Restart required: bash scripts/manage.sh restart"
    ;;

  set-auth)
    AUTH="${2:?Usage: manage.sh set-auth [password|none]}"
    check_config
    if [[ "$AUTH" == "none" ]]; then
      warn "⚠️  Disabling authentication! Only do this on trusted networks."
    fi
    set_config_value "auth" "$AUTH"
    warn "Restart required: bash scripts/manage.sh restart"
    ;;

  set-cert)
    CERT="${2:?Usage: manage.sh set-cert [true|false]}"
    check_config
    set_config_value "cert" "$CERT"
    warn "Restart required: bash scripts/manage.sh restart"
    ;;

  set-cert-file)
    CERT_FILE="${2:?Usage: manage.sh set-cert-file /path/to/cert.pem}"
    check_config
    set_config_value "cert" "$CERT_FILE"
    warn "Restart required: bash scripts/manage.sh restart"
    ;;

  set-key-file)
    KEY_FILE="${2:?Usage: manage.sh set-key-file /path/to/key.pem}"
    check_config
    set_config_value "cert-key" "$KEY_FILE"
    warn "Restart required: bash scripts/manage.sh restart"
    ;;

  set-workspace)
    WS="${2:?Usage: manage.sh set-workspace /path/to/workspace}"
    check_config
    set_config_value "user-data-dir" "$WS"
    warn "Restart required: bash scripts/manage.sh restart"
    ;;

  set-max-memory)
    MEM="${2:?Usage: manage.sh set-max-memory MB}"
    # Set Node.js max memory via environment
    SYSTEMD_DIR="$HOME/.config/systemd/user"
    mkdir -p "${SYSTEMD_DIR}/code-server.service.d"
    cat > "${SYSTEMD_DIR}/code-server.service.d/memory.conf" <<OVERRIDE
[Service]
Environment="NODE_OPTIONS=--max-old-space-size=${MEM}"
OVERRIDE
    systemctl --user daemon-reload 2>/dev/null || true
    log "Max memory set to ${MEM}MB"
    warn "Restart required: bash scripts/manage.sh restart"
    ;;

  set-setting)
    KEY="${2:?Usage: manage.sh set-setting KEY VALUE}"
    VALUE="${3:?Usage: manage.sh set-setting KEY VALUE}"
    SETTINGS_DIR="$HOME/.local/share/code-server/User"
    SETTINGS_FILE="${SETTINGS_DIR}/settings.json"
    mkdir -p "$SETTINGS_DIR"
    if [[ ! -f "$SETTINGS_FILE" ]]; then
      echo '{}' > "$SETTINGS_FILE"
    fi
    # Use jq if available, otherwise python
    if command -v jq &>/dev/null; then
      jq --arg k "$KEY" --arg v "$VALUE" '.[$k] = ($v | try fromjson // $v)' "$SETTINGS_FILE" > "${SETTINGS_FILE}.tmp" && mv "${SETTINGS_FILE}.tmp" "$SETTINGS_FILE"
    else
      warn "jq not found — manually edit ${SETTINGS_FILE}"
    fi
    log "Set VS Code setting: ${KEY} = ${VALUE}"
    ;;

  logs)
    if use_systemd; then
      journalctl --user -u code-server -f --no-pager -n "${2:-50}"
    elif [[ -f /tmp/code-server.log ]]; then
      tail -f -n "${2:-50}" /tmp/code-server.log
    else
      warn "No logs found"
    fi
    ;;

  help|*)
    cat <<EOF
code-server Manager

Usage: bash manage.sh COMMAND [OPTIONS]

Commands:
  start                    Start code-server
  stop                     Stop code-server
  restart                  Restart code-server
  status                   Show status and access info
  enable                   Enable auto-start on boot
  disable                  Disable auto-start
  version                  Show installed version
  logs [N]                 Show last N log lines (default: 50)

Configuration:
  set-password PASSWORD    Set login password
  set-bind ADDRESS:PORT    Set bind address (e.g. 0.0.0.0:8443)
  set-auth [password|none] Set authentication mode
  set-cert [true|false]    Enable/disable self-signed HTTPS
  set-cert-file PATH       Set custom SSL certificate
  set-key-file PATH        Set custom SSL key
  set-workspace PATH       Set default workspace directory
  set-max-memory MB        Set max memory (Node.js heap)
  set-setting KEY VALUE    Set VS Code setting

Admin (requires root):
  enable-user USERNAME     Enable code-server for system user
EOF
    ;;
esac
