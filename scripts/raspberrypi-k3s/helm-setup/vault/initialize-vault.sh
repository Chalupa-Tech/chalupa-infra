#!/bin/bash
# Vault Initialization and Raft Cluster Setup Script
# Run this AFTER install-hashi-vault.sh has completed and pods are ready

set -e

NAMESPACE="vault"
INIT_OUTPUT_FILE="vault-init-keys.json"

echo "=== Vault HA Raft Initialization Script ==="
echo ""

# Function to wait for pod to be running (but not necessarily ready, as unsealed Vault isn't "Ready")
wait_for_pod() {
    local pod=$1
    echo "Waiting for $pod to be running..."
    kubectl wait --request-timeout=60s --for=condition=PodReadyToStartContainers pod/$pod -n $NAMESPACE --timeout=300s
}

# Function to check if Vault is already initialized
check_initialized() {
    kubectl exec --request-timeout=60s -n $NAMESPACE vault-0 -- vault status 2>/dev/null | grep -q "Initialized.*true" && return 0 || return 1
}

# Step 1: Wait for all Vault pods to be running
echo "Step 1: Waiting for Vault pods to be running..."
for i in 0 1 2; do
    wait_for_pod "vault-$i"
done
echo "✓ All Vault pods are ready"
echo ""

# Step 2: Check if already initialized
if check_initialized; then
    echo "⚠️  Vault is already initialized. Skipping initialization."
    echo "   If you need to unseal, use: kubectl exec --request-timeout=60s -n $NAMESPACE vault-0 -- vault operator unseal"
    exit 0
fi

# Step 3: Initialize Vault on vault-0
echo "Step 2: Initializing Vault on vault-0..."
kubectl exec --request-timeout=60s -n $NAMESPACE vault-0 -- vault operator init \
    -key-shares=5 \
    -key-threshold=3 \
    -format=json > "$INIT_OUTPUT_FILE"

echo "✓ Vault initialized! Keys saved to: $INIT_OUTPUT_FILE"
echo ""
echo "╔════════════════════════════════════════════════════════════════╗"
echo "║  ⚠️  IMPORTANT: SAVE YOUR UNSEAL KEYS AND ROOT TOKEN SECURELY! ║"
echo "║  The file $INIT_OUTPUT_FILE contains your recovery keys.       ║"
echo "║  Store these in a secure location and DELETE this file!        ║"
echo "╚════════════════════════════════════════════════════════════════╝"
echo ""

# Extract unseal keys
UNSEAL_KEY_1=$(cat "$INIT_OUTPUT_FILE" | jq -r '.unseal_keys_b64[0]')
UNSEAL_KEY_2=$(cat "$INIT_OUTPUT_FILE" | jq -r '.unseal_keys_b64[1]')
UNSEAL_KEY_3=$(cat "$INIT_OUTPUT_FILE" | jq -r '.unseal_keys_b64[2]')
ROOT_TOKEN=$(cat "$INIT_OUTPUT_FILE" | jq -r '.root_token')

# Step 4: Unseal vault-0
echo "Step 3: Unsealing vault-0..."
kubectl exec --request-timeout=60s -n $NAMESPACE vault-0 -- vault operator unseal "$UNSEAL_KEY_1"
kubectl exec --request-timeout=60s -n $NAMESPACE vault-0 -- vault operator unseal "$UNSEAL_KEY_2"
kubectl exec --request-timeout=60s -n $NAMESPACE vault-0 -- vault operator unseal "$UNSEAL_KEY_3"
echo "✓ vault-0 unsealed"
echo ""

# Step 5: Join vault-1 to Raft cluster
echo "Step 4: Joining vault-1 to Raft cluster..."
kubectl exec --request-timeout=60s -n $NAMESPACE vault-1 -- vault operator raft join http://vault-0.vault-internal:8200
echo "✓ vault-1 joined"

# Step 6: Unseal vault-1
echo "Step 5: Unsealing vault-1..."
kubectl exec --request-timeout=60s -n $NAMESPACE vault-1 -- vault operator unseal "$UNSEAL_KEY_1"
kubectl exec --request-timeout=60s -n $NAMESPACE vault-1 -- vault operator unseal "$UNSEAL_KEY_2"
kubectl exec --request-timeout=60s -n $NAMESPACE vault-1 -- vault operator unseal "$UNSEAL_KEY_3"
echo "✓ vault-1 unsealed"
echo ""

# Step 7: Join vault-2 to Raft cluster
echo "Step 6: Joining vault-2 to Raft cluster..."
kubectl exec --request-timeout=60s -n $NAMESPACE vault-2 -- vault operator raft join http://vault-0.vault-internal:8200
echo "✓ vault-2 joined"

# Step 8: Unseal vault-2
echo "Step 7: Unsealing vault-2..."
kubectl exec --request-timeout=60s -n $NAMESPACE vault-2 -- vault operator unseal "$UNSEAL_KEY_1"
kubectl exec --request-timeout=60s -n $NAMESPACE vault-2 -- vault operator unseal "$UNSEAL_KEY_2"
kubectl exec --request-timeout=60s -n $NAMESPACE vault-2 -- vault operator unseal "$UNSEAL_KEY_3"
echo "✓ vault-2 unsealed"
echo ""

# Step 9: Verify cluster status
echo "Step 8: Verifying Raft cluster status..."
kubectl exec --request-timeout=60s -n $NAMESPACE vault-0 -- vault login "$ROOT_TOKEN" > /dev/null 2>&1
kubectl exec --request-timeout=60s -n $NAMESPACE vault-0 -- vault operator raft list-peers
echo ""

echo "=== Vault HA Cluster Initialization Complete! ==="
echo ""
echo "Root Token: $ROOT_TOKEN"
echo ""
echo "Access the Vault UI at: http://<LOADBALANCER_IP>:8200"
echo "Get the LoadBalancer IP with: kubectl get svc -n vault vault-ui"
echo ""
echo "⚠️  Remember to securely store your unseal keys and root token!"
echo "⚠️  Delete $INIT_OUTPUT_FILE after saving the keys elsewhere!"
