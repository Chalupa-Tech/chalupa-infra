# Skills & Documentation Audit for chalupa-infra

## Goal

Audit the chalupa-infra repo for:
1. Useful skills that would help any user (human or AI) interact with the repo
2. Documentation gaps, stale docs, and inconsistencies
3. A cohesive CLAUDE.md that ties everything together

## Context

Two skills were created during a live incident session (2026-04-04):
- `infra-hotfix` — branch/commit/push/PR workflow for infra changes
- `infra-onboard-service` — full checklist for deploying a new service

These live at `~/.claude/skills/` (user-level). The question is whether they should also be in the repo's `.claude/skills/` for other contributors, and what other skills are needed.

## Skills to Consider

Based on tonight's incident response, these workflows were performed manually and could be skills:

| Workflow | Frequency | Candidate Skill |
|----------|-----------|-----------------|
| Bump resource limits for OOM | Monthly | `resource-tune` — check usage vs limits, propose changes |
| Diagnose ArgoCD sync issues | When stuck | `argocd-debug` — check sync status, cache, operation state |
| Run Ansible playbooks safely | Weekly | `ansible-run` — lint, dry-run, target selection |
| Migrate Tailscale → chalupatech.com refs | One-time but pattern repeats | `dns-migrate` |
| Triage alerts (Discord/VMAlert) | Daily | `triage-alert` — check firing alerts, diagnose, suggest fix |
| Velero backup verification | Should be periodic | `backup-check` |

## Documentation Audit Checklist

1. **CLAUDE.md** — Does it exist? Is it current? Does it cover:
   - Repo layout and conventions
   - How to make changes (branch from main, PR process)
   - ArgoCD sync behavior and gotchas
   - OpenBao/ExternalSecrets onboarding pattern
   - Ansible playbook inventory and tags
   - Resource limit policy (no CPU limits)
   - DNS conventions (chalupatech.com, not Tailscale)

2. **ADRs** — Are they up to date? Key ones to check:
   - ADR-004 (OIDC) — still references Tailscale URLs
   - ADR-007 (CNI migration) — is this complete? What's the status?
   - Any missing ADRs for recent decisions?

3. **Stale references** — grep for `tailbecff0.ts.net` across all docs, templates, CI workflows. Catalog which ones are:
   - Actively broken (pod-network DNS failures)
   - Working but fragile (CI/build-time only)
   - Documentation-only (informational, should still update)

4. **schwab-tenant-onboarding.md** — references Tailscale URLs extensively

## Deliverables

1. Updated/created CLAUDE.md for chalupa-infra
2. Skills defined and placed in `.claude/skills/` within the repo
3. List of stale Tailscale references with fix plan (PR or follow-up prompt)
4. Updated ADRs where content is factually wrong
