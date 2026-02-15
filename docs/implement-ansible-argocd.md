# Implementation: Ansible ArgoCD & Cluster Orchestration

This document summarizes the automation implemented for the Raspberry Pi K3s cluster using Ansible, including the multi-play orchestration and the dynamic ArgoCD setup.

## 1. Multi-Stage Orchestration
The playbook `playbooks/site.yml` was restructured into three sequential plays to handle cluster dependencies:
- **Primary Server (tpi2)**: Provisions the first node and registers its Tailscale IP and K3s Node Token.
- **HA Servers (tpi1)**: Joins the cluster as additional control plane nodes using the primary's discovered IP and Token.
- **Agents**: Joins worker nodes to the established control plane.

## 2. Dynamic Secret & Fact Discovery
- **.env Fallback**: `inventory/group_vars/all.yml` uses `ini` lookups to automatically read `TS_AUTHKEY` and `K3S_TOKEN` from a local `.env` file if environment variables are missing.
- **Token Capture**: The primary server play extracts the K3s node token directly from the installation script's output using regex, removing the need to manually distribute tokens.

## 3. ArgoCD Role (`roles/argocd_setup`)
A new role was created to bootstrap the cluster's GitOps engine on the primary server:
- **Prerequisite Handling**: Automatically installs `helm` via the official script if not found on the target.
- **Namespacing**: Ensures the `argocd` namespace exists.
- **Custom Values**: Templates `argocd-values.yaml` to:
    - Enable local accounts for all users in the `managed_users` list.
    - Configure RBAC to grant `role:admin` to users marked with `argocd_admin: true`.
    - Enable `--insecure` mode for simplified internal access.
- **User Provisioning**: Patches the `argocd-secret` to set initial passwords (`ArgoCD123!`) and `passwordMtime` for all managed users.

## 4. Managed User Configuration
Users are defined globally in `inventory/group_vars/all.yml`:
```yaml
managed_users:
  - name: ddowell
    pubkey_file: "..."
    argocd_admin: true
  - name: tbigelow
    pubkey_file: "..."
    argocd_admin: true
```
This single source of truth controls both Linux user setup (SSH keys, sudoers) and ArgoCD access control.

## 5. Execution
The entire environment can be provisioned or updated with a single command:
```bash
ansible-playbook playbooks/site.yml
```
