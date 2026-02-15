#!/bin/bash
set -e

# Configuration
APPSET_FILE="${APPSET_FILE:-k8s/platform/core-apps-appset.yaml}"
CRD_BOOTSTRAP_SCRIPT="${CRD_BOOTSTRAP_SCRIPT:-scripts/raspberrypi-k3s/local/observability/bootstrap_observability_crds.sh}"
NAMESPACE="argocd"

# Determine Repo URL
if [ -n "$REPO_URL" ]; then
    echo "Using provided REPO_URL: $REPO_URL"
elif git remote get-url origin >/dev/null 2>&1; then
    REPO_URL=$(git remote get-url origin)
    echo "Detected REPO_URL from git: $REPO_URL"
else
    echo "Error: REPO_URL environment variable not set and could not detect git remote."
    exit 1
fi

# Ensure CRDs are bootstrapped first
if [ -f "$CRD_BOOTSTRAP_SCRIPT" ]; then
    echo "Running CRD bootstrap script..."
    bash "$CRD_BOOTSTRAP_SCRIPT"
else
    echo "Warning: CRD bootstrap script not found at $CRD_BOOTSTRAP_SCRIPT"
fi

# Apply the ApplicationSet with the correct Repo URL
echo "Deploying Core ApplicationSet..."
sed "s|{{REPO_URL}}|$REPO_URL|g" "$APPSET_FILE" | kubectl apply -n "$NAMESPACE" -f -

echo "Core applications deployed successfully."
