# chalupa-infra — Claude CLI Project Instructions

## What This Is

GitOps infrastructure-as-code repository managing a Raspberry Pi 5 K3s cluster. Two-stage lifecycle: Ansible bootstraps nodes and core services, then ArgoCD manages ongoing application delivery.

## Architecture Overview

- **K3s HA Cluster**: 3 control planes (tpi2, dpi2, dpi3) + 3 workers (tpi1, tpi3, dpi1), all Raspberry Pi 5 (8GB RAM)
- **Gitea**: Docker-hosted on docker1 (15.204.88.41), serves as Git server + container/package registry
- **ArgoCD**: GitOps CD — watches `main` branch, syncs ApplicationSets
- **OpenBao**: HA secret management (Vault fork), 3-node Raft cluster
- **External Secrets Operator**: Bridges OpenBao secrets into k8s Secrets
- **Tailscale**: VPN overlay connecting all nodes (tailbecff0.ts.net domain)
- **Observability**: Kube Prometheus Stack (Prometheus + Grafana + Alertmanager)
- **cert-manager**: TLS via Cloudflare DNS validation

## Project Layout

```
ansible/                        # Ansible orchestration
  inventory/                    # Hosts, group_vars (vars.yml + vault.yml)
  playbooks/                    # site.yml (main), docker-node.yml, openbao-only.yml
  roles/                        # Modular: k3s_install, argocd_setup, openbao_setup,
                                #   gitea_setup, tailscale_install, user_setup, pi_prepare
k8s/
  platform/                     # Core infrastructure ApplicationSets
    argocd/                     # ArgoCD install manifests
    cert-manager/               # Helm chart wrapper
    external-secrets/           # ESO + ClusterSecretStore
    observability/              # Kube Prometheus Stack
    openbao/                    # OpenBao HA Helm chart
    management-proxy/           # Nginx reverse proxy
    tailscale-operator/         # Tailscale ingress
    core-apps-appset.yaml       # ApplicationSet for all platform services
  apps/                         # End-user application manifests
    example-app-appset.yaml     # Template ApplicationSet for apps
scripts/                        # Deploy scripts, local utilities
.github/workflows/              # CI (validate-infra.yml) + CD (deploy-infra.yml)
docs/                           # Architecture guides, implementation docs
```

## Key Commands

```bash
# Bootstrap cluster (requires vault password)
cd ansible && ansible-playbook playbooks/site.yml --ask-vault-pass

# Deploy only Docker node (Gitea)
cd ansible && ansible-playbook playbooks/docker-node.yml --ask-vault-pass

# Fetch local kubeconfig
bash scripts/raspberrypi-k3s/local/kube-config/fetch-kubeconfig.sh

# Bootstrap observability CRDs (if ArgoCD sync fails on large CRDs)
bash scripts/raspberrypi-k3s/local/observability/bootstrap_observability_crds.sh

# Deploy core platform apps to ArgoCD
bash scripts/deploy-core-apps.sh
```

## Secret Management (Two-Tier)

1. **Ansible Vault** (Tier 1 — bootstrap): `ansible/inventory/group_vars/all/vault.yml`. Used for core infrastructure secrets (Tailscale authkey, Cloudflare API key, OpenBao root token). Core platform apps MUST NOT depend on OpenBao for bootstrap.
2. **OpenBao** (Tier 2 — runtime): All application secrets. Accessed via External Secrets Operator. Per-namespace access control via Kubernetes auth roles.

## Conventions

- **GitOps**: All changes via PRs to `main`. ArgoCD auto-syncs on merge.
- **ApplicationSets**: Use App-of-Apps pattern. Platform services in `core-apps-appset.yaml`, user apps in `apps/`.
- **Linting**: yamllint, ansible-lint (production profile), shellcheck. CI runs on all PRs.
- **Secrets**: Vault variables prefixed with `vault_`, mapped to plaintext names in `vars.yml`.
- **Resource limits**: Raspberry Pi nodes have 8GB RAM. All deployments must specify resource requests/limits.

## Networking

| Service | URL |
|---------|-----|
| Gitea | `https://gitea.tailbecff0.ts.net` |
| ArgoCD | `https://pi-k3s.chalupatech.com/argocd` |
| Grafana | `https://grafana.tailbecff0.ts.net` |
| OpenBao | `https://openbao.tailbecff0.ts.net` |

All services accessible via Tailscale. Public access via pi-k3s.chalupatech.com (cert-manager + Cloudflare).

## Current State

- Platform services deployed and operational
- No user applications deployed yet (go-notify is first target)
- ClusterSecretStore references `go-notify` namespace/role but neither exists yet
- Gitea container registry enabled but K3s nodes lack registry auth config
- GitHub Actions CI/CD operational (Tailscale join pattern for VPN access)
