---
name: code-server
description: >-
  Install and manage VS Code in the browser with code-server. Full IDE accessible from any device.
categories: [dev-tools, productivity]
dependencies: [bash, curl, systemd]
---

# Code Server Manager

## What This Does

Install, configure, and manage [code-server](https://github.com/coder/code-server) — VS Code running in your browser. Access your full development environment from any device with a web browser. Includes systemd service management, authentication setup, extension management, and reverse proxy configuration.

**Example:** "Install code-server, set it up on port 8443 with password auth, install my favorite extensions, and configure it as a systemd service."

## Quick Start (5 minutes)

### 1. Install code-server

```bash
bash scripts/install.sh
```

This will:
- Download the latest code-server release
- Install it to `~/.local/bin/code-server`
- Create a default config at `~/.config/code-server/config.yaml`
- Set up a systemd user service

### 2. Start code-server

```bash
bash scripts/manage.sh start
```

Access at `http://localhost:8443` — password is in `~/.config/code-server/config.yaml`.

### 3. Configure

```bash
# Change password
bash scripts/manage.sh set-password "your-secure-password"

# Change bind address (allow remote access)
bash scripts/manage.sh set-bind "0.0.0.0:8443"

# Restart to apply changes
bash scripts/manage.sh restart
```

## Core Workflows

### Workflow 1: Fresh Install + Setup

**Use case:** Set up a remote dev environment on a VPS or home server.

```bash
# Install
bash scripts/install.sh

# Set a strong password
bash scripts/manage.sh set-password "$(openssl rand -base64 24)"

# Allow remote connections
bash scripts/manage.sh set-bind "0.0.0.0:8443"

# Enable and start
bash scripts/manage.sh enable
bash scripts/manage.sh start

# Show access info
bash scripts/manage.sh status
```

**Output:**
```
✅ code-server is running
   URL: http://0.0.0.0:8443
   Password: <your-password>
   PID: 12345
   Uptime: 2 minutes
```

### Workflow 2: Extension Management

**Use case:** Install your favorite VS Code extensions.

```bash
# Install extensions
bash scripts/extensions.sh install ms-python.python
bash scripts/extensions.sh install bradlc.vscode-tailwindcss
bash scripts/extensions.sh install esbenp.prettier-vscode
bash scripts/extensions.sh install dbaeumer.vscode-eslint

# List installed extensions
bash scripts/extensions.sh list

# Install from a file (one extension ID per line)
bash scripts/extensions.sh install-from extensions.txt

# Uninstall
bash scripts/extensions.sh uninstall ms-python.python
```

### Workflow 3: Reverse Proxy with Nginx

**Use case:** Serve code-server over HTTPS with a domain.

```bash
# Generate nginx config for code-server
bash scripts/nginx-proxy.sh generate \
  --domain code.example.com \
  --port 8443 \
  --ssl  # Uses certbot for Let's Encrypt

# Output: /etc/nginx/sites-available/code-server
# Enables WebSocket support for terminal
```

**Generated config includes:**
- WebSocket upgrade headers (required for terminal)
- SSL termination with Let's Encrypt
- Proxy headers for proper IP forwarding
- Gzip compression

### Workflow 4: Backup & Restore Settings

**Use case:** Migrate your setup to another machine.

```bash
# Backup config + extensions list
bash scripts/backup.sh create ~/code-server-backup.tar.gz

# Restore on new machine
bash scripts/backup.sh restore ~/code-server-backup.tar.gz
```

### Workflow 5: Update code-server

**Use case:** Upgrade to the latest version.

```bash
# Check current version
bash scripts/manage.sh version

# Update to latest
bash scripts/install.sh --update

# Restart
bash scripts/manage.sh restart
```

## Configuration

### Config File (`~/.config/code-server/config.yaml`)

```yaml
bind-addr: 127.0.0.1:8443
auth: password
password: your-password-here
cert: false
```

### Configuration Options

```bash
# Authentication mode
bash scripts/manage.sh set-auth password    # Password (default)
bash scripts/manage.sh set-auth none        # No auth (LAN only!)

# Enable HTTPS with self-signed cert
bash scripts/manage.sh set-cert true

# Set custom cert files
bash scripts/manage.sh set-cert-file /path/to/cert.pem
bash scripts/manage.sh set-key-file /path/to/key.pem

# Set workspace directory
bash scripts/manage.sh set-workspace /home/user/projects
```

### Environment Variables

```bash
# Override config file location
export CODE_SERVER_CONFIG="$HOME/.config/code-server/config.yaml"

# Set password via env (useful for containers)
export PASSWORD="your-password"

# Set hashed password (more secure)
export HASHED_PASSWORD="$(echo -n 'your-password' | npx argon2-cli -e)"
```

## Advanced Usage

### Run as System Service (multi-user)

```bash
# Install as system-wide service (requires root)
sudo bash scripts/install.sh --system

# Enable for a specific user
sudo bash scripts/manage.sh enable-user username

# Start system service
sudo systemctl start code-server@username
```

### Docker Deployment

```bash
# Generate docker-compose.yml
bash scripts/docker.sh generate \
  --port 8443 \
  --password "your-password" \
  --workspace /home/user/projects

# Start with Docker
docker compose up -d

# View logs
docker compose logs -f code-server
```

### Multiple Instances

```bash
# Run on different ports for different projects
bash scripts/manage.sh start --port 8443 --workspace ~/project-a
bash scripts/manage.sh start --port 8444 --workspace ~/project-b
```

## Troubleshooting

### Issue: "code-server: command not found"

**Fix:**
```bash
# Add to PATH
export PATH="$HOME/.local/bin:$PATH"
echo 'export PATH="$HOME/.local/bin:$PATH"' >> ~/.bashrc

# Or reinstall
bash scripts/install.sh
```

### Issue: Can't connect remotely

**Check:**
1. Bind address includes `0.0.0.0`: `grep bind-addr ~/.config/code-server/config.yaml`
2. Firewall allows port: `sudo ufw allow 8443`
3. Service is running: `bash scripts/manage.sh status`

### Issue: Terminal not working through reverse proxy

**Fix:** Ensure WebSocket upgrade headers in nginx:
```nginx
proxy_set_header Upgrade $http_upgrade;
proxy_set_header Connection "upgrade";
```

### Issue: Extensions not installing

**Fix:**
```bash
# Check extension marketplace connectivity
curl -s https://open-vsx.org/api/-/search?query=python | head -c 100

# code-server uses Open VSX, not Microsoft marketplace
# Some extensions may not be available — check https://open-vsx.org
```

### Issue: High memory usage

**Fix:**
```bash
# Limit memory
bash scripts/manage.sh set-max-memory 2048  # MB

# Disable unused features
bash scripts/manage.sh set-setting "extensions.autoUpdate" false
bash scripts/manage.sh set-setting "search.followSymlinks" false
```

## Security Best Practices

1. **Always use a strong password** — `openssl rand -base64 24`
2. **Use HTTPS** — Self-signed cert or reverse proxy with Let's Encrypt
3. **Don't bind to 0.0.0.0 without auth** — Anyone can access your terminal
4. **Use a reverse proxy** — Adds TLS, rate limiting, access control
5. **Keep updated** — `bash scripts/install.sh --update`

## Dependencies

- `bash` (4.0+)
- `curl` (for downloading)
- `systemd` (for service management, optional)
- `tar` (for extraction)
- `openssl` (for password/cert generation)
- Optional: `nginx` (for reverse proxy)
- Optional: `docker` (for container deployment)

## Key Principles

1. **Single command install** — No manual downloads or PATH juggling
2. **Systemd-managed** — Starts on boot, restarts on crash
3. **Secure by default** — Password auth enabled, bound to localhost
4. **Extension ecosystem** — Access to Open VSX marketplace
5. **Portable config** — Backup/restore across machines
