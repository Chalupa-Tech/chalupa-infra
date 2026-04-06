---
workstream: fw-infra
repo: chalupa-infra
priority: high
depends_on: []
---

# Phase 8: Node Lifecycle — Version Pinning, Bootstrap Validation, and Lifecycle Unification

## Problem

tpi2 joined the cluster with K3s v1.34.6+k3s1 while every other node runs v1.34.5+k3s1. The kubelet reported Ready but the control plane couldn't proxy to it (502 on port 10250), no flannel pod was scheduled, and metrics-server couldn't reach it. Any pod scheduled there crash-looped. This went undetected until the CNPG operator landed on it.

Root cause: the bootstrap has no version pinning. Every `curl -sfL https://get.k3s.io | sh` grabs whatever stable currently resolves to. There's also no post-join validation — a node registers as Ready even when it's fundamentally broken.

Additionally, the install/uninstall/promote/demote playbooks duplicate logic (CNI cleanup, K3s install invocations, verification steps) without sharing it, and the standalone shell script in `scripts/raspberrypi-k3s/local/k3s/uninstall-k3s.sh` duplicates the Ansible uninstall logic entirely.

## Goals

1. **Version pinning**: Single `k3s_version` variable controls what every install path uses
2. **Bootstrap validation**: Every node join (bootstrap, promote, demote) verifies the node actually works
3. **Lifecycle unification**: Shared roles/tasks for install, uninstall, verify — no duplication
4. **Drift detection**: Easy way to check if any node is running a different version

## Current State

| Script | Version pinned? | Post-join validation? | CNI cleanup? |
|--------|----------------|----------------------|--------------|
| `ansible/remote/install.sh` | No | No | No |
| `roles/k3s_install/` | No (skips if binary exists) | No | No |
| `playbooks/promote-agent.yml` | No (`curl \| sh`) | Checks role label only | Yes (inline) |
| `playbooks/demote-server.yml` | No (via k3s_install role) | Checks Ready + flannel | Yes (inline) |
| `scripts/.../uninstall-k3s.sh` | N/A | N/A | Yes (inline, duplicated) |

## Implementation Plan

### 1. Add `k3s_version` to group_vars

```yaml
# ansible/inventory/group_vars/all/vars.yml
k3s_version: "v1.34.5+k3s1"
```

### 2. Update `install.sh` to accept version

Pass `INSTALL_K3S_VERSION` from the Ansible environment:

```yaml
# roles/k3s_install/tasks/main.yml
environment:
  INSTALL_K3S_VERSION: "{{ k3s_version }}"
  TS_AUTHKEY: "{{ ts_authkey }}"
  K3S_TOKEN: "{{ k3s_install_token | default('') }}"
```

Remove the "skip if binary exists" guard — instead, compare installed version to `k3s_version` and re-run if they differ. This makes the role idempotent for upgrades too.

### 3. Create shared roles

**`roles/k3s_verify/`** — Post-join validation (extract from demote-server.yml Phase 4):
- Node shows Ready
- Kubelet version matches `k3s_version`
- Flannel pod is running on the node
- Metrics are reporting (`kubectl top node <name>`)
- kubectl exec works (proxy to kubelet functional)

**`roles/k3s_cleanup/`** — Uninstall and network cleanup (extract duplicated logic):
- Stop services, run uninstall scripts
- Remove residual directories
- Delete CNI interfaces, flush iptables
- Reboot for clean state

### 4. Update lifecycle playbooks to use shared roles

- `promote-agent.yml`: Replace inline `curl | sh` with `k3s_install` role + `k3s_verify` role
- `demote-server.yml`: Replace inline cleanup with `k3s_cleanup` role, replace inline verify with `k3s_verify` role
- `bootstrap.yml`: Add `k3s_verify` role after each node group joins

### 5. Delete duplicates

- Remove `scripts/raspberrypi-k3s/local/k3s/uninstall-k3s.sh` (replaced by `k3s_cleanup` role)

### 6. Add drift detection playbook

`playbooks/k3s-version-check.yml` — runs against all nodes, compares installed version to `k3s_version`, reports any mismatches. Quick audit tool.

## Verification

1. Drain and remove tpi2 from cluster
2. Re-add tpi2 using updated bootstrap — verify it gets `v1.34.5+k3s1`
3. Run `k3s-version-check.yml` — all nodes should match
4. Run promote/demote cycle on a test node — verify version consistency and validation passes
5. Temporarily set `k3s_version` to a wrong value and bootstrap a node — verify it installs the pinned version, not latest

## Risk

Low — no changes to running nodes. Only affects how new nodes join and how lifecycle playbooks operate. tpi2 is already broken and needs re-bootstrapping regardless.

## Context

- tpi2 was previously removed from inventory after wireguard-native migration (host key changed, SSH unreachable)
- tpi2 somehow rejoined with a newer K3s version — either manual install or automated reinstall
- The CNPG operator crash-looped on tpi2 because kubelet proxy was broken (502 Bad Gateway)
- Community consensus: pin `INSTALL_K3S_VERSION` in Ansible, optionally use system-upgrade-controller for rolling upgrades of existing nodes
