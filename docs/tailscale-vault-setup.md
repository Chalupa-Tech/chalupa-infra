# Tailscale Operator — Vault Setup

One-time manual steps required before ArgoCD syncs the `tailscale-operator` app.

## 1. Write secrets to OpenBao

```bash
vault kv put secret/platform/tailscale-operator \
  client_id="<YOUR_OAUTH_CLIENT_ID>" \
  client_secret="<YOUR_OAUTH_CLIENT_SECRET>"
```

## 2. Create a Vault policy

```hcl
# tailscale-operator.hcl
path "secret/data/platform/tailscale-operator" {
  capabilities = ["read"]
}
```

```bash
vault policy write tailscale-operator tailscale-operator.hcl
```

## 3. Wire Kubernetes auth

The `vault-backend` ClusterSecretStore authenticates using Kubernetes auth. You need a role that authorises the `external-secrets` service account to read the secret above.

**Option A — new dedicated role (recommended):**

```bash
vault write auth/kubernetes/role/external-secrets-role \
  bound_service_account_names=external-secrets \
  bound_service_account_namespaces=external-secrets \
  policies=tailscale-operator \
  ttl=1h
```

Then update `k8s/platform/external-secrets/cluster-secret-store.yaml` to use `external-secrets-role` and the `external-secrets` namespace:

```yaml
auth:
  kubernetes:
    mountPath: "kubernetes"
    role: "external-secrets-role"
    serviceAccountRef:
      name: "external-secrets"
      namespace: "external-secrets"
```

**Option B — extend the existing `go-notify-role`:**

```bash
vault write auth/kubernetes/role/go-notify-role \
  policies="go-notify,tailscale-operator"
  # (keep all other existing attributes)
```

> Option A is cleaner — it decouples ESO's Vault identity from any single app namespace.

## 4. Verify

Once ArgoCD syncs:

```bash
kubectl get externalsecret operator-oauth -n tailscale-operator
kubectl get secret operator-oauth -n tailscale-operator
kubectl get pods -n tailscale-operator
```

The operator pod should come up and a new device tagged `tag:k8s-operator` should appear in the Tailscale admin console.
