#!/bin/bash
set -e

NAMESPACE="argocd"

echo "=== Uninstalling ArgoCD ==="

# 1. Clean up Applications and Projects (handling finalizers)
if kubectl get ns "$NAMESPACE" >/dev/null 2>&1; then
    echo "Cleaning up ArgoCD Applications..."
    apps=$(kubectl get applications.argoproj.io -n "$NAMESPACE" -o name 2>/dev/null || true)
    if [ -n "$apps" ]; then
        echo "Found applications: $apps"
        kubectl delete $apps -n "$NAMESPACE" --timeout=30s || true
        # Remove finalizers if still existing
        for app in $apps; do
            if kubectl get "$app" -n "$NAMESPACE" >/dev/null 2>&1; then
                echo "Removing finalizers for $app..."
                kubectl patch "$app" -n "$NAMESPACE" --type json -p='[{"op": "remove", "path": "/metadata/finalizers"}]' 2>/dev/null || true
            fi
        done
    fi

    echo "Cleaning up ArgoCD AppProjects..."
    projects=$(kubectl get appprojects.argoproj.io -n "$NAMESPACE" -o name 2>/dev/null || true)
    if [ -n "$projects" ]; then
        kubectl delete $projects -n "$NAMESPACE" --ignore-not-found
    fi
fi

# 2. Uninstall Helm release
if helm list -n "$NAMESPACE" | grep -q "argocd"; then
    echo "Uninstalling ArgoCD Helm release..."
    helm uninstall argocd -n "$NAMESPACE"
else
    echo "ArgoCD Helm release not found."
fi

# 3. Delete Namespace
if kubectl get ns "$NAMESPACE" >/dev/null 2>&1; then
    echo "Deleting '$NAMESPACE' namespace..."
    kubectl delete namespace "$NAMESPACE" --ignore-not-found

    echo "Waiting for namespace to terminate..."
    kubectl wait --for=delete namespace/"$NAMESPACE" --timeout=120s || echo "Namespace deletion timed out (it might still be terminating)"
fi

# 4. Delete CRDs
echo "Deleting ArgoCD CRDs..."
kubectl delete crd applications.argoproj.io applicationsets.argoproj.io appprojects.argoproj.io --ignore-not-found

# 5. Clean up cluster-wide resources
echo "Cleaning up cluster-wide resources..."
kubectl delete clusterrole -l app.kubernetes.io/part-of=argocd --ignore-not-found
kubectl delete clusterrolebinding -l app.kubernetes.io/part-of=argocd --ignore-not-found

echo "=== ArgoCD Uninstall Complete ==="
