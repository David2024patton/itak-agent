#!/bin/bash
# iTaK Agent Installer for Linux and macOS
# Usage: curl -fsSL https://raw.githubusercontent.com/David2024patton/itak-agent/main/install.sh | bash

set -e

REPO="David2024patton/itak-agent"
INSTALL_DIR="/usr/local/bin"
BINARY_NAME="itak-agent"
DATA_DIR="$HOME/.itak-agent"

# Detect OS and architecture
OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m)

case "$ARCH" in
  x86_64|amd64) ARCH="amd64" ;;
  aarch64|arm64) ARCH="arm64" ;;
  *) echo "Unsupported architecture: $ARCH"; exit 1 ;;
esac

case "$OS" in
  linux|darwin) ;;
  *) echo "Unsupported OS: $OS (use install.ps1 for Windows)"; exit 1 ;;
esac

echo ""
echo "  ┌──────────────────────────────────┐"
echo "  │     iTaK Agent Installer         │"
echo "  │     OS: $OS / $ARCH              │"
echo "  └──────────────────────────────────┘"
echo ""

# Get latest release tag
echo "→ Finding latest release..."
LATEST=$(curl -fsSL "https://api.github.com/repos/$REPO/releases/latest" | grep '"tag_name"' | sed -E 's/.*"([^"]+)".*/\1/')
if [ -z "$LATEST" ]; then
  echo "  No releases found. Building from source..."
  echo "  Cloning repository..."
  git clone "https://github.com/$REPO.git" /tmp/itak-agent-build
  cd /tmp/itak-agent-build
  go build -o "$BINARY_NAME" ./cmd/itakagent
  sudo mv "$BINARY_NAME" "$INSTALL_DIR/$BINARY_NAME"
  rm -rf /tmp/itak-agent-build
else
  DOWNLOAD_URL="https://github.com/$REPO/releases/download/$LATEST/$BINARY_NAME-$OS-$ARCH"
  echo "→ Downloading $BINARY_NAME $LATEST for $OS/$ARCH..."
  curl -fsSL -o /tmp/$BINARY_NAME "$DOWNLOAD_URL"
  chmod +x /tmp/$BINARY_NAME
  sudo mv /tmp/$BINARY_NAME "$INSTALL_DIR/$BINARY_NAME"
fi

# Create data directory
mkdir -p "$DATA_DIR"

# Verify installation
if command -v $BINARY_NAME &> /dev/null; then
  echo ""
  echo "✓ Installed successfully!"
  echo "  Binary: $INSTALL_DIR/$BINARY_NAME"
  echo "  Data:   $DATA_DIR"
  echo ""
  echo "  Start the agent:"
  echo "    itak-agent --port 42800"
  echo ""
  echo "  Then open http://localhost:42800 in your browser."
  echo ""
else
  echo ""
  echo "⚠ Binary installed to $INSTALL_DIR but not found in PATH."
  echo "  Add $INSTALL_DIR to your PATH environment variable."
  echo ""
fi
