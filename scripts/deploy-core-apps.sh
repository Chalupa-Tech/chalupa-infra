#!/bin/bash
set -e

# Bootstrap script for initial cluster setup.
# After first apply, ArgoCD self-heals from the repo — this script
# is only needed when bootstrapping a fresh cluster or re-applying CRDs.

APPSET_FILE="${APPSET_FILE:-k8s/platform/core-apps-appset.yaml}"
CRD_BOOTSTRAP_SCRIPT="${CRD_BOOTSTRAP_SCRIPT:-scripts/raspberrypi-k3s/local/observability/bootstrap_observability_crds.sh}"
NAMESPACE="argocd"

# Ensure CRDs are bootstrapped first
if [ -f "$CRD_BOOTSTRAP_SCRIPT" ]; then
    echo "Running CRD bootstrap script..."
    bash "$CRD_BOOTSTRAP_SCRIPT"
else
    echo "Warning: CRD bootstrap script not found at $CRD_BOOTSTRAP_SCRIPT"
fi

echo "Deploying Core ApplicationSet..."
kubectl apply -n "$NAMESPACE" -f "$APPSET_FILE"

echo "Core applications deployed successfully."
