#!/bin/bash
# Generate Docker deployment for code-server
set -euo pipefail

GREEN='\033[0;32m'
NC='\033[0m'
log() { echo -e "${GREEN}[docker]${NC} $1"; }

PORT=8443
PASSWORD=""
WORKSPACE="$HOME/projects"

while [[ $# -gt 0 ]]; do
  case $1 in
    generate) shift ;;
    --port) PORT="$2"; shift 2 ;;
    --password) PASSWORD="$2"; shift 2 ;;
    --workspace) WORKSPACE="$2"; shift 2 ;;
    *) shift ;;
  esac
done

if [[ -z "$PASSWORD" ]]; then
  PASSWORD=$(openssl rand -base64 16 2>/dev/null || head -c 16 /dev/urandom | base64)
  log "Generated password: ${PASSWORD}"
fi

cat > docker-compose.yml <<COMPOSE
services:
  code-server:
    image: codercom/code-server:latest
    container_name: code-server
    restart: unless-stopped
    ports:
      - "${PORT}:8080"
    environment:
      - PASSWORD=${PASSWORD}
    volumes:
      - ${WORKSPACE}:/home/coder/project
      - code-server-config:/home/coder/.config
      - code-server-local:/home/coder/.local
    user: "\${UID:-1000}:\${GID:-1000}"

volumes:
  code-server-config:
  code-server-local:
COMPOSE

log "✅ docker-compose.yml generated"
log "   Port: ${PORT}"
log "   Workspace: ${WORKSPACE}"
log ""
log "Start: docker compose up -d"
log "Access: http://localhost:${PORT}"
log "Password: ${PASSWORD}"
