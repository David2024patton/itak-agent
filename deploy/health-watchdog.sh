#!/bin/bash
# iTaK Torch Health Watchdog
# Runs alongside the itaktorch systemd service. Pings /health and notifies systemd.
# Usage: started by itaktorch.service WatchdogSec or as a sidecar.

HEALTH_URL="${ITAK_TORCH_HEALTH_URL:-http://localhost:41950/health}"
INTERVAL="${ITAK_TORCH_WATCHDOG_INTERVAL:-30}"

while true; do
    if curl -sf "$HEALTH_URL" > /dev/null 2>&1; then
        # Service is healthy, notify systemd watchdog.
        systemd-notify WATCHDOG=1 2>/dev/null || true
    else
        echo "[watchdog] iTaK Torch health check FAILED: $HEALTH_URL" >&2
    fi
    sleep "$INTERVAL"
done
