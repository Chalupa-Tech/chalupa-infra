# GEMINI.md - chalupa-infra

## Project Overview
`chalupa-infra` is a GitOps-based infrastructure repository for managing a K3s Kubernetes cluster on Raspberry Pi 5 nodes (8GB or 16GB RAM). It follows a two-stage management lifecycle:
1.  **Ansible**: Used for initial node preparation, K3s installation in HA mode, and bootstrapping ArgoCD.
2.  **ArgoCD**: Once bootstrapped, ArgoCD manages the lifecycle of core platform services and end-user applications using the `ApplicationSet` (App-of-Apps) pattern.

The cluster nodes (`tpi1`, `tpi2`, `tpi3`) are interconnected using **Tailscale** for secure and simplified networking.

## Core Technologies
- **Ansible**: Infrastructure orchestration and cluster bootstrapping.
- **K3s**: Lightweight Kubernetes distribution in a High-Availability (HA) configuration.
- **ArgoCD**: GitOps continuous delivery tool for application lifecycle.
- **Tailscale**: Peer-to-peer VPN for secure node-to-node connectivity and service exposing.
- **cert-manager**: Cluster-wide SSL/TLS certificate management using Let's Encrypt (DNS-01 solver).
- **OpenBao**: HA secret management (Vault fork) with automated initialization, unsealing, and Kubernetes/Userpass auth configuration.
- **Kubernetes (k8s)**: Platform services include `cert-manager`, `external-secrets`, and a full `observability` stack (Prometheus, Grafana, etc.).

## Core Domains
All services are exposed via the root domain `chalupatech.com`:
- **ArgoCD**: `https://argocd.chalupatech.com`
- **Grafana**: `https://grafana.chalupatech.com`
- **OpenBao**: `https://openbao.chalupatech.com`

## Directory Structure
- `scripts/raspberrypi-k3s/ansible/`: Ansible configuration, playbooks, and roles.
  - `roles/`: Modular components (`argocd_setup`, `k3s_install`, `openbao_setup`, `pi_prepare`, `tailscale_services`, `user_setup`).
  - `playbooks/site.yml`: Main multi-stage orchestration playbook.
- `k8s/platform/`: ArgoCD `ApplicationSet` and manifests for core infrastructure services (`cert-manager`, `external-secrets`, `observability`, `openbao`).
- `k8s/apps/`: Definitions for end-user applications.
- `scripts/`: Utility scripts for deployment, local kubeconfig fetching, and troubleshooting.
- `.github/workflows/`: CI/CD pipelines for infrastructure validation (Ansible/YAML linting) and automation.
- `CONVENTIONS.md`: Architectural and operational standards (e.g., SSL policy).

## Key Workflows

### 1. Bootstrapping the Cluster
The cluster is provisioned in four distinct stages via Ansible:
1.  **Primary Server**: Provisions the first control plane node, registers its Tailscale IP, and captures the K3s node token.
2.  **HA Servers**: Joins additional control plane nodes to the primary using discovered facts.
3.  **Agents**: (Optional) Joins worker nodes to the control plane.
4.  **Finalize**: Bootstraps ArgoCD and OpenBao (including initialization and unsealing).

Command to execute:
```bash
cd scripts/raspberrypi-k3s/ansible
ansible-playbook playbooks/site.yml --ask-vault-pass
```
*Note: Secrets (Tailscale AuthKey, Cloudflare API Key) are managed via Ansible Vault in `inventory/group_vars/all/vault.yml`. Ensure you have the vault password.*

### 2. Managing Applications (GitOps)
New applications are added by:
1. Creating manifests in `k8s/apps/<app-name>`.
2. Adding an entry to the `generators.list.elements` in an `ApplicationSet` (e.g., `k8s/apps/example-app-appset.yaml`).
3. Committing and pushing changes to the repository's `main` branch (ArgoCD is configured to track `main`).

### 3. Local Environment Setup
- **Kubeconfig**: Fetch the cluster's kubeconfig locally using:
  ```bash
  bash scripts/raspberrypi-k3s/local/kube-config/fetch-kubeconfig.sh
  ```
- **Observability**: If observability sync fails due to CRD size, run:
  ```bash
  bash scripts/raspberrypi-k3s/local/observability/bootstrap_observability_crds.sh
  ```
- **Accessing Services**: Core services are accessible over HTTPS via their `chalupatech.com` subdomains once DNS is configured correctly.

## Development Conventions
- **Workflow**: All changes must be merged via Pull Requests (PRs). Direct commits to the main branch are discouraged/restricted.
- **SSL/TLS**: Mandatory for all external endpoints. Certificates are managed by `cert-manager` using the `letsencrypt-prod` ClusterIssuer (see `CONVENTIONS.md`).
- **Ansible**: Roles are modular and located in `scripts/raspberrypi-k3s/ansible/roles/`. Use `ansible-lint` to verify playbooks.
- **Secrets Management**: Sensitive data (API keys, auth tokens) MUST be stored in `scripts/raspberrypi-k3s/ansible/inventory/group_vars/all/vault.yml` using Ansible Vault.
    - **Promptless Running**: It is recommended to set the `ANSIBLE_VAULT_PASSWORD_FILE` environment variable to point to a local password file to avoid manual password entry.
    - **Variable Naming**: Vaulted variables should be prefixed with `vault_` and mapped to plaintext variable names in `vars.yml`.
- **Kubernetes**: Prefer `ApplicationSet` for grouping related applications. Namespace creation and server-side apply are standard sync options in `ApplicationSets`.
- **Networking**: All node communication and Kubernetes API access are routed through Tailscale.

## Pull Request Process
1.  **Branching**: All changes must be developed on a feature or fix branch (e.g., `feat/my-feature` or `fix/issue-description`).
2.  **Linting**: Ensure all YAML and Ansible files pass `yamllint` and `ansible-lint` locally before pushing.
3.  **Opening PR**: Create a Pull Request targeting the `main` branch.
4.  **Verification**: PR body must describe the Goal, Changes, and Testing. All CI checks (validation actions) must pass.
5.  **Merging**: Once approved and checks are green, PRs are merged into `main`, which ArgoCD automatically tracks for deployment.

## Maintenance & Troubleshooting
- **Logs**: Standard `kubectl logs` and ArgoCD UI (`https://argocd.chalupatech.com`).
- **Linter**: `.ansible-lint` and `.yamllint` are configured for the project.
