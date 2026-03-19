#!/usr/bin/env bash
# ──────────────────────────────────────────────────────────────────
#  iTaK Agent - One-Click Setup (Linux / macOS)
#
#  What this does:
#    1. Checks that Docker is running
#    2. Builds the iTaK Agent image
#    3. Starts the container on port 42800
#    4. Opens the dashboard in your browser
#
#  Usage:  chmod +x setup.sh && ./setup.sh
# ──────────────────────────────────────────────────────────────────
set -e

GREEN='\033[0;32m'
RED='\033[0;31m'
CYAN='\033[0;36m'
BOLD='\033[1m'
NC='\033[0m'

echo ""
echo -e "${CYAN}${BOLD}╔══════════════════════════════════════════╗${NC}"
echo -e "${CYAN}${BOLD}║        iTaK Agent - Quick Setup          ║${NC}"
echo -e "${CYAN}${BOLD}╚══════════════════════════════════════════╝${NC}"
echo ""

# ── Step 1: Check Docker ─────────────────────────────────────────
echo -e "  ${BOLD}[1/4]${NC} Checking Docker..."
if ! docker info > /dev/null 2>&1; then
    echo -e "  ${RED}ERROR: Docker is not running!${NC}"
    echo ""
    echo "  Please start Docker and try again."
    echo "  Install: https://docs.docker.com/get-docker/"
    exit 1
fi
echo -e "        ${GREEN}Docker is running.${NC}"

# ── Step 2: Find project root ────────────────────────────────────
echo -e "  ${BOLD}[2/4]${NC} Locating project..."
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"
COMPOSE_FILE="$SCRIPT_DIR/docker-compose.yml"

if [ ! -f "$COMPOSE_FILE" ]; then
    echo -e "  ${RED}ERROR: docker-compose.yml not found${NC}"
    exit 1
fi
echo "        Project root: $PROJECT_ROOT"

# ── Step 3: Build ────────────────────────────────────────────────
echo -e "  ${BOLD}[3/4]${NC} Building iTaK Agent (first run takes ~30s)..."
echo ""
cd "$PROJECT_ROOT"
docker compose -f "$COMPOSE_FILE" build
echo ""

# ── Step 4: Start ────────────────────────────────────────────────
echo -e "  ${BOLD}[4/4]${NC} Starting iTaK Agent..."
docker compose -f "$COMPOSE_FILE" up -d

echo ""
echo "  Waiting for agent to start..."
sleep 3

# ── Done ─────────────────────────────────────────────────────────
echo ""
echo -e "${GREEN}${BOLD}╔══════════════════════════════════════════╗${NC}"
echo -e "${GREEN}${BOLD}║            Setup Complete!               ║${NC}"
echo -e "${GREEN}${BOLD}╠══════════════════════════════════════════╣${NC}"
echo -e "${GREEN}${BOLD}║                                          ║${NC}"
echo -e "${GREEN}${BOLD}║  Dashboard:  http://localhost:42800       ║${NC}"
echo -e "${GREEN}${BOLD}║  API:        http://localhost:42800/v1    ║${NC}"
echo -e "${GREEN}${BOLD}║  Health:     http://localhost:42800/health║${NC}"
echo -e "${GREEN}${BOLD}║                                          ║${NC}"
echo -e "${GREEN}${BOLD}║  Stop:       docker stop itak-agent      ║${NC}"
echo -e "${GREEN}${BOLD}║  Logs:       docker logs itak-agent      ║${NC}"
echo -e "${GREEN}${BOLD}║  Restart:    docker restart itak-agent    ║${NC}"
echo -e "${GREEN}${BOLD}║                                          ║${NC}"
echo -e "${GREEN}${BOLD}╚══════════════════════════════════════════╝${NC}"
echo ""

# Open browser (works on most systems).
if command -v xdg-open > /dev/null 2>&1; then
    xdg-open http://localhost:42800 &
elif command -v open > /dev/null 2>&1; then
    open http://localhost:42800
fi
