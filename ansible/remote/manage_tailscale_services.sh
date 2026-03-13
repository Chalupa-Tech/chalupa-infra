#!/bin/bash
# Usage: ./manage_tailscale_services.sh "svc:name1:port1" "svc:name2:port2" ...
set -e

if [ "$#" -eq 0 ]; then
    echo "Usage: $0 <svc:name:port> [<svc:name:port> ...]"
    exit 1
fi

for service_def in "$@"; do
    # Split by colon
    # Expected format: svc:chalupa-k3s:6443
    IFS=':' read -r PREFIX NAME PORT <<< "$service_def"
    
    if [ -z "$PREFIX" ] || [ -z "$NAME" ] || [ -z "$PORT" ]; then
        echo "Error: Invalid service definition '$service_def'. Expected format 'svc:name:port'."
        continue
    fi

    FULL_NAME="${PREFIX}:${NAME}"
    
    echo "Configuring Tailscale Service: $FULL_NAME on port $PORT..."
    
    # Check for existing conflicting configuration (Reset if Web/HTTPS found)
    if command -v python3 &> /dev/null; then
        SHOULD_RESET=$(tailscale serve status --json | python3 -c "
import sys, json
try:
    data = json.load(sys.stdin)
    services = data.get('Services', {})
    svc_name = '$FULL_NAME'
    if svc_name in services:
        tcp_config = services[svc_name].get('TCP', {}).get('$PORT', {})
        # If it has 'HTTPS': true, it is terminating TLS, which we don't want.
        # If it has a 'Web' handler for this port, it's also a conflict.
        if tcp_config.get('HTTPS') is True or 'Web' in services[svc_name]:
             print('yes')
except Exception:
    pass
")
        if [ "$SHOULD_RESET" == "yes" ]; then
            echo "Detected conflicting Web/HTTPS configuration for $FULL_NAME."
            echo "Resetting service configuration to switch to TCP pass-through..."
            tailscale serve --service="$FULL_NAME" reset
        fi
    fi

    # Configure TCP pass-through
    if tailscale serve --service="$FULL_NAME" --tcp="$PORT" "127.0.0.1:$PORT"; then
        echo "Tailscale Service host configured: $FULL_NAME (port $PORT) -> 127.0.0.1:$PORT"
    else
        echo "Warning: Could not configure Tailscale Service host '$FULL_NAME'."
    fi
done
