# Chalupa Infra

This repository contains the infrastructure-as-code (GitOps) for the `chalupa-infra` cluster.

## Architecture & Flow

The infrastructure is managed in two stages:
1.  **Ansible**: Bootstraps the Raspberry Pi nodes, installs K3s (HA), and initializes ArgoCD with core services.
2.  **ArgoCD**: Manages the lifecycle of core platform services and end-user applications using the `ApplicationSet` pattern.

### Node Ordering
The cluster is provisioned in a specific order to establish the control plane:
1.  **Primary Server (`tpi2`)**: The first control plane node. It generates the cluster token and initializes the database.
2.  **HA Servers (`tpi1`, `tpi3`)**: Join the cluster as additional control plane nodes.
3.  **Agents**: (Optional) Join as worker-only nodes.

## Bootstrap Guide

### 1. Prerequisites
- **SSH Access**: Ensure you have SSH access to the Raspberry Pis.
- **Ansible**: Installed on your local machine.
- **Tailscale Auth Key**: Required for node networking.

### 2. Ansible Authentication
Ansible looks for secrets in the following order:
1.  Environment variables: `TS_AUTHKEY`, `K3S_TOKEN`.
2.  A `.env` file located at `scripts/raspberrypi-k3s/ansible/.env`.

Example `.env`:
```ini
TS_AUTHKEY=tskey-auth-xxxxxx
K3S_TOKEN=optional-custom-token
```

### 3. Provision the Cluster
Run the main playbook from the `scripts/raspberrypi-k3s/ansible` directory:

```bash
cd scripts/raspberrypi-k3s/ansible
ansible-playbook playbooks/site.yml
```

This command will:
- Set up Linux users and SSH keys.
- Install K3s in an HA configuration.
- Install Helm and ArgoCD on the primary node.
- **Bootstrap Core Services**: Deploy `cert-manager`, `external-secrets`, and the `observability` stack via a core `ApplicationSet`.

### 4. Accessing ArgoCD
ArgoCD is configured with an Ingress at `https://argocd.local`.

**Managed Users & Passwords**:
Initial passwords for users defined in `inventory/group_vars/all.yml` (e.g., `tbigelow`, `ddowell`) are set to:
`ArgoCD123!`

It is recommended to change these immediately upon first login.

## Managing Applications

We use ArgoCD `ApplicationSets` to manage groups of applications.

### Core Services
Core platform services are defined in `k8s/platform/core-apps-appset.yaml`. These are bootstrapped during the Ansible run but are managed by ArgoCD thereafter.

### Adding New Applications
To add new applications, follow the pattern in `k8s/apps/example-app-appset.yaml`.

1.  Create your application manifests in a sub-folder (e.g., `k8s/apps/my-new-app`).
2.  Add an entry to the `generators.list.elements` in an ApplicationSet:

```yaml
- name: my-new-app
  path: k8s/apps/my-new-app
```

3.  Apply the ApplicationSet (or let the "App of Apps" discover it):

```bash
# Manual apply with repo URL injection
export REPO_URL=$(git remote get-url origin)
sed "s|{{REPO_URL}}|$REPO_URL|g" k8s/apps/example-app-appset.yaml | kubectl apply -f -
```

## Troubleshooting

### Prometheus Operator CRDs
The `observability` stack requires large CRDs. If the sync fails, ensure the bootstrap script was run:
```bash
bash scripts/raspberrypi-k3s/local/observability/bootstrap_observability_crds.sh
```
This script is automatically called by the bootstrap process.
