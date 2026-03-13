#!/bin/bash
# code-server extension management
set -euo pipefail

BINARY=$(command -v code-server 2>/dev/null || echo "$HOME/.local/bin/code-server")

GREEN='\033[0;32m'
RED='\033[0;31m'
NC='\033[0m'

log() { echo -e "${GREEN}[extensions]${NC} $1"; }
err() { echo -e "${RED}[extensions]${NC} $1" >&2; }

check_binary() {
  if [[ ! -x "$BINARY" ]]; then
    err "code-server not found. Run: bash scripts/install.sh"
    exit 1
  fi
}

case "${1:-help}" in
  install)
    check_binary
    EXT="${2:?Usage: extensions.sh install EXTENSION_ID}"
    log "Installing ${EXT}..."
    "$BINARY" --install-extension "$EXT" 2>&1
    log "✅ ${EXT} installed"
    ;;

  install-from)
    check_binary
    FILE="${2:?Usage: extensions.sh install-from FILE}"
    if [[ ! -f "$FILE" ]]; then
      err "File not found: $FILE"
      exit 1
    fi
    TOTAL=$(grep -c -v '^\s*$\|^\s*#' "$FILE" || echo 0)
    COUNT=0
    while IFS= read -r ext; do
      [[ -z "$ext" || "$ext" =~ ^# ]] && continue
      ext=$(echo "$ext" | tr -d '[:space:]')
      COUNT=$((COUNT + 1))
      log "[$COUNT/$TOTAL] Installing ${ext}..."
      "$BINARY" --install-extension "$ext" 2>&1 || err "Failed: ${ext}"
    done < "$FILE"
    log "✅ Done — installed ${COUNT} extensions"
    ;;

  uninstall)
    check_binary
    EXT="${2:?Usage: extensions.sh uninstall EXTENSION_ID}"
    "$BINARY" --uninstall-extension "$EXT" 2>&1
    log "🗑 ${EXT} uninstalled"
    ;;

  list)
    check_binary
    log "Installed extensions:"
    "$BINARY" --list-extensions 2>/dev/null
    ;;

  list-versions)
    check_binary
    "$BINARY" --list-extensions --show-versions 2>/dev/null
    ;;

  export)
    check_binary
    FILE="${2:-extensions.txt}"
    "$BINARY" --list-extensions 2>/dev/null > "$FILE"
    COUNT=$(wc -l < "$FILE")
    log "Exported ${COUNT} extensions to ${FILE}"
    ;;

  search)
    QUERY="${2:?Usage: extensions.sh search QUERY}"
    log "Searching Open VSX for '${QUERY}'..."
    RESULTS=$(curl -sL "https://open-vsx.org/api/-/search?query=${QUERY}&size=10" 2>/dev/null)
    if command -v jq &>/dev/null; then
      echo "$RESULTS" | jq -r '.extensions[] | "\(.namespace).\(.name) — \(.displayName // .name) (\(.version))"' 2>/dev/null || err "No results or parse error"
    else
      echo "$RESULTS"
    fi
    ;;

  help|*)
    cat <<EOF
code-server Extension Manager

Usage: bash extensions.sh COMMAND [OPTIONS]

Commands:
  install EXTENSION_ID         Install an extension
  install-from FILE            Install extensions from file (one ID per line)
  uninstall EXTENSION_ID       Uninstall an extension
  list                         List installed extensions
  list-versions                List with versions
  export [FILE]                Export extension list to file
  search QUERY                 Search Open VSX marketplace

Examples:
  bash extensions.sh install ms-python.python
  bash extensions.sh install-from my-extensions.txt
  bash extensions.sh search "tailwind"
  bash extensions.sh export extensions.txt
EOF
    ;;
esac
