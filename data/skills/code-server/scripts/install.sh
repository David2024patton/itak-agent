#!/bin/bash
# code-server installer — downloads, installs, and configures code-server
set -euo pipefail

VERSION=""
SYSTEM_INSTALL=false
UPDATE_ONLY=false
INSTALL_DIR="$HOME/.local/bin"
CONFIG_DIR="$HOME/.config/code-server"
DATA_DIR="$HOME/.local/share/code-server"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

log() { echo -e "${GREEN}[code-server]${NC} $1"; }
warn() { echo -e "${YELLOW}[code-server]${NC} $1"; }
err() { echo -e "${RED}[code-server]${NC} $1" >&2; }

usage() {
  cat <<EOF
Usage: bash install.sh [OPTIONS]

Options:
  --version VERSION    Install specific version (default: latest)
  --system             Install system-wide to /usr/local/bin (requires root)
  --update             Update existing installation
  -h, --help           Show this help

Examples:
  bash install.sh                    # Install latest for current user
  bash install.sh --version 4.96.4   # Install specific version
  bash install.sh --update           # Update to latest
  sudo bash install.sh --system      # Install system-wide
EOF
  exit 0
}

# Parse args
while [[ $# -gt 0 ]]; do
  case $1 in
    --version) VERSION="$2"; shift 2 ;;
    --system) SYSTEM_INSTALL=true; shift ;;
    --update) UPDATE_ONLY=true; shift ;;
    -h|--help) usage ;;
    *) err "Unknown option: $1"; exit 1 ;;
  esac
done

if $SYSTEM_INSTALL; then
  INSTALL_DIR="/usr/local/bin"
  if [[ $EUID -ne 0 ]]; then
    err "System install requires root. Use: sudo bash install.sh --system"
    exit 1
  fi
fi

# Detect architecture
ARCH=$(uname -m)
case $ARCH in
  x86_64)  ARCH="amd64" ;;
  aarch64) ARCH="arm64" ;;
  armv7l)  ARCH="armv7l" ;;
  *) err "Unsupported architecture: $ARCH"; exit 1 ;;
esac

OS=$(uname -s | tr '[:upper:]' '[:lower:]')
if [[ "$OS" != "linux" ]]; then
  err "This installer supports Linux only. For macOS, use: brew install code-server"
  exit 1
fi

# Get latest version if not specified
if [[ -z "$VERSION" ]]; then
  log "Fetching latest version..."
  VERSION=$(curl -sL https://api.github.com/repos/coder/code-server/releases/latest | grep '"tag_name"' | sed -E 's/.*"v([^"]+)".*/\1/')
  if [[ -z "$VERSION" ]]; then
    err "Failed to fetch latest version. Specify manually with --version"
    exit 1
  fi
fi

log "Installing code-server v${VERSION} (${OS}-${ARCH})"

# Check if already installed at same version
if command -v code-server &>/dev/null; then
  CURRENT=$(code-server --version 2>/dev/null | head -1 | awk '{print $1}')
  if [[ "$CURRENT" == "$VERSION" ]] && ! $UPDATE_ONLY; then
    log "code-server v${VERSION} is already installed"
    exit 0
  fi
  if [[ -n "$CURRENT" ]]; then
    log "Upgrading from v${CURRENT} to v${VERSION}"
  fi
fi

# Download
TARBALL="code-server-${VERSION}-${OS}-${ARCH}.tar.gz"
URL="https://github.com/coder/code-server/releases/download/v${VERSION}/${TARBALL}"
TMP_DIR=$(mktemp -d)
trap 'rm -rf "$TMP_DIR"' EXIT

log "Downloading ${URL}..."
if ! curl -fsSL "$URL" -o "${TMP_DIR}/${TARBALL}"; then
  err "Download failed. Check version and architecture."
  exit 1
fi

# Extract
log "Extracting..."
tar -xzf "${TMP_DIR}/${TARBALL}" -C "$TMP_DIR"
EXTRACTED_DIR="${TMP_DIR}/code-server-${VERSION}-${OS}-${ARCH}"

# Install binary
mkdir -p "$INSTALL_DIR"
cp "${EXTRACTED_DIR}/bin/code-server" "$INSTALL_DIR/code-server"
chmod +x "$INSTALL_DIR/code-server"

# Install lib (needed for node modules)
LIB_DIR="${INSTALL_DIR}/../lib/code-server"
mkdir -p "$LIB_DIR"
if [[ -d "${EXTRACTED_DIR}/lib" ]]; then
  cp -r "${EXTRACTED_DIR}/lib/"* "$LIB_DIR/" 2>/dev/null || true
fi

log "Installed to ${INSTALL_DIR}/code-server"

# Create default config if not exists
if [[ ! -f "${CONFIG_DIR}/config.yaml" ]]; then
  mkdir -p "$CONFIG_DIR"
  DEFAULT_PASSWORD=$(openssl rand -base64 16 2>/dev/null || head -c 16 /dev/urandom | base64)
  cat > "${CONFIG_DIR}/config.yaml" <<CONF
bind-addr: 127.0.0.1:8443
auth: password
password: ${DEFAULT_PASSWORD}
cert: false
CONF
  log "Created config at ${CONFIG_DIR}/config.yaml"
  log "Default password: ${DEFAULT_PASSWORD}"
  warn "Change this password! Run: bash scripts/manage.sh set-password 'new-password'"
fi

# Create data directory
mkdir -p "$DATA_DIR"

# Set up systemd user service
if command -v systemctl &>/dev/null && [[ $EUID -ne 0 ]]; then
  SYSTEMD_DIR="$HOME/.config/systemd/user"
  mkdir -p "$SYSTEMD_DIR"
  cat > "${SYSTEMD_DIR}/code-server.service" <<SERVICE
[Unit]
Description=code-server - VS Code in the browser
After=network.target

[Service]
Type=exec
ExecStart=${INSTALL_DIR}/code-server --config ${CONFIG_DIR}/config.yaml
Restart=on-failure
RestartSec=5

[Install]
WantedBy=default.target
SERVICE
  systemctl --user daemon-reload 2>/dev/null || true
  log "Systemd user service created. Enable with: systemctl --user enable code-server"
fi

# System-wide systemd service
if $SYSTEM_INSTALL && command -v systemctl &>/dev/null; then
  cat > /etc/systemd/system/code-server@.service <<SERVICE
[Unit]
Description=code-server for %i
After=network.target

[Service]
Type=exec
User=%i
ExecStart=${INSTALL_DIR}/code-server --config /home/%i/.config/code-server/config.yaml
Restart=on-failure
RestartSec=5

[Install]
WantedBy=multi-user.target
SERVICE
  systemctl daemon-reload
  log "System service created. Enable for user: systemctl enable code-server@username"
fi

# Verify
if "${INSTALL_DIR}/code-server" --version &>/dev/null; then
  INSTALLED_VERSION=$("${INSTALL_DIR}/code-server" --version 2>/dev/null | head -1)
  log "✅ code-server ${INSTALLED_VERSION} installed successfully"
else
  err "Installation verification failed"
  exit 1
fi

# PATH check
if ! echo "$PATH" | tr ':' '\n' | grep -q "^${INSTALL_DIR}$"; then
  warn "Add to PATH: export PATH=\"${INSTALL_DIR}:\$PATH\""
  warn "Or add to ~/.bashrc: echo 'export PATH=\"${INSTALL_DIR}:\$PATH\"' >> ~/.bashrc"
fi

log ""
log "Quick start:"
log "  1. Start:  bash scripts/manage.sh start"
log "  2. Open:   http://localhost:8443"
log "  3. Login:  Password in ${CONFIG_DIR}/config.yaml"
