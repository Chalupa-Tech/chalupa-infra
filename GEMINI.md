# GEMINI.md - chalupa-infra

## Project Overview
`chalupa-infra` is a GitOps-based infrastructure repository for managing a K3s Kubernetes cluster on Raspberry Pi nodes. It follows a two-stage management lifecycle:
1.  **Ansible**: Used for initial node preparation, K3s installation in HA mode, and bootstrapping ArgoCD.
2.  **ArgoCD**: Once bootstrapped, ArgoCD manages the lifecycle of core platform services and end-user applications using the `ApplicationSet` (App-of-Apps) pattern.

The cluster nodes (`tpi1`, `tpi2`, `tpi3`) are interconnected using **Tailscale** for secure and simplified networking.

## Core Technologies
- **Ansible**: Infrastructure orchestration and cluster bootstrapping.
- **K3s**: Lightweight Kubernetes distribution in a High-Availability (HA) configuration.
- **ArgoCD**: GitOps continuous delivery tool.
- **Tailscale**: Peer-to-peer VPN for node connectivity.
- **Kubernetes (k8s)**: Platform services include `cert-manager`, `external-secrets`, and a full `observability` stack (Prometheus, Grafana, etc.).

## Directory Structure
- `scripts/raspberrypi-k3s/ansible/`: Ansible roles, playbooks, and inventory for cluster setup.
- `k8s/platform/`: ArgoCD `ApplicationSet` and manifests for core infrastructure services.
- `k8s/apps/`: Definitions for end-user applications.
- `scripts/`: Utility scripts for deployment, local kubeconfig fetching, and troubleshooting.
- `.github/workflows/`: CI/CD pipelines for infrastructure validation and Gemini-powered automation.

## Key Workflows

### 1. Bootstrapping the Cluster
The cluster is provisioned from the Ansible directory:
```bash
cd scripts/raspberrypi-k3s/ansible
ansible-playbook playbooks/site.yml
```
*Note: Secrets should be provided via environment variables (`TS_AUTHKEY`, `K3S_TOKEN`) or a `.env` file in the ansible directory.*

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

## Development Conventions
- **Workflow**: All changes must be merged via Pull Requests (PRs). Direct commits to the main branch are discouraged/restricted.
- **Ansible**: Roles are modular and located in `scripts/raspberrypi-k3s/ansible/roles/`. Use `ansible-lint` to verify playbooks.
- **Kubernetes**: Prefer `ApplicationSet` for grouping related applications. Namespace creation and server-side apply are standard sync options in `ApplicationSets`.
- **Networking**: All node communication and Kubernetes API access are routed through Tailscale.

## Maintenance & Troubleshooting
- **Logs**: Standard `kubectl logs` and ArgoCD UI (`https://argocd.local`).
- **Linter**: `.ansible-lint` and `.yamllint` are configured for the project.
