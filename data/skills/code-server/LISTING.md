# Listing Copy: Code Server Manager

## Metadata
- **Type:** Skill
- **Name:** code-server
- **Display Name:** Code Server Manager
- **Categories:** [dev-tools, productivity]
- **Price:** $12
- **Dependencies:** [bash, curl, systemd]
- **Icon:** 💻

## Tagline
VS Code in your browser — Install, configure, and manage code-server in 5 minutes

## Description

Setting up a remote development environment shouldn't take hours of configuration. Whether you're working from a tablet, a different machine, or just want your IDE accessible anywhere, you need a reliable way to run VS Code in the browser.

Code Server Manager handles the entire lifecycle: one-command installation of code-server, automatic systemd service setup, password authentication, extension management, and even nginx reverse proxy generation for HTTPS access. No manual downloads, no PATH juggling, no config file editing.

**What it does:**
- 🚀 One-command install with automatic architecture detection (amd64/arm64)
- 🔐 Secure by default — password auth, localhost-only binding
- 📦 Extension manager — install, export, restore from Open VSX marketplace
- 🔄 Systemd integration — auto-start on boot, restart on crash
- 🌐 Nginx reverse proxy generator with WebSocket + SSL support
- 🐳 Docker deployment with docker-compose generation
- 💾 Backup & restore — migrate config + extensions across machines
- ⬆️ One-command updates to latest version

Perfect for developers, sysadmins, and remote workers who want a full VS Code IDE accessible from any browser.

## Quick Start Preview

```bash
# Install code-server
bash scripts/install.sh

# Start it
bash scripts/manage.sh start

# Access at http://localhost:8443
# Install extensions
bash scripts/extensions.sh install ms-python.python
```

## Core Capabilities

1. Automated installation — Downloads correct binary for your OS/arch
2. Systemd service — Runs as managed service with auto-restart
3. Password management — Set/change auth from CLI
4. Extension ecosystem — Install, list, export, bulk-install from file
5. Nginx proxy — Generate reverse proxy config with SSL support
6. Docker support — Generate docker-compose.yml for container deployment
7. Backup/restore — Migrate config + extensions + settings across machines
8. Multi-instance — Run multiple code-server instances on different ports
9. Memory control — Set Node.js heap limits via systemd overrides
10. VS Code settings — Manage settings.json from CLI

## Installation Time
**5 minutes** — Run install script, start service, open browser
