#!/bin/bash

# Script to fetch kubeconfig from a remote K3s node (via Tailscale) and merge it into local ~/.kube/config

if [ -z "$1" ]; then
    echo "Usage: $0 <user>@<host>"
    echo "Example: $0 pi@100.x.y.z"
    exit 1
fi

REMOTE="$1"
TEMP_CONFIG="/tmp/k3s-kubeconfig.yaml"
CONTEXT_NAME="chalupa-k3s"

echo "Fetching kubeconfig from $REMOTE..."
# Use ssh with sudo cat instead of scp to handle permission issues (root ssh disabled)
ssh -t "$REMOTE" "sudo cat /etc/rancher/k3s/k3s.yaml" > "$TEMP_CONFIG"

# Check if the file is empty or contains an error (basic check)
if [ ! -s "$TEMP_CONFIG" ] || grep -q "Permission denied" "$TEMP_CONFIG"; then
    echo "Error: Failed to fetch kubeconfig. Ensure you have sudo access on the remote host."
    rm -f "$TEMP_CONFIG"
    exit 1
fi

# Remove any potential CRLF characters if fetching from a weird terminal setup, 
# though less likely on linux-to-linux/mac.
# Also, ssh -t might output the password prompt or connection closed messages to stdout mixed with the file if not careful.
# A cleaner way is usually just `ssh "$REMOTE" "sudo cat ..."` without -t if sudo is passwordless, 
# but if sudo requires password, -t is needed for interaction, but that messes up the redirection.
# Let's try without -t first. If the user has passwordless sudo or an agent, it works. 
# If they need a password, `ssh user@host sudo cat file` will fail if no tty.
#
# BETTER APPROACH:
# Use `ssh -t` but we can't redirect easily if it mixes output.
# Actually, for most Pi setups, sudo requires a password.
# Let's try to just run it. If it hangs or fails, the user will see.
# Re-writing the command to be more robust:
# We'll just run `ssh $REMOTE "sudo cat /etc/rancher/k3s/k3s.yaml"` 
# If it fails, we tell the user.

ssh "$REMOTE" "sudo cat /etc/rancher/k3s/k3s.yaml" > "$TEMP_CONFIG"

if [ $? -ne 0 ] || [ ! -s "$TEMP_CONFIG" ]; then
    echo "Error: Failed to fetch kubeconfig."
    echo "If sudo requires a password, try running this manually:"
    echo "  ssh -t $REMOTE 'sudo cat /etc/rancher/k3s/k3s.yaml' > k3s.yaml"
    exit 1
fi

# Extract the IP from the remote string (user@ip -> ip)
REMOTE_IP="${REMOTE#*@}"

echo "Updating server address to https://$REMOTE_IP:6443..."
# Replace 127.0.0.1 with the Tailscale IP of the remote node
sed -i '' "s/127.0.0.1/$REMOTE_IP/g" "$TEMP_CONFIG"

# Rename the context/cluster/user to avoid conflicts
echo "Renaming context to '$CONTEXT_NAME'..."
sed -i '' "s/default/$CONTEXT_NAME/g" "$TEMP_CONFIG"

echo "Merging into ~/.kube/config..."

# Ensure .kube directory exists
if [ ! -d ~/.kube ]; then
    echo "Creating ~/.kube directory..."
    mkdir -p ~/.kube
fi

# Backup existing config if it exists
if [ -f ~/.kube/config ]; then
    cp ~/.kube/config ~/.kube/config.bak.$(date +%s)
fi

# If kubectl is available, merge. Otherwise, just copy.
if command -v kubectl &> /dev/null; then
    # If KUBECONFIG is set, use it; otherwise default
    # We explicitly include the existing config only if it exists
    if [ -f ~/.kube/config ]; then
        export KUBECONFIG=~/.kube/config:$TEMP_CONFIG
    else
        export KUBECONFIG=$TEMP_CONFIG
    fi
    
    # This merges the two configs into a temporary view, we need to flatten it back to the file
    kubectl config view --flatten > ~/.kube/config.new
    mv ~/.kube/config.new ~/.kube/config
else
    echo "kubectl not found, overwriting ~/.kube/config with new config..."
    mv "$TEMP_CONFIG" ~/.kube/config
fi

# Cleanup
rm "$TEMP_CONFIG"

echo "Done! You can now switch context:"
echo "  kubectl config use-context $CONTEXT_NAME"