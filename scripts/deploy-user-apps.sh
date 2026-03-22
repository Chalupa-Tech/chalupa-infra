#!/bin/bash
set -e

APPSET_FILE="${APPSET_FILE:-k8s/apps/user-apps-appset.yaml}"
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

echo "Deploying User Apps ApplicationSet..."
sed "s|{{REPO_URL}}|$REPO_URL|g" "$APPSET_FILE" | kubectl apply -n "$NAMESPACE" -f -

echo "User applications deployed successfully."
