#!/bin/bash
set -e

echo "=== Uninstalling HashiCorp Vault ==="

# Uninstall Helm release
if helm list -n vault | grep -q "vault"; then
    echo "Uninstalling Vault Helm release..."
    helm uninstall vault -n vault
else
    echo "Vault Helm release not found."
fi

# Delete PVCs to ensure fresh storage on next install
echo "Deleting PersistentVolumeClaims..."
kubectl delete pvc -l app.kubernetes.io/instance=vault -n vault --ignore-not-found
kubectl delete pvc -l app.kubernetes.io/name=vault -n vault --ignore-not-found

# Delete Namespace
echo "Deleting 'vault' namespace..."
kubectl delete namespace vault --ignore-not-found

echo "Waiting for namespace to terminate..."
kubectl wait --for=delete namespace/vault --timeout=120s || echo "Namespace deletion timed out (it might still be terminating)"

echo "=== Uninstall Complete ==="
