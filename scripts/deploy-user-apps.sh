#!/bin/bash
set -e

# Bootstrap script for initial cluster setup.
# After first apply, ArgoCD self-heals from the repo — this script
# is only needed when bootstrapping a fresh cluster.

APPSET_FILE="${APPSET_FILE:-k8s/apps/user-apps-appset.yaml}"
NAMESPACE="argocd"

echo "Deploying User Apps ApplicationSet..."
kubectl apply -n "$NAMESPACE" -f "$APPSET_FILE"

echo "User applications deployed successfully."
