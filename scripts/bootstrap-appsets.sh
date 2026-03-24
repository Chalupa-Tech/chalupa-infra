#!/bin/bash
set -e

# Bootstrap script for initial cluster setup.
# Applies the appset-manager Application, which then syncs both
# ApplicationSet files from k8s/bootstrap/ automatically.
# After first apply, ArgoCD self-heals — this script is only
# needed when bootstrapping a fresh cluster.

CRD_BOOTSTRAP_SCRIPT="${CRD_BOOTSTRAP_SCRIPT:-scripts/raspberrypi-k3s/local/observability/bootstrap_observability_crds.sh}"

# Ensure CRDs are bootstrapped first
if [ -f "$CRD_BOOTSTRAP_SCRIPT" ]; then
    echo "Running CRD bootstrap script..."
    bash "$CRD_BOOTSTRAP_SCRIPT"
else
    echo "Warning: CRD bootstrap script not found at $CRD_BOOTSTRAP_SCRIPT"
fi

echo "Deploying appset-manager (manages all ApplicationSets)..."
kubectl apply -n argocd -f k8s/bootstrap/appset-manager.yaml

echo "Bootstrap complete. ArgoCD will sync ApplicationSets from git."
