# Repo Structure, Skills, and Documentation Overhaul

## Goal

Restructure chalupa-infra's Claude Code integration and documentation to be consistent, discoverable, and useful for any contributor.

## Decisions Needed

1. **Directory naming**: `prompts/` vs something else for session prompts. Current state: `prompts/fw-infra/` with a few files. Whatever we pick, be consistent.

2. **Skills location**: `.claude/skills/` in the repo (committed). Currently has `infra-hotfix` and `infra-onboard-service` at user-level (`~/.claude/skills/`). Move canonical copies into the repo.

## Tasks

### 1. Create/Update CLAUDE.md for chalupa-infra

The repo has a CLAUDE.md but it may be stale. It should cover:
- Repo layout (k8s/platform, k8s/apps, ansible/roles)
- PR workflow (branch from main, ArgoCD auto-syncs)
- Resource limit policy (memory limits required, no CPU limits)
- DNS convention (use chalupatech.com, not Tailscale hostnames)
- OpenBao/ExternalSecrets pattern (managed_apps → playbook → SecretStore)
- ArgoCD ApplicationSet structure (core-apps vs user-apps, sync waves)
- Ansible playbook tags and when to run them
- Known gotchas (ArgoCD cache, Chart.lock, Helm artifacts)

### 2. Move Skills into Repo

Move from `~/.claude/skills/` to `.claude/skills/` in chalupa-infra:

**Existing:**
- `infra-hotfix` — branch/commit/push/PR workflow
- `infra-onboard-service` — new service deployment checklist

**To create (identified from 2026-04-04 incident session):**
- `resource-tune` — audit pod memory usage vs limits, suggest bumps
- `argocd-debug` — diagnose sync failures, cache issues, stuck operations
- `triage-alert` — check firing alerts, diagnose root cause, suggest fix
- `ansible-run` — safe playbook execution with lint + dry-run

### 3. Audit Stale Tailscale References

Grep for `tailbecff0.ts.net` across all files. Categorize:
- **Broken from pod network** (templates, SecretStores) → fix to chalupatech.com
- **Working but fragile** (CI workflows, Gitea registry) → plan migration
- **Docs only** (ADRs, onboarding guides) → update text

### 4. Clean Up Stale Docs

- Review all ADRs in `docs/adr/` — mark completed ones, update stale references
- Update `docs/schwab-tenant-onboarding.md` (heavy Tailscale references)
- Remove or archive anything that no longer reflects reality

### 5. Ensure .gitignore is Correct

`.claude/skills/` should NOT be gitignored (we want skills committed).
Verify `.claude/settings.local.json` IS gitignored (user-specific).

## Context from 2026-04-04 Incident Session

Tonight's session exposed several gaps:
- Grafana, NATS OOM'd due to insufficient memory limits
- Velero never deployed because: (a) bitnami/kubectl:1.34 doesn't exist, (b) ArgoCD cache stuck, (c) OpenBao role never created, (d) SecretStore used Tailscale hostname
- Multiple services had no memory limits at all (ArgoCD, Longhorn, Traefik)
- 37 orphaned node-debugger pods in default namespace
- go-schwab-auth rollout crashing (separate issue — `/bin/sh` missing in scratch image)

All of these had existing automation (managed_apps, resource limit patterns) that wasn't followed. Skills and docs would have prevented most of them.
