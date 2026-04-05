---
name: infra-onboard-service
description: Onboard a new service to the K3s cluster. Use when adding a new application, deploying a new Helm chart, or setting up a service that needs secrets from OpenBao. Runs through the full checklist — Helm chart, ArgoCD ApplicationSet, OpenBao role, ExternalSecret, resource limits, and observability.
tools: Read, Edit, Write, Bash, Glob, Grep
---

# Service Onboarding Skill

Walk through the complete checklist for deploying a new service to the K3s cluster.

## Checklist

### 1. Helm Chart / Manifests
- [ ] Create chart under `k8s/platform/<name>/` (platform) or `k8s/apps/<group>/<name>/` (app)
- [ ] Set memory limits (no CPU limits per cluster policy)
- [ ] Set `nodeSelector: compute-class: pi5` if it should run on Pi nodes
- [ ] Add `Chart.lock` if using Helm dependencies (required for ArgoCD rendering)

### 2. ArgoCD Registration
- [ ] Add entry to the correct ApplicationSet generator:
  - Platform services: `k8s/bootstrap/core-apps-appset.yaml`
  - User apps: `k8s/bootstrap/user-apps-appset.yaml`
- [ ] Set appropriate sync wave (0=core, 1=secrets, 2=networking, 3=observability, 4=apps)

### 3. Secrets (if needed)
- [ ] Add service to `managed_apps` in `ansible/inventory/group_vars/all/vars.yml`
- [ ] Run: `ansible-playbook site.yml -t openbao_setup -l <control_node>`
- [ ] Create `templates/secret-store.yaml` with `server: "https://openbao.chalupatech.com"`
- [ ] Create `templates/external-secret.yaml` referencing the store
- [ ] Verify: `kubectl get externalsecret -n <namespace>`

### 4. Resource Limits
- [ ] Set memory requests and limits for all containers
- [ ] NO CPU limits (cluster policy — causes CFS throttling on Pi)
- [ ] Base limits on expected usage + 2x headroom

### 5. Observability
- [ ] Add VMServiceScrape or ServiceMonitor if the service exposes metrics
- [ ] Add Grafana dashboard ConfigMap if warranted
- [ ] Verify scrape target appears: port-forward VMAgent and check `/targets`

### 6. Networking
- [ ] Add IngressRoute if the service needs external access
- [ ] Use `*.chalupatech.com` domain (NOT Tailscale hostnames)
- [ ] cert-manager will auto-provision TLS via the ClusterIssuer

## Common Mistakes

- Forgetting to add to `managed_apps` → ExternalSecret fails with "invalid role"
- Using Tailscale hostnames in templates → fails from pod network
- Missing `Chart.lock` → ArgoCD renders with stale/default values
- Missing resource limits → unbounded memory growth, node OOM
- Setting CPU limits → CFS throttling on Pi5 ARM nodes
