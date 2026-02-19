#!/bin/bash
set -e

NAMESPACE="openbao"

echo "=== Cleaning up OpenBao on Kubernetes ==="

if kubectl get namespace "$NAMESPACE" >/dev/null 2>&1; then
  echo "Deleting OpenBao Helm release (if any)..."
  helm uninstall openbao -n "$NAMESPACE" 2>/dev/null || true

  echo "Deleting OpenBao namespace and resources..."
  kubectl delete namespace "$NAMESPACE" --timeout=60s || true

  # Force delete PVCs if the namespace deletion is slow or stuck
  echo "Ensuring all PVCs are cleared..."
  kubectl get pvc -n "$NAMESPACE" -o name | xargs kubectl delete -n "$NAMESPACE" --force --grace-period=0 2>/dev/null || true
else
  echo "Namespace '$NAMESPACE' not found."
fi

echo "=== Local Marker Cleanup (Manual Action Required) ==="
echo "To fully reset OpenBao for Ansible, you MUST delete the initialization markers on the primary node (tpi1/tpi2/tpi3):"
echo "  sudo rm /var/lib/rancher/k3s/server/openbao_init_data.json"
echo ""
echo "=== OpenBao Cleanup Complete ==="
