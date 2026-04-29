---
name: adr-008-service-account-framework
type: brief
description: ADR capturing service-account framework decisions (mint-and-capture, platform/apps path split, admin-password drift fix). Target chalupa-infra/docs/adr/ADR-008-service-account-framework.md.
target_repo: chalupa-infra
target_path: docs/adr/ADR-008-service-account-framework.md
created: 2026-04-15
---

# ADR-008: Service Account Framework

**Status:** Accepted
**Date:** 2026-04-15
**Deciders:** ddowell
**Supersedes:** none (new)
**Relates to:** ADR-004 (OpenBao OIDC SSO)

---

## Context

Operator requested a Grafana API token for Claude Desktop (external
read-only query tool) at the end of session 44. Pieces of the "how do
we provision a service account" flow exist across:

- ADR-004 OpenBao OIDC SSO (user identity, not SA)
- `infra-onboard-service` skill (ExternalSecret shape)
- `ansible/roles/openbao_setup/tasks/app_setup.yml` (managed_apps K8s-SA
  pattern)
- `docs/schwab-tenant-onboarding.md` (multi-tenant secret layout)

...but no top-level doc answers "what's the canonical way to create a
**platform** service account, store its token, deliver it to a
consumer, rotate it, revoke it?" Without one, each request becomes a
research expedition and gets hand-rolled slightly differently.

This ADR records the framework decisions. The operational runbook
lives at `docs/service-account-framework.md`.

---

## Decision

### D1. Mint-and-capture via K8s Job (not declarative Helm, not manual UI)

**Grafana has no declarative service-account provisioning.** Grafana's
Helm chart `provisioning/` system covers datasources, dashboards,
plugins, and alerting, but **not** service accounts or tokens. Helm
values cannot mint a token at deploy time. API keys (the legacy
alternative) are deprecated in Grafana 10+.

The viable patterns, evaluated:

| Approach | Declarative? | Reusable? | Complexity | Verdict |
|---|---|---|---|---|
| Helm values / grafana.ini | No (unsupported) | — | — | Rejected |
| Manual UI click + paste into OpenBao | No | No (repeats per SA) | Low | Rejected — every new SA request repeats the manual work |
| grafana-operator v5 (GrafanaServiceAccount CRD) | Yes | Yes | High — new operator, token-readback capability unverified | Deferred — possible future migration, but scope too large for Phase 2 |
| **Mint Job calls Grafana HTTP API, writes token to OpenBao** | Yes (idempotent) | Yes (parameterized by SA name/role/path) | Medium | **Chosen** |

The Mint Job approach:
- Uses the current Helm chart (no new operators).
- Is idempotent — rerunning the Job against an existing SA mints a new
  token only if `ROTATE=true` is set.
- Captures the token in OpenBao at creation time (Grafana does not
  expose token values post-creation; this is the only window).
- Runs as an ArgoCD `PostSync` hook, so tokens exist after each
  observability deploy and ArgoCD owns the lifecycle.

### D2. OpenBao path layout — `platform/` vs `apps/`

Adopt:

```
secret/data/platform/<service>/<credential>    # this doc
secret/data/apps/<app>/<credential>            # app-level, future
secret/data/<legacy>/<credential>              # pre-framework
```

**Rationale:** The existing flat `secret/data/<service>/*` convention
(velero/, observability/, schwab-*/, etc.) is fine for single-tenant
app credentials but has no discriminator between *platform*
infrastructure credentials (shared) and *application* credentials
(namespace-scoped). Introducing `platform/` creates a clean policy
boundary — platform admins get `platform/*`, app namespaces get only
their subtree.

**Migration:** Do **not** migrate existing paths. The convention
applies to new work. Legacy paths are documented as "reference
implementations" in the runbook. Cost of migration (policy rewrites,
ExternalSecret updates, ArgoCD resyncs across several namespaces)
exceeds benefit unless it becomes trivial.

### D3. Admin-password drift fix is a Phase 2 prerequisite

Grafana's current observability Helm values do not pin
`adminPassword`. The subchart auto-generates one and stores it in the
`observability-grafana` k8s Secret at install time, but subsequent OIDC
logins or admin password edits cause drift between the stored Helm
secret and Grafana's SQLite `admin_user` row. Symptom: sidecar reload
failed with 401 (documented in `values.yaml:68-72`).

Any tool authenticating to Grafana via the Helm-stored admin creds —
the Mint Job included — will hit the same 401.

**Fix:** Seed admin password into OpenBao, back via ExternalSecret +
SecretStore, wire `admin.existingSecret` in values.yaml. The subchart
then sets `GF_SECURITY_ADMIN_USER` and `GF_SECURITY_ADMIN_PASSWORD`
env vars, which Grafana uses to reset the SQLite admin hash on every
pod boot → drift cannot persist.

This is carved out as **Phase 2a** (standalone PR) so its blast
radius is scoped (changes Grafana admin login behavior; requires a
pod restart to take effect) and separate from Phase 2b (Mint Job,
depends on 2a).

### D4. In-cluster consumer wiring deferred

The concrete deliverable (`claude-desktop` token) is an **external
consumer** — operator retrieves via `vault kv get` on a laptop. No
ExternalSecret or k8s Secret materialization needed.

When the first in-cluster agent lands, following the runbook adds
SecretStore + ExternalSecret + Reloader annotation in ~10 minutes.
Building that wiring before a consumer exists violates the "don't
build what isn't wired" rule (see brain feedback memory).

### D5. Reloader not deployed in this phase

stakater/reloader is the community-consensus answer for "rotated
secret triggers pod restart," but without an in-cluster SA consumer
today it has no user. Revisit when the first agent lands.

### D6. SQLite persistence enabled (added 2026-04-28, session 126)

D3's "reset admin from env on every boot" is **scoped to admin credentials only**.
The original framework assumed all other Grafana SQLite state could be ephemeral too — that was wrong.
Service accounts, UI-saved dashboards, folders, alerts, annotations, and user prefs all live in SQLite, and none of them have an env-var equivalent that Grafana re-asserts on boot.
Pod restart from any cause (helm upgrade, OOM, node reboot) wiped them silently.

This surfaced when phase-1 (Image Renderer install) bumped the chart, which rolled the Grafana pod, which wiped the `claude-desktop` SA out of SQLite.
The OpenBao token then pointed at a SA-id that no longer existed; mcp-grafana's Bearer auth started 401-ing; auto-redirect to OIDC turned the 401 into HTML; mcp-grafana base64-encoded the HTML as a "PNG"; Anthropic's Vision API rejected it with HTTP 400.

**Fix:** `persistence.enabled: true` on the Grafana subchart, 10Gi on `longhorn-single`, matching the rest of the observability stack.
SQLite now survives pod restarts.
Admin still self-heals from env (D3) — defense in depth in case the PVC is ever recreated.
Deployment strategy switched from RollingUpdate to Recreate because a ReadWriteOnce PVC with `replicas: 1` cannot surge — the new pod would stay Pending on volume attach.

**Why not Grafana Operator with `GrafanaServiceAccount` CRD?**
The operator path heals SA loss via reconciliation rather than preventing it.
Architecturally elegant but adds a whole controller to run for a single-replica Grafana on a homelab.
Reconsider when dashboards-as-code becomes a primary workflow with multiple SAs and many CRD-managed dashboards.

**Why not external Postgres backend?**
Right answer if HA Grafana ever becomes a goal.
Premature for one user on one Pi5.

---

## Consequences

**Positive**
- New service-account requests follow a documented 10-minute flow.
- Tokens are minted declaratively, rotatable via Job rerun, and
  revocable via KV delete + Grafana API delete.
- Admin-password drift (a latent Grafana bug on this cluster) is
  fixed as a side effect.
- Framework is extensible — the same Mint-Job shape works for GitHub
  apps, Discord bots, and other external platforms (swap the API
  calls in the script).

**Negative**
- One more policy + K8s auth role to maintain (`grafana-sa-minter`).
- Bash mint script is stringly-typed and less robust than a small Go
  binary; acceptable for v1 given the low call frequency (one SA per
  request, not continuous).
- Admin-password pinning means Grafana admin password is now
  operator-managed, not subchart-generated — operator must track it
  in OpenBao.
- Legacy `secret/data/<service>/*` paths remain; two conventions
  coexist indefinitely.

**Neutral**
- Grafana-operator migration remains an option. If it later ships SA
  token readback, the Mint Job can be replaced with a CRD. The
  framework docs and OpenBao layout are unaffected.

---

## Alternatives considered

- **grafana-operator v5 now** — rejected for Phase 2 scope. Operator
  deployment, CRD adoption, and unverified token-readback capability
  add more risk than value for a single SA token. Revisit if two or
  more SA requests land before Phase 3.
- **Manual UI click forever** — rejected. Repeatability is the whole
  point of a framework.
- **Static KV secret the operator curates by hand** — rejected for
  the same reason.
- **Dynamic secrets engine for Grafana** — OpenBao does not have a
  native Grafana secrets engine; writing a plugin is out of scope.

---

## Validation

Phase 2a (admin-password fix) validates by:
- `vault kv get secret/platform/grafana/admin-password` returns creds.
- `curl -u admin:$PW https://grafana.chalupatech.com/api/org`
  returns 200 after pod restart.
- OIDC login still works for human users.

Phase 2b (mint Job + claude-desktop token) validates by:
- ArgoCD PostSync Job succeeds.
- `vault kv get secret/platform/grafana/claude-desktop-token` returns
  a token.
- `curl -H "Authorization: Bearer $TOKEN" .../api/datasources`
  returns 200.
- Rotation: rerun with `ROTATE=true` → new token works, old 401s.

---

## References

- `docs/service-account-framework.md` — operational runbook (this ADR's
  companion)
- `docs/adr/ADR-004-openbao-oidc-sso.md` — OpenBao OIDC setup (user
  identity, complementary to SA identity)
- [Grafana Service Accounts HTTP API](https://grafana.com/docs/grafana/latest/developers/http_api/serviceaccount/)
- [Grafana provisioning (service accounts NOT supported)](https://grafana.com/docs/grafana/latest/administration/provisioning/)
- [External Secrets Operator — SecretStore](https://external-secrets.io/latest/api/secretstore/)
- [stakater/reloader](https://github.com/stakater/Reloader) — deferred
