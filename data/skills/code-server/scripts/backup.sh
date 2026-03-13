#!/bin/bash
# Backup and restore code-server config + extensions
set -euo pipefail

GREEN='\033[0;32m'
RED='\033[0;31m'
NC='\033[0m'

log() { echo -e "${GREEN}[backup]${NC} $1"; }
err() { echo -e "${RED}[backup]${NC} $1" >&2; }

BINARY=$(command -v code-server 2>/dev/null || echo "$HOME/.local/bin/code-server")
CONFIG_DIR="$HOME/.config/code-server"
DATA_DIR="$HOME/.local/share/code-server"

case "${1:-help}" in
  create)
    OUTPUT="${2:-code-server-backup-$(date +%Y%m%d).tar.gz}"
    TMP=$(mktemp -d)
    trap 'rm -rf "$TMP"' EXIT

    # Export config
    if [[ -d "$CONFIG_DIR" ]]; then
      cp -r "$CONFIG_DIR" "$TMP/config"
      log "Config backed up"
    fi

    # Export extensions list
    if [[ -x "$BINARY" ]]; then
      "$BINARY" --list-extensions 2>/dev/null > "$TMP/extensions.txt"
      log "Extensions list exported ($(wc -l < "$TMP/extensions.txt") extensions)"
    fi

    # Export VS Code settings
    if [[ -d "$DATA_DIR/User" ]]; then
      mkdir -p "$TMP/user-settings"
      cp "$DATA_DIR/User/settings.json" "$TMP/user-settings/" 2>/dev/null || true
      cp "$DATA_DIR/User/keybindings.json" "$TMP/user-settings/" 2>/dev/null || true
      cp -r "$DATA_DIR/User/snippets" "$TMP/user-settings/" 2>/dev/null || true
      log "User settings backed up"
    fi

    tar -czf "$OUTPUT" -C "$TMP" .
    SIZE=$(du -h "$OUTPUT" | cut -f1)
    log "✅ Backup created: ${OUTPUT} (${SIZE})"
    ;;

  restore)
    INPUT="${2:?Usage: backup.sh restore BACKUP_FILE}"
    if [[ ! -f "$INPUT" ]]; then
      err "File not found: $INPUT"
      exit 1
    fi

    TMP=$(mktemp -d)
    trap 'rm -rf "$TMP"' EXIT
    tar -xzf "$INPUT" -C "$TMP"

    # Restore config
    if [[ -d "$TMP/config" ]]; then
      mkdir -p "$CONFIG_DIR"
      cp -r "$TMP/config/"* "$CONFIG_DIR/"
      log "Config restored"
    fi

    # Restore user settings
    if [[ -d "$TMP/user-settings" ]]; then
      mkdir -p "$DATA_DIR/User"
      cp -r "$TMP/user-settings/"* "$DATA_DIR/User/" 2>/dev/null || true
      log "User settings restored"
    fi

    # Reinstall extensions
    if [[ -f "$TMP/extensions.txt" ]]; then
      TOTAL=$(wc -l < "$TMP/extensions.txt")
      log "Installing ${TOTAL} extensions..."
      while IFS= read -r ext; do
        [[ -z "$ext" ]] && continue
        "$BINARY" --install-extension "$ext" 2>/dev/null || err "Failed: $ext"
      done < "$TMP/extensions.txt"
      log "Extensions restored"
    fi

    log "✅ Restore complete. Restart code-server to apply."
    ;;

  help|*)
    cat <<EOF
code-server Backup & Restore

Usage: bash backup.sh COMMAND [OPTIONS]

Commands:
  create [OUTPUT]    Create backup (default: code-server-backup-YYYYMMDD.tar.gz)
  restore FILE       Restore from backup

What's backed up:
  - Config (config.yaml)
  - Extension list
  - VS Code settings, keybindings, snippets
EOF
    ;;
esac
