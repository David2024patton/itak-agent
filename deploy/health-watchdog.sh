#!/bin/bash
# GOTorch Health Watchdog
# Runs alongside the gotorch systemd service. Pings /health and notifies systemd.
# Usage: started by gotorch.service WatchdogSec or as a sidecar.

HEALTH_URL="${GOTORCH_HEALTH_URL:-http://localhost:41950/health}"
INTERVAL="${GOTORCH_WATCHDOG_INTERVAL:-30}"

while true; do
    if curl -sf "$HEALTH_URL" > /dev/null 2>&1; then
        # Service is healthy, notify systemd watchdog.
        systemd-notify WATCHDOG=1 2>/dev/null || true
    else
        echo "[watchdog] GOTorch health check FAILED: $HEALTH_URL" >&2
    fi
    sleep "$INTERVAL"
done
