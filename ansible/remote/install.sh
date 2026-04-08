#!/bin/bash

# Bootstrap script: installs Tailscale and K3s binary/service.
# All K3s configuration lives in /etc/rancher/k3s/config.yaml,
# managed by the k3s_config Ansible role. This script only handles
# binary installation and service creation.

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
  echo "  TS_AUTHKEY      (Required) Tailscale Auth Key (must be reusable or valid)"
  echo "  K3S_TOKEN       (Required for joining nodes, Optional for 1st server) Shared secret"
  echo ""
  echo "K3s configuration (flannel, labels, taints, TLS SANs) is managed via"
  echo "/etc/rancher/k3s/config.yaml by the k3s_config Ansible role."
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

# --- 3. Install K3s ---
# All K3s configuration (flannel backend, node-ip, labels, taints, TLS SANs)
# is managed via /etc/rancher/k3s/config.yaml by the k3s_config Ansible role.
# This script only installs the binary and creates the systemd service.

echo "Installing K3s ($ROLE)..."

if [ "$ROLE" == "server" ]; then

    if [ -z "$SERVER_IP" ]; then
        # Case A: First Server (Cluster Init)
        echo "Mode: Initializing NEW Cluster..."

        curl -sfL https://get.k3s.io | INSTALL_K3S_VERSION="${INSTALL_K3S_VERSION}" INSTALL_K3S_EXEC="server" K3S_TOKEN="${K3S_TOKEN}" sh -

    else
        # Case B: Joining Server (HA)
        echo "Mode: Joining EXISTING Cluster as Server (HA)..."

        curl -sfL https://get.k3s.io | INSTALL_K3S_VERSION="${INSTALL_K3S_VERSION}" INSTALL_K3S_EXEC="server" \
            K3S_URL="https://${SERVER_IP}:6443" K3S_TOKEN="${K3S_TOKEN}" sh -
    fi

elif [ "$ROLE" == "agent" ]; then

    curl -sfL https://get.k3s.io | INSTALL_K3S_VERSION="${INSTALL_K3S_VERSION}" K3S_URL="https://${SERVER_IP}:6443" \
        K3S_TOKEN="${K3S_TOKEN}" sh -s - agent

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
