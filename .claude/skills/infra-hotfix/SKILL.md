---
name: infra-hotfix
description: Create a branch, commit, push, and PR for chalupa-infra fixes. Use when making config changes, bumping resource limits, fixing Helm values, or any infrastructure change that needs a PR. Handles the full branch-from-main → commit → push → PR workflow with consistent formatting.
tools: Read, Edit, Write, Bash, Glob, Grep
---

# Infrastructure Hotfix Skill

Create PRs for chalupa-infra changes with consistent workflow and formatting.

## Workflow

1. **Stash any WIP** on the current branch
2. **Checkout main** and pull latest
3. **Create a feature branch** from main (naming: `fix/` for fixes, `feat/` for features)
4. **Stage and commit** the changes with a conventional commit message
5. **Push** the branch
6. **Create a PR** using the standard template below

## Branch Naming

- Fixes: `fix/<short-description>` (e.g., `fix/grafana-oom-memory-limit`)
- Features: `feat/<short-description>`
- Deploy: `deploy/<app-name>-<version>`

## PR Template

```
## Summary
<1-3 bullet points describing what changed and why>

## Deployment notes
<How this deploys — ArgoCD auto-sync, Ansible playbook required, manual steps, etc.>

## Risk
<Low/Medium/High — what could go wrong>

## Test plan
- [ ] <Verification steps as checklist>
```

## Rules

- ALWAYS branch from `main` — ArgoCD self-heals, so changes on other branches won't deploy
- NEVER commit Helm chart tarballs (charts/*.tgz) — only Chart.lock
- NEVER run `helm dependency build` in the working tree unless cleaning up after
- NO "Generated with Claude Code" tags on commits or PRs
- NO CPU limits (cluster-wide policy per #273) unless specifically requested
- Keep commit messages concise — focus on "why" not "what"
