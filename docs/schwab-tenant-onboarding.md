# Schwab Tenant Onboarding Guide

How to deploy a new Schwab service group (go-schwab-auth + go-notify) for a new tenant on the K3s cluster.

## Prerequisites

- Access to the [Schwab Developer Portal](https://developer.schwab.com)
- SSH access to K3s control node (tpi2)
- GitHub access to Chalupa-Tech repos: chalupa-infra, go-schwab-auth, go-notify
- Tailscale access to the tailnet

## Overview

Each tenant gets:
- A dedicated K8s namespace (`schwab-<tenant>`)
- OpenBao secrets (Schwab credentials, Discord webhook, tokens)
- A registration entry in go-schwab-auth (shared instance, per-tenant config)
- ArgoCD-managed deployments (base infrastructure + go-notify + go-schwab-auth)

The shared callback URL (`https://go-schwab-auth.tailbecff0.ts.net/callback`) handles all tenants — the OAuth `state` parameter identifies which registration initiated the flow.

---

## Step 1: Register a Schwab App

1. Log in to [developer.schwab.com](https://developer.schwab.com)
2. Create a new app (or reuse an existing one)
3. Note the **App Key** (client ID) and **Secret** (client secret)
4. Set the callback URL to: `https://go-schwab-auth.tailbecff0.ts.net/callback`

> **Important**: Schwab callback URL changes only propagate after market hours (after 4 PM ET). If you change the callback URL, wait until after hours to test the OAuth flow.

## Step 2: Seed OpenBao Secrets

The tenant needs two secret paths seeded in OpenBao before deployment.

### Schwab credentials

```bash
# From tpi2 or via kubectl exec
kubectl exec -n openbao openbao-0 -- sh -c "
  BAO_TOKEN=<root_token> bao kv put secret/schwab-<tenant>/schwab \
    api_key=<schwab_client_id> \
    api_secret=<schwab_client_secret>
"
```

### Discord webhook

```bash
kubectl exec -n openbao openbao-0 -- sh -c "
  BAO_TOKEN=<root_token> bao kv put secret/schwab-<tenant>/discord \
    webhook_url=<discord_webhook_url>
"
```

The token path (`secret/schwab-<tenant>/tokens/<registration>`) is written automatically by go-schwab-auth after the first OAuth flow completes.

## Step 3: Add Tenant to Ansible Vars

Edit `ansible/inventory/group_vars/all/vars.yml` and add the tenant to `managed_apps`:

```yaml
managed_apps:
  - name: schwab-ddowell
    auth_service: true
    auth_sa: go-schwab-auth
  # Add new tenant:
  - name: schwab-<tenant>
    auth_service: true
    auth_sa: go-schwab-auth
```

The `auth_service: true` flag creates an additional write policy so go-schwab-auth can store tokens in OpenBao.

## Step 4: Run Ansible

Run the OpenBao setup playbook to provision policies and K8s auth roles for the new tenant:

```bash
cd ansible
ansible-playbook -i inventory/hosts.yml playbooks/openbao_setup.yml --tags app_setup
```

This creates:
- K8s namespace `schwab-<tenant>`
- OpenBao read policy: `app-schwab-<tenant>-policy` (reads `secret/data/schwab-<tenant>/*`)
- OpenBao auth write policy: `app-schwab-<tenant>-auth-policy` (writes tokens)
- K8s auth role: `schwab-<tenant>-role` (binds `default` SA)
- K8s auth role: `schwab-<tenant>-auth-role` (binds `go-schwab-auth` SA)

## Step 5: Add Registration to go-schwab-auth

Edit `config/registrations.yaml` in the go-schwab-auth repo:

```yaml
registrations:
  - name: ddowell-individual
    provider: schwab
    scopes: [api]
    vault_secret_path: secret/data/schwab-ddowell/schwab
    vault_token_path: secret/data/schwab-ddowell/tokens/individual
    refresh_token_ttl: 168h
  # Add new registration:
  - name: <tenant>-individual
    provider: schwab
    scopes: [api]
    vault_secret_path: secret/data/schwab-<tenant>/schwab
    vault_token_path: secret/data/schwab-<tenant>/tokens/individual
    refresh_token_ttl: 168h
```

## Step 6: Rebuild go-schwab-auth

Commit and push the registration change. The CI pipeline builds a new container image with the updated `registrations.yaml` baked in:

1. Update `CHANGELOG.md` with the new registration (CI enforces changelog entries)
2. Merge PR to `main`
3. Tag a new release (e.g., `v0.3.0`) — triggers `build-push.yml` workflow
4. New image is pushed to `gitea.tailbecff0.ts.net`

## Step 7: Add ApplicationSet Entries

Edit `k8s/bootstrap/user-apps-appset.yaml` in chalupa-infra and add three entries for the new tenant:

```yaml
generators:
  - list:
      elements:
        # Existing tenant
        - name: schwab-ddowell-base
          path: k8s/apps/schwab/base
          namespace: schwab-ddowell
        - name: schwab-ddowell-go-notify
          path: k8s/apps/schwab/go-notify
          namespace: schwab-ddowell
        - name: schwab-ddowell-go-schwab-auth
          path: k8s/apps/schwab/go-schwab-auth
          namespace: schwab-ddowell
        # New tenant
        - name: schwab-<tenant>-base
          path: k8s/apps/schwab/base
          namespace: schwab-<tenant>
        - name: schwab-<tenant>-go-notify
          path: k8s/apps/schwab/go-notify
          namespace: schwab-<tenant>
        - name: schwab-<tenant>-go-schwab-auth
          path: k8s/apps/schwab/go-schwab-auth
          namespace: schwab-<tenant>
```

The Helm charts in `k8s/apps/schwab/` are tenant-agnostic — the namespace determines which OpenBao secrets are mounted.

## Step 8: Merge to Main

Create a PR in chalupa-infra with the ApplicationSet changes. Once merged:

1. The `appset-manager` ApplicationSet detects the change to `user-apps-appset.yaml`
2. ArgoCD creates three new Applications for the tenant
3. `base` deploys: SecretStore, ExternalSecret (pulls from OpenBao), PVC
4. `go-notify` deploys: CronJob (daily reports) + Deployment (NATS subscriber)
5. `go-schwab-auth` deploys: Deployment, Service, ServiceAccount, Tailscale Ingress

ArgoCD auto-syncs with `prune: true` and `selfHeal: true`.

## Step 9: Complete Initial OAuth

Once deployed, the new tenant needs to authorize their Schwab account:

1. Open `https://go-schwab-auth.tailbecff0.ts.net/` in a browser (requires Tailscale access)
2. Click the link for the new registration (e.g., `<tenant>-individual`)
3. Log in to the Schwab account and approve access
4. The callback exchanges the auth code for tokens and stores them in OpenBao
5. The scheduler begins monitoring token expiry (checks every 6 hours, auto-refreshes at T-1h)

> **Reminder**: If the Schwab callback URL was recently changed, this step must be done after market hours (after 4 PM ET).

## Verification

After completing all steps:

```bash
# Check pods are running
kubectl get pods -n schwab-<tenant>

# Check go-schwab-auth logs for the new registration
kubectl logs -n schwab-<tenant> deploy/go-schwab-auth | head -20

# Verify tokens were written to OpenBao
kubectl exec -n openbao openbao-0 -- sh -c "
  BAO_TOKEN=<root_token> bao kv get secret/schwab-<tenant>/tokens/individual
"

# Check NATS subscriber is connected (go-notify)
kubectl logs -n schwab-<tenant> deploy/go-notify-subscriber | head -10
```

## Tenant Checklist

| Step | Action | Repo |
|------|--------|------|
| 1 | Register Schwab app, get credentials | Schwab Developer Portal |
| 2 | Seed OpenBao secrets (schwab + discord) | K3s cluster |
| 3 | Add to `managed_apps` in vars.yml | chalupa-infra |
| 4 | Run Ansible `openbao_setup` playbook | chalupa-infra |
| 5 | Add registration to `registrations.yaml` | go-schwab-auth |
| 6 | Tag + release go-schwab-auth | go-schwab-auth |
| 7 | Add ApplicationSet entries | chalupa-infra |
| 8 | Merge chalupa-infra PR | chalupa-infra |
| 9 | Complete OAuth flow via Tailscale URL | Browser |
