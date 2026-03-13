#!/bin/bash
# Generate nginx reverse proxy config for code-server
set -euo pipefail

GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m'

log() { echo -e "${GREEN}[nginx]${NC} $1"; }
warn() { echo -e "${YELLOW}[nginx]${NC} $1"; }
err() { echo -e "${RED}[nginx]${NC} $1" >&2; }

DOMAIN=""
PORT=8443
USE_SSL=false
OUTPUT="/etc/nginx/sites-available/code-server"

while [[ $# -gt 0 ]]; do
  case $1 in
    generate) shift ;;
    --domain) DOMAIN="$2"; shift 2 ;;
    --port) PORT="$2"; shift 2 ;;
    --ssl) USE_SSL=true; shift ;;
    --output) OUTPUT="$2"; shift 2 ;;
    *) shift ;;
  esac
done

if [[ -z "$DOMAIN" ]]; then
  err "Usage: bash nginx-proxy.sh generate --domain code.example.com [--port 8443] [--ssl]"
  exit 1
fi

if ! command -v nginx &>/dev/null; then
  err "nginx not installed. Install with: sudo apt install nginx"
  exit 1
fi

# Generate config
if $USE_SSL; then
  CONFIG="server {
    listen 80;
    server_name ${DOMAIN};
    return 301 https://\$server_name\$request_uri;
}

server {
    listen 443 ssl http2;
    server_name ${DOMAIN};

    ssl_certificate /etc/letsencrypt/live/${DOMAIN}/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/${DOMAIN}/privkey.pem;
    ssl_protocols TLSv1.2 TLSv1.3;
    ssl_ciphers HIGH:!aNULL:!MD5;
    ssl_prefer_server_ciphers on;

    location / {
        proxy_pass http://127.0.0.1:${PORT};
        proxy_http_version 1.1;
        proxy_set_header Upgrade \$http_upgrade;
        proxy_set_header Connection \"upgrade\";
        proxy_set_header Host \$host;
        proxy_set_header X-Real-IP \$remote_addr;
        proxy_set_header X-Forwarded-For \$proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto \$scheme;
        proxy_set_header Accept-Encoding gzip;
        proxy_read_timeout 86400;
        proxy_send_timeout 86400;
    }
}"
else
  CONFIG="server {
    listen 80;
    server_name ${DOMAIN};

    location / {
        proxy_pass http://127.0.0.1:${PORT};
        proxy_http_version 1.1;
        proxy_set_header Upgrade \$http_upgrade;
        proxy_set_header Connection \"upgrade\";
        proxy_set_header Host \$host;
        proxy_set_header X-Real-IP \$remote_addr;
        proxy_set_header X-Forwarded-For \$proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto \$scheme;
        proxy_set_header Accept-Encoding gzip;
        proxy_read_timeout 86400;
        proxy_send_timeout 86400;
    }
}"
fi

# Write config
if [[ "$OUTPUT" == /etc/* ]]; then
  if [[ $EUID -ne 0 ]]; then
    warn "Writing to ${OUTPUT} requires root. Outputting to stdout:"
    echo ""
    echo "$CONFIG"
    echo ""
    log "Save this to ${OUTPUT} and run:"
    log "  sudo ln -sf ${OUTPUT} /etc/nginx/sites-enabled/code-server"
    log "  sudo nginx -t && sudo systemctl reload nginx"
    if $USE_SSL; then
      log ""
      log "For SSL, first run:"
      log "  sudo certbot certonly --nginx -d ${DOMAIN}"
    fi
    exit 0
  fi
fi

echo "$CONFIG" > "$OUTPUT"
log "Config written to ${OUTPUT}"

# Enable site
if [[ -d /etc/nginx/sites-enabled ]] && [[ $EUID -eq 0 ]]; then
  ln -sf "$OUTPUT" /etc/nginx/sites-enabled/code-server
  log "Site enabled"
fi

# Test config
if [[ $EUID -eq 0 ]]; then
  if nginx -t 2>/dev/null; then
    log "✅ Nginx config is valid"
    systemctl reload nginx
    log "✅ Nginx reloaded"
  else
    err "Nginx config test failed. Check syntax."
    exit 1
  fi
fi

if $USE_SSL; then
  log ""
  log "For SSL certificate, run:"
  log "  sudo certbot certonly --nginx -d ${DOMAIN}"
  log "  sudo systemctl reload nginx"
fi

log ""
log "Access code-server at: http${USE_SSL:+s}://${DOMAIN}"
