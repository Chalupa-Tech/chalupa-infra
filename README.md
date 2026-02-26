# Chalupa Infra

This repository contains the infrastructure-as-code (GitOps) for the `chalupa-infra` cluster.

## Architecture & Flow

The infrastructure is managed in two primary stages:
1.  **Ansible**: Bootstraps the Raspberry Pi nodes, installs K3s (HA), and initializes ArgoCD with core services.
2.  **ArgoCD**: Manages the lifecycle of core platform services and end-user applications using the `ApplicationSet` pattern.

### Node & Service Orchestration
The main playbook `playbooks/site.yml` is executed in four sequential stages:
1.  **Primary Server (`tpi2`)**: Provisions the first control plane node, registers its Tailscale IP, and captures the cluster token.
2.  **HA Servers (`tpi1`, `tpi3`)**: Join the cluster as additional control plane nodes using the discovered token and IP.
3.  **Agents**: (Optional) Join worker-only nodes.
4.  **Finalize**: Bootstraps ArgoCD and OpenBao on the control plane.

## Bootstrap Guide

### 1. Prerequisites
- **Tailscale**: Nodes should be authenticated to your tailnet.
- **SSH Access**: Ensure you have SSH access to the nodes (usually via Tailscale IP).
- **Ansible**: Installed locally to run the playbooks.

### 2. Ansible Authentication (Secrets)
Secrets are managed using **Ansible Vault**. The encrypted secrets are stored in `scripts/raspberrypi-k3s/ansible/inventory/group_vars/all/vault.yml`.

#### Running Playbooks
When running playbooks, you must provide the vault password:
```bash
ansible-playbook playbooks/site.yml --ask-vault-pass
```

#### Promptless Execution (Recommended)
For a smoother experience, you can store your vault password in a file (e.g., `~/.vault_pass.txt`) and set the `ANSIBLE_VAULT_PASSWORD_FILE` environment variable:

1.  Create the password file: `echo "your_password" > ~/.vault_pass.txt`
2.  Set the environment variable: `export ANSIBLE_VAULT_PASSWORD_FILE=~/.vault_pass.txt`
3.  Run playbooks without flags: `ansible-playbook playbooks/site.yml`

*Note: Never commit your vault password file to the repository.*

### 3. Provision the Cluster
Run the main playbook from the `scripts/raspberrypi-k3s/ansible` directory:

```bash
cd scripts/raspberrypi-k3s/ansible
ansible-playbook playbooks/site.yml
```

This command will:
- Set up Linux users, SSH keys, and passwordless sudo.
- Prepare nodes (cgroups memory enablement).
- Install K3s in an HA configuration with Tailscale service exposing.
- **Bootstrap GitOps**: Install Helm and ArgoCD on the primary node.
- **Configure OpenBao**: Initialize, unseal, and configure auth methods (Userpass & Kubernetes) and user-specific namespaces.
- **Deploy Core Services**: Deploy the core `ApplicationSet` which includes `cert-manager`, `external-secrets`, `observability`, and `openbao`.

### 4. Accessing Services

#### ArgoCD
ArgoCD is configured with an Ingress at `https://argocd.local`.

**Managed Users & Passwords**:
Initial passwords for users defined in `inventory/group_vars/all.yml` (e.g., `tbigelow`, `ddowell`) are set to:
`ArgoCD123!`

#### OpenBao (Secret Management)
OpenBao is available within the cluster and manages secrets in per-user namespaces.

**Authentication for Managed Users**:
- **Userpass (Web UI/CLI)**:
    - **Namespace**: `{{ username }}` (e.g., `tbigelow`)
    - **Method**: `userpass`
    - **Username**: `{{ username }}`
    - **Password**: `OpenBao123!`
- **Kubernetes Auth (Applications)**:
    - **Namespace**: `{{ username }}`
    - **Role**: `{{ username }}-role`
    - **Service Account**: `{{ username }}-sa`
    - **Bound Namespace**: `{{ username }}`

Policies are automatically created granting users full administrative rights (`create`, `read`, `update`, `delete`, `list`, `sudo`) within their assigned OpenBao namespace.

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
