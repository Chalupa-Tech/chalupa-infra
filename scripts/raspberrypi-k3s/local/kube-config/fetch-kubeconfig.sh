#!/bin/bash

# Script to fetch kubeconfig from remote K3s nodes and populate multiple contexts:
# - One per server in ansible/inventory/hosts.yml
# - One for the shared chalupa-k3s.tailbecff0.ts.net

# Configuration
SHARED_CONTEXT="chalupa-k3s"
SHARED_HOST="chalupa-k3s.tailbecff0.ts.net"
TEMP_CONFIG="/tmp/k3s-kubeconfig.yaml"

# Determine path to inventory (handles running from different directories)
if [ -f "ansible/inventory/hosts.yml" ]; then
    INVENTORY_FILE="ansible/inventory/hosts.yml"
elif [ -f "../../ansible/inventory/hosts.yml" ]; then
    INVENTORY_FILE="../../ansible/inventory/hosts.yml"
elif [ -f "scripts/raspberrypi-k3s/ansible/inventory/hosts.yml" ]; then
    INVENTORY_FILE="scripts/raspberrypi-k3s/ansible/inventory/hosts.yml"
else
    echo "Error: ansible/inventory/hosts.yml not found."
    exit 1
fi

# The user can specify the SSH user as an argument
SSH_USER="$1"
if [ -z "$SSH_USER" ]; then
    # Default to current user if not provided
    SSH_USER=$(whoami)
    echo "No SSH user provided, using current user: $SSH_USER"
fi

# Function to parse server hostnames and their addresses from the inventory
# It looks for groups ending in '_servers' and extracts host names and ansible_host
get_servers() {
    awk '
    /^[ ]*#/ { next }
    /_servers:/ { p=1 }
    /_agents:/ { p=0 }
    p && /^[ ]+[a-zA-Z0-9_-]+:$/ {
        last_host = $1;
        sub(/:$/, "", last_host);
    }
    p && /ansible_host:/ && last_host != "hosts" {
        print last_host, $2
    }
    ' "$INVENTORY_FILE"
}

# Collect servers
SERVERS=$(get_servers)

if [ -z "$SERVERS" ]; then
    echo "Error: No servers found in $INVENTORY_FILE."
    exit 1
fi

# We only need to fetch the k3s.yaml once, from any server.
# The CA and client certs are the same across all nodes in the same cluster.
read -r FIRST_HOST FIRST_ADDR <<< "$(echo "$SERVERS" | head -n 1)"

echo "Fetching base kubeconfig from $FIRST_HOST ($FIRST_ADDR)..."
# Use ssh with sudo cat to handle permission issues
ssh "$SSH_USER@$FIRST_ADDR" "sudo cat /etc/rancher/k3s/k3s.yaml" > "$TEMP_CONFIG"

if [ $? -ne 0 ] || [ ! -s "$TEMP_CONFIG" ]; then
    echo "Error: Failed to fetch kubeconfig from $FIRST_HOST."
    echo "Ensure you have SSH access and sudo privileges."
    exit 1
fi

# Create a temporary directory for generating context files
MERGE_DIR=$(mktemp -d)
CLEAN_CONFIG="$MERGE_DIR/base-k3s.yaml"
cp "$TEMP_CONFIG" "$CLEAN_CONFIG"

# Build a list of KUBECONFIG paths for merging
KUBECONFIG_LIST=""

# 1. Create the shared context
echo "Creating shared context for $SHARED_HOST..."
SHARED_FILE="$MERGE_DIR/k3s-shared.yaml"
sed "s/127.0.0.1/$SHARED_HOST/g" "$CLEAN_CONFIG" | \
sed "s/default/$SHARED_CONTEXT/g" > "$SHARED_FILE"
KUBECONFIG_LIST="$SHARED_FILE"

# 2. Create node-specific contexts for each server
while read -r HOST ADDR; do
    echo "Creating context for node $HOST ($ADDR)..."
    HOST_CONFIG="$MERGE_DIR/k3s-$HOST.yaml"
    CONTEXT_NAME="chalupa-k3s-$HOST"

    sed "s/127.0.0.1/$ADDR/g" "$CLEAN_CONFIG" | \
    sed "s/default/$CONTEXT_NAME/g" > "$HOST_CONFIG"

    KUBECONFIG_LIST="$KUBECONFIG_LIST:$HOST_CONFIG"
done <<< "$SERVERS"

# 3. Merge everything into ~/.kube/config
if command -v kubectl &> /dev/null; then
    echo "Merging into ~/.kube/config using kubectl..."

    # Backup existing config
    if [ -f ~/.kube/config ]; then
        cp ~/.kube/config ~/.kube/config.bak.$(date +%s)
        export KUBECONFIG="$KUBECONFIG_LIST:~/.kube/config"
    else
        export KUBECONFIG="$KUBECONFIG_LIST"
    fi

    # Flatten the configuration into a single file
    kubectl config view --flatten > ~/.kube/config.new
    mv ~/.kube/config.new ~/.kube/config
    echo "Success! ~/.kube/config updated."
else
    echo "kubectl not found. Context files are preserved in $MERGE_DIR for manual merging."
    exit 1
fi

# Cleanup
rm -rf "$MERGE_DIR"
rm "$TEMP_CONFIG"

echo ""
echo "Done! Available contexts:"
while read -r HOST ADDR; do
    echo "  - chalupa-k3s-$HOST"
done <<< "$SERVERS"
echo "  - $SHARED_CONTEXT"
echo ""
echo "Switch context with: kubectl config use-context <name>"
