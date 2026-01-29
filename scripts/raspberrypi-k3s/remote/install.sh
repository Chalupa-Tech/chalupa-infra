#!/bin/bash

# Script to install Tailscale and then K3s with Tailscale integration.
# Based on: https://docs.k3s.io/networking/distributed-multicloud#integration-with-the-tailscale-vpn-provider-experimental

set -e

# Check for root
if [ "$EUID" -ne 0 ]; then
  echo "Error: Please run as root (use sudo)."
  exit 1
fi

# Usage help
if [ -z "$1" ]; then
  echo "Usage: $0 <server|agent> [server_ip]"
  echo ""
  echo "  server             : Initialize a NEW cluster (1st node)"
  echo "  server <server_ip> : Join an EXISTING cluster as a server (HA/Etcd)"
  echo "  agent <server_ip>  : Join as a worker only"
  echo ""
  echo "Environment Variables:"
  echo "  TS_AUTHKEY  (Required) Tailscale Auth Key (must be reusable or valid)"
  echo "  K3S_TOKEN   (Required for joining nodes, Optional for 1st server) Shared secret"
  exit 1
fi

ROLE="$1"
SERVER_IP="$2"

# --- 0. Raspberry Pi Checks (Cgroups) ---
# K3s requires cgroup_memory to be enabled. On Pi OS, this is often disabled by default.
check_pi_cgroups() {
    CMDLINE_FILE=""
    if [ -f /boot/firmware/cmdline.txt ]; then
        CMDLINE_FILE="/boot/firmware/cmdline.txt"
    elif [ -f /boot/cmdline.txt ]; then
        CMDLINE_FILE="/boot/cmdline.txt"
    fi

    if [ -n "$CMDLINE_FILE" ]; then
        CURRENT_CMDLINE=$(cat "$CMDLINE_FILE")
        NEEDS_UPDATE=0
        
        if ! echo "$CURRENT_CMDLINE" | grep -q "cgroup_enable=memory"; then
            echo "Missing cgroup_enable=memory in $CMDLINE_FILE"
            NEEDS_UPDATE=1
        fi
        
        if ! echo "$CURRENT_CMDLINE" | grep -q "cgroup_memory=1"; then
            echo "Missing cgroup_memory=1 in $CMDLINE_FILE"
            NEEDS_UPDATE=1
        fi

        if [ "$NEEDS_UPDATE" -eq 1 ]; then
            echo "Updating $CMDLINE_FILE to enable memory cgroups..."
            # Append flags to the end of the line (keeping it one line)
            sed -i 's/$/ cgroup_enable=memory cgroup_memory=1/' "$CMDLINE_FILE"
            echo "Boot config updated. A REBOOT IS REQUIRED."
            echo "Please reboot this Pi and run this script again."
            exit 1
        else
            echo "Memory cgroups already enabled."
        fi
    fi
}

# Check for iptables legacy (common issue on older Pi OS / Debian)
check_iptables() {
    if command -v iptables >/dev/null; then
        if iptables --version | grep -q "nf_tables"; then
            echo "Info: iptables is using nftables backend."
            # Modern K3s supports nftables, but if you have issues, try switching to legacy:
            # update-alternatives --set iptables /usr/sbin/iptables-legacy
        fi
    fi
}

check_pi_cgroups
check_iptables

# Validate Environment Variables
if [ -z "$TS_AUTHKEY" ]; then
  echo "Error: TS_AUTHKEY environment variable is required."
  echo "Export it before running the script: export TS_AUTHKEY=tskey-..."
  exit 1
fi

# Validation: Joining nodes (Agent OR Secondary Server) need a token and IP
if { [ "$ROLE" == "agent" ] || { [ "$ROLE" == "server" ] && [ -n "$SERVER_IP" ]; }; }; then
    if [ -z "$K3S_TOKEN" ]; then
        echo "Error: K3S_TOKEN environment variable is required when joining a cluster."
        exit 1
    fi
    if [ -z "$SERVER_IP" ]; then
        echo "Error: server_ip argument is required when joining a cluster."
        exit 1
    fi
fi

# --- 1. Install Tailscale ---
if ! command -v tailscale &> /dev/null; then
    echo "Installing Tailscale..."
    curl -fsSL https://tailscale.com/install.sh | sh
else
    echo "Tailscale already installed."
fi

# --- 2. Authenticate Tailscale ---
# We check if Tailscale is already logged in
if tailscale status &> /dev/null; then
    echo "Tailscale is already up."
else
    echo "Authenticating Tailscale..."
    # Using --authkey to automate. 
    tailscale up --authkey="$TS_AUTHKEY"
fi

# --- 3. Get Tailscale IP ---
TS_IP=$(tailscale ip -4)
echo "Detected Tailscale IP: $TS_IP"

# --- 4. Install K3s ---
echo "Installing K3s ($ROLE)..."

COMMON_ARGS="--vpn-auth=name=tailscale,joinKey=$TS_AUTHKEY --node-external-ip=$TS_IP --flannel-iface=tailscale0"

if [ "$ROLE" == "server" ]; then
    
    # Servers need to include their Tailscale IP in the TLS SAN list so others can verify the cert
    SERVER_ARGS="$COMMON_ARGS --tls-san=$TS_IP"

    if [ -z "$SERVER_IP" ]; then
        # Case A: First Server (Cluster Init)
        echo "Mode: Initializing NEW Cluster..."
        
        # Use provided token if set, otherwise auto-generate
        if [ -n "$K3S_TOKEN" ]; then
            TOKEN_ARG="--token=$K3S_TOKEN"
        else
            TOKEN_ARG=""
        fi

        curl -sfL https://get.k3s.io | INSTALL_K3S_EXEC="server --cluster-init $TOKEN_ARG $SERVER_ARGS" sh -
        
    else
        # Case B: Joining Server (HA)
        echo "Mode: Joining EXISTING Cluster as Server (HA)..."
        
        # Must have token and server IP (validated above)
        curl -sfL https://get.k3s.io | INSTALL_K3S_EXEC="server --server https://${SERVER_IP}:6443 --token=$K3S_TOKEN $SERVER_ARGS" sh -
    fi

elif [ "$ROLE" == "agent" ]; then
    
    # For agent, we need the server URL. 
    # Note: The server IP provided must be reachable. If using Tailscale, it should be the Server's Tailscale IP.
    
    curl -sfL https://get.k3s.io | K3S_URL=https://${SERVER_IP}:6443 K3S_TOKEN=${K3S_TOKEN} INSTALL_K3S_EXEC="agent $COMMON_ARGS" sh -

else
    echo "Error: Invalid role '$ROLE'. Use 'server' or 'agent'."
    exit 1
fi

echo "-------------------------------------------------------"
echo "K3s installation complete!"
if [ "$ROLE" == "server" ]; then
    echo "Node Token (save this for agents):"
    cat /var/lib/rancher/k3s/server/node-token
    echo ""
    echo "Kubeconfig is available at /etc/rancher/k3s/k3s.yaml"
fi
