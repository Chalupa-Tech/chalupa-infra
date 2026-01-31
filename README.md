# Chalupa Infra

This repository contains the infrastructure-as-code (GitOps) for the `chalupa-infra` cluster.

## Bootstrap Guide

This guide describes how to bootstrap the GitOps stack on a fresh cluster (tailored for Raspberry Pi / K3s).

### Prerequisites
- A running Kubernetes cluster (K3s recommended).
- `kubectl` configured to talk to your cluster.
- `git` installed.

### 1. Install ArgoCD
Deploy the core GitOps controller. We use a non-HA configuration tuned for low-resource environments.

```bash
# Apply the installation manifest (Namespace: argocd)
kubectl apply -f k8s/platform/argocd/install.yaml

# Wait for pods to start
kubectl wait --for=condition=Ready pods --all -n argocd --timeout=300s
```

### 2. Bootstrap Prometheus CRDs
**Critical Step**: The Prometheus Operator CRDs are too large for the default client-side `kubectl apply` used by Helm/ArgoCD initially. We must manually "seed" them using Server-Side Apply.

```bash
# Run the helper script
chmod +x scripts/bootstrap_observability_crds.sh
./scripts/bootstrap_observability_crds.sh
```

### 3. Deploy the Root Application
The "App of Apps" pattern. This single application will automatically discover and deploy everything else in this repository.

```bash
# Apply the Root App
kubectl apply -f k8s/gitops/root/app.yaml
```

### 4. Verification
Check the status of the applications in ArgoCD:

```bash
# Check Application status
kubectl get applications -n argocd

# Check Pods
kubectl get pods -n argocd
kubectl get pods -n external-secrets
kubectl get pods -n observability
```

## Troubleshooting

### "App path does not exist"
If ArgoCD complains that `k8s/gitops/root` does not exist, it may be looking at the `HEAD` of the default branch (`main`) while you are pushing to a feature branch. Ensure your changes are merged to `main`, or patch the application to point to your branch.

### Prometheus "Missing" or "OutOfSync"
If the Observability app stalls:
1.  Ensure you ran the bootstrap script in Step 2.
2.  Trigger a sync with Server-Side Apply enabled (this is now default in `applications.yaml` but can be forced):
    ```bash
    kubectl patch application observability -n argocd --type merge -p '{"operation": {"sync": {"prune": true, "syncOptions": ["ServerSideApply=true"]}}}'
    ```
