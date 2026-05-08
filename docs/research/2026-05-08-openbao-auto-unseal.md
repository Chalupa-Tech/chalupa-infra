---
date: 2026-05-08
author: ddowell
session: 131
phase: platform/phase-8-openbao-auto-unseal
status: decided — Option A (in-cluster transit), split into phase-8a + phase-8b
---

# OpenBao auto-unseal — design research

## Problem statement

The OpenBao Helm chart at `k8s/platform/openbao/values.yaml` ships with default Shamir seal and **no seal stanza**.
Every pod restart leaves a follower sealed until a human runs `ansible-playbook playbooks/openbao-unseal.yml`.
Session 129 (2026-05-07) found 2 of 3 pods sealed for 33–39 days.
The cluster has been one `openbao-0` restart away from total auth outage for a month.

Phase-9 (just landed) narrows the *leader-transition* failure window by switching cluster-internal clients to the load-balanced `openbao` Service (was leader-only `openbao-active`).
Phase-8 must eliminate the *seal cascade itself*: follower restarts must not require human intervention.

## Constraints (operator + cluster)

- **No-cloud ADR posture**: AWS/Azure/GCP KMS off the table.
- **No HSM**: PKCS#11 off the table.
- **Hardware**: 6× Raspberry Pi 5 (8GB) k3s cluster + 1 docker host (docker1) + 1 framework node (oracle1). All on Tailscale mesh.
- **Current OpenBao version**: 2.3.1 (chart `openbao-helm` 0.4.0).
- **Current Shamir threshold**: 1-of-1 (single key in `vault_openbao_unseal_key`, ansible vault).
- **StatefulSet update strategy**: chart default `OnDelete` is correct for OpenBao production (verified against [openbao-helm values.yaml](https://github.com/openbao/openbao-helm/blob/main/charts/openbao/values.yaml)) — no change needed.

## R1 — Seal source candidates

Four live options for OpenBao 2.3.1 in this environment.

| # | Option | Self-hosted? | Net deployment surface | Cyclic dependency? | Eliminates manual unseal? |
|---|---|---|---|---|---|
| A | Transit (separate OpenBao in same k3s cluster) | yes | new chart + ApplicationSet + ESO secret | yes (transit instance Shamir) | yes (after one bootstrap) |
| B | Transit (separate OpenBao via systemd on docker1 / oracle1) | yes | ansible role + systemd unit | yes (transit instance Shamir) | yes (after one bootstrap) |
| C | K8s automation of Shamir (CronJob/operator pattern) | yes | one CronJob + Secret + RBAC | no | yes (operationally) |
| D | Static auto-unseal (key from file/env) | yes | seal stanza + Secret mount | no | yes — built into OpenBao |

**Off the table** for this cluster: cloud KMS (no-cloud ADR), PKCS#11/HSM (no hardware), KMIP (would need a new KMIP server — no benefit over transit).
Bank-vaults operator: per [bank-vaults discussion #2594](https://github.com/orgs/bank-vaults/discussions/2594) (last activity May 2026), no working OpenBao operator exists; project planning Bank-Vaults CLI/secret-init/secrets-webhook OpenBao support but not the operator.

### Option A — Transit, in-cluster

Spin up a separate OpenBao Helm release (`openbao-transit` namespace), single replica, pinned to a stable node via `nodeSelector`.
This instance holds the transit key.
Production OpenBao seal stanza points at it.

**Pros:**
- Proper transit-seal pattern; matches HashiCorp Vault production guidance.
- Migration to fully-external transit (Option B) later is mostly a Helm-values flip.
- Production cluster auto-unseals across restarts.

**Cons:**
- Same cluster failure domain as production. Whole-cluster reboot (e.g., power outage) still requires manual unseal *of the transit instance*, then production auto-unseals from there. Net: 2-step recovery instead of 3-step.
- New ArgoCD application, new ExternalSecret pattern (token for production → transit), new namespace.
- StatefulSet of size 1 with stable node pin = no HA. OK because transit instance restart is a manual intervention regardless.

### Option B — Transit, out-of-cluster (systemd)

Same shape as A but transit OpenBao runs as a systemd service on docker1 or oracle1.

**Pros:**
- Independent failure domain from k3s. K3s control-plane incidents do not seal the transit source.
- Strongest isolation per [stderr.at OpenBao Part 6](https://blog.stderr.at/openshift-platform/security/secrets-management/openbao/2026-03-05-openbao-part-6-auto-unsealing/) recommendation: "the root of trust for unsealing lives outside the production cluster."

**Cons:**
- New deployment surface: ansible role for the systemd unit, separate binary lifecycle, new TLS surface.
- docker1 already runs Gitea; oracle1 is a framework node — neither is "spare."
- More operator work to introduce; harder to undo than A.

### Option C — K8s automation of Shamir

Keep current single-key Shamir.
Add a CronJob (or Job + reconciler) that runs every minute:
1. Lists OpenBao pods.
2. For each pod, runs `bao status -format=json`. If `sealed=true`, runs `bao operator unseal $KEY`.

The unseal key flows: ansible vault → k8s Secret (created by an Ansible task during bootstrap) → mounted into the unsealer Job.

**Pros:**
- Smallest possible change. No new OpenBao instance. No cyclic dependency.
- Reuses existing key + ansible-vault SoT.
- Unsealer can be deleted instantly to revert; failure mode reverts to current manual playbook.
- Compatible with future migration to A/B/D — does not lock in.

**Cons:**
- Unseal key now exists in *two* places: ansible vault (SoT) + k8s Secret (operational copy). Anyone with `secrets/get` in the unsealer namespace gets the key.
  - Mitigation: tight RBAC, dedicated namespace, audit logs, operator-only ServiceAccount.
- Not "auto-unseal" in OpenBao's sense — recovery keys / migration semantics don't apply. If we later migrate to a real seal type, the migration is the standard Shamir → seal flow.
- The CronJob is one more moving part to monitor; if it stops running, we silently regress to manual. Mitigation: alert on `kube_cronjob_status_last_schedule_time` lag.

### Option D — Static auto-unseal (requires chart bump)

OpenBao 2.5.0 (released Feb 2026) introduced [static auto-unseal](https://openbao.org/docs/rfcs/static-auto-unseal/): seal stanza loads a key from a file or env var.

Stanza syntax (verified):
```hcl
seal "static" {
  current_key    = "file:///vault/secrets/current-unseal-key"
  current_key_id = "0c94ff77-42b6-4de2-b27a-8f7c166fb162"
  previous_key    = "file:///vault/secrets/last-unseal-key"
  previous_key_id = "33c9886e-7c93-4252-bf7c-8b15f4fd937c"
}
```

**Pros:**
- Cleanest possible end state. OpenBao itself loads the key at startup; no external orchestration.
- No cyclic dependency.
- Built-in `previous_key` mechanism for rotation.

**Cons:**
- Requires OpenBao **2.5.x** chart bump from current 2.3.1. That's a separate production change with its own validation surface.
- The OpenBao team explicitly notes this is "not the recommended pattern for production" and requires careful threat modeling — the key sits in a file readable by the OpenBao process.
- Same security posture as Option C (key in cluster), just expressed via OpenBao native config.

## R2 — Transit cyclic dependency (Options A and B)

If we pick A or B, the transit instance itself must be Shamir-sealed.
On every transit-instance restart, an operator must manually unseal it.
Mitigations:

- **Stable placement**: pin to a node that rarely reboots (docker1 for B, a designated control plane for A). NodeSelector + DaemonSet OnDelete.
- **No auto-restarts**: minimize Helm/ArgoCD churn on the transit instance — flag it `argocd.argoproj.io/sync-options: SkipDryRunOnMissingResource=true` and `Prune=false` to discourage accidental rolls.
- **Document the transit-unseal procedure** as a 1-shot ansible playbook (`playbooks/openbao-transit-unseal.yml`) — same shape as current `openbao-unseal.yml` but scoped to the transit instance.
- **Backup**: transit instance's raft data must be backed up regularly. Loss of transit raft + loss of Shamir keys = unrecoverable production cluster (the root key is encrypted only by the transit key).

The cyclic dependency is **acceptable** because transit instance restarts are rare events (its workload is one transit key + a token check) and recovery is a known short procedure.
But it does NOT eliminate manual unseal entirely — it shifts it from "every production pod restart" to "every transit instance restart."

For Option B, the systemd-on-docker1 placement gives the strongest "transit instance never restarts unless docker1 reboots" guarantee.

## R3 — Community sweep (claims + sources)

| Claim | Source | Date | Verdict |
|---|---|---|---|
| Transit-seal stanza syntax for OpenBao 2.3.x is identical to Vault | [openbao.org/docs/configuration/seal/transit](https://openbao.org/docs/configuration/seal/transit/) | docs cover 2.3.x–2.5.x | Verified |
| Static auto-unseal is 2.5.x-only | [openbao.org/docs/rfcs/static-auto-unseal](https://openbao.org/docs/rfcs/static-auto-unseal/) | RFC, in 2.5.0 | Verified |
| StatefulSet should use `OnDelete` update strategy | [openbao-helm values.yaml](https://github.com/openbao/openbao-helm/blob/main/charts/openbao/values.yaml) | chart default | Verified — already chart default; current values.yaml inherits |
| Bank-vaults has no OpenBao operator path today | [bank-vaults discussion #2594](https://github.com/orgs/bank-vaults/discussions/2594) | last activity May 2026 | Verified — Option C cannot leverage bank-vaults; we'd write our own CronJob |
| Auto-unseal mechanism loss = unrecoverable cluster | [openbao.org/docs/concepts/seal](https://openbao.org/docs/concepts/seal/) | docs | Verified — recovery keys only authorize, do not decrypt root key |
| Metric prefix is `vault.` not `bao.` | [openbao.org/docs/internals/telemetry/metrics/core-system](https://openbao.org/docs/internals/telemetry/metrics/core-system/) | docs | **Verified — phase-8 prompt's `bao_core_unsealed` is wrong; correct is `vault_core_unsealed`** |
| `vault.core.unsealed == 1` means unsealed; `== 0` means sealed | same | docs | Verified |
| `vault.core.active == 1` means active leader; `== 0` means standby | same | docs | Verified |

Per `feedback_research_before_infra` (memory): no single source taken at face value; vmrule expression below uses verified prefix.

## R4 — Key rotation + backup

### Option C (Shamir-automation)

- **Rotation**: same as today. Re-init OpenBao with new Shamir threshold + key. Old key invalidated. This is a planned-outage operation regardless of automation.
- **Backup**: same as today. Ansible vault on operator's laptop is SoT. Add `vault_openbao_unseal_key` to whatever backup procedure already covers `ansible/inventory/group_vars/all/vault.yml`.
- **Operational copy**: the k8s Secret derived from the ansible-vault key is regenerated on every `ansible-playbook site.yml` run. It is not a separate SoT.

### Options A/B (transit)

- **Rotation**: rotate the transit key via the transit secret engine API on the transit instance: `bao write -f transit/keys/openbao-cluster-unseal/rotate`. Old key versions retained for decrypting legacy data — production OpenBao keeps using the new version automatically.
- **Cadence**: annual is conservative; semi-annual if compliance requires.
- **What breaks during rotation**: nothing transient — the transit secret engine handles version bumping transparently. Pods do NOT need to restart.
- **Backup**: transit instance's raft snapshot must be backed up. AND the Shamir keys for the transit instance itself must be in ansible vault. AND the production cluster's recovery keys must be safely stored (recovery keys ≠ unseal keys; required only for recovery rekey scenarios).

### Option D (static)

- **Rotation**: write new key to `current_key` file path; OpenBao loads new key on next restart (or via SIGHUP if supported). Old key remains in `previous_key` to decrypt legacy root-key wrap.
- **Backup**: same as Option C — key SoT in ansible vault, k8s Secret is the operational copy.

## Recommendation

**Decision (operator, 2026-05-08): Option A — in-cluster transit OpenBao, split into phase-8a (introduce transit) + phase-8b (cutover + observability + runbook).**

Reasoning:
- **Proper engineering pattern.** Transit is the documented OpenBao production unseal mechanism; recovery semantics, key rotation, and incident classes map onto OpenBao's documented operational model rather than bespoke "the CronJob fell behind" modes.
- **Per `feedback_ship_direct_to_end_state` memory:** when the operator has conviction about end state, build direct; no intermediate shape that gets inverted later.
- **Per `project_platform_llm_consumption` memory:** the platform should look like a real platform — Option C's hand-rolled CronJob doesn't.
- **Cyclic-dependency concern is manageable.** Transit instance is Shamir-sealed; restart requires manual unseal via a 1-shot ansible playbook scoped to one pod. Whole-cluster reboots are rare.
- **In-cluster (A) over out-of-cluster (B):** minimizes new deployment surface (reuse existing chart wrapper / ApplicationSet / ESO patterns); B remains a viable future migration if k3s incidents start affecting transit availability.

### Why not Option C (Shamir-automation)

Considered and rejected after operator pushback.
C does not actually eliminate manual-unseal scenarios — it only shifts them.
If the unsealer CronJob breaks or its Secret is corrupted, the cluster regresses to manual-unseal.
Transit (A) shifts the same failure surface to a single, named, observable component (the transit instance), with documented recovery procedures.

### Why not Option D (static auto-unseal)

Requires OpenBao 2.3.1 → 2.5.x chart bump.
Bumping the chart for the sole purpose of unlocking static auto-unseal isn't worth the validation surface; if a chart bump happens for other reasons later, Option D becomes the cleanest end state and the transit instance from 8a/8b can be retired.

## Reconciliation note: phase-9c overlap

Phase-8 action item 4 ("New runbook: `docs/runbooks/openbao-incident.md`") **overlaps with phase-9c** (`workstreams/platform/prompts/phase-9c-openbao-incident-runbook.md`), which independently plans to create the same file.

Per `feedback_reconcile_duplicate_phases` (memory): reconcile before executing.

Two clean splits:

- **Split-A**: Phase-9c writes `docs/runbooks/openbao-incident.md` covering current Shamir-seal world (Incidents A/B/C as written in 9c). Phase-8 *updates* the runbook to add new incident classes (transit-source-down, key-rotation-failure) once auto-unseal is live.
- **Split-B**: Phase-8 writes the runbook covering both worlds in a single pass. Phase-9c is dropped as redundant.

Split-A is cleaner if phase-9c lands first (it's `depends_on: []` and runnable now per its prompt).
Split-B is cleaner if phase-8 lands first.

Recommend **Split-A** — phase-9c is small, independent, and the runbook is more useful in-tree before phase-8 ships than after.
This phase's action item 4 then becomes "extend `docs/runbooks/openbao-incident.md` with auto-unseal-specific incidents."

## Phase split (decided)

**Phase-8a — introduce transit OpenBao instance + production telemetry** (`prompts/phase-8a-openbao-transit-instance.md`)

Purely additive.
Production cluster stays on Shamir.
Scope:
1. New chart wrapper at `k8s/platform/openbao-transit/` — single-replica StatefulSet, node-pinned, Shamir-sealed, transit secrets engine enabled, scoped token created.
2. ApplicationSet entry in `core-apps-appset.yaml`.
3. Ansible playbook `playbooks/openbao-transit-bootstrap.yml` — init + enable transit + create unseal key + create scoped token + write token to a static k8s Secret in the openbao namespace.
4. Enable Prometheus telemetry on production OpenBao chart (`telemetry { prometheus_retention_time = "30s"; disable_hostname = true }` in config + listener block).
5. Add ServiceMonitor / VMServiceScrape targeting `openbao` service on `/v1/sys/metrics?format=prometheus`.
6. Add `OpenBaoSealed` + `OpenBaoStandbyMissing` vmrules (using verified `vault_core_unsealed` / `vault_core_active` metrics).
7. Validate: transit healthy; encrypt/decrypt round-trip via scoped token works; production metrics scraping; alerts evaluating (if currently sealed, `OpenBaoSealed` will fire — that's correct behavior).

**Phase-8b — cutover production seal Shamir → transit + runbook** (`prompts/phase-8b-openbao-transit-cutover.md`)

Safety-critical.
Operator on standby.
Scope:
1. Add `seal "transit" {…}` stanza to `k8s/platform/openbao/values.yaml` referencing transit instance + scoped token from k8s Secret.
2. Run seal migration per pod: `bao operator unseal -migrate <shamir-key>`.
3. Validate via deliberate restart of each pod (openbao-2 → openbao-1 → openbao-0); each must come up unsealed without intervention.
4. Extend `docs/runbooks/openbao-incident.md` (assumes phase-9c lands first) with auto-unseal-specific incidents: transit-source-down, key rotation, unseal migration rollback.
5. Update memory `reference_openbao_auto_unseal_design` with the chosen architecture + key locations + rotation procedure.

Rollback for 8b: remove seal stanza, run existing `playbooks/openbao-unseal.yml` to manually unseal with original Shamir key.

## Sequencing

1. **Phase-9c** (runbook for current Shamir world) — should land first; independent and small.
2. **Phase-8a** (transit instance + telemetry) — additive, no production seal change.
3. **Bake** — ≥24h with transit instance healthy, observe metrics + alerts behavior.
4. **Phase-8b** (cutover) — operator on standby.

8a and 9c are independent and can run in either order or parallel sessions.

## Resolved questions

- **Seal source**: Option A (in-cluster transit). [decided]
- **Phase-9c reconciliation**: Split-A — phase-9c writes runbook for Shamir world; phase-8b extends it for auto-unseal world. [decided]
- **Metric prefix**: `vault_core_unsealed`, `vault_core_active`. [verified against OpenBao docs]

## Open questions for phase-8a execution

1. **Transit instance node pin**: which node? Recommend a control plane (tpi2/dpi2/dpi3) for stability. Operator's call before 8a starts.
2. **Transit instance backup**: raft snapshot cron, or rely on regenerating from ansible-vault keys? Decide during 8a; document in research-doc addendum.
3. **Transit token mount path**: file (Secret-to-volume) or env var? Recommend file mount for clearer audit; operator's call.

## Sources

- [openbao.org/docs/configuration/seal/transit/](https://openbao.org/docs/configuration/seal/transit/) — transit seal config reference
- [openbao.org/docs/rfcs/static-auto-unseal/](https://openbao.org/docs/rfcs/static-auto-unseal/) — static auto-unseal RFC + 2.5.x availability
- [openbao.org/docs/concepts/seal/](https://openbao.org/docs/concepts/seal/) — recovery-key vs unseal-key semantics; SPOF risk
- [openbao.org/docs/internals/telemetry/metrics/core-system/](https://openbao.org/docs/internals/telemetry/metrics/core-system/) — `vault.core.unsealed` / `vault.core.active` metric definitions
- [openbao.org/docs/release-notes/2-3-0/](https://openbao.org/docs/release-notes/2-3-0/) — KMIP auto-unseal added in 2.3.x; static not present
- [openbao-helm values.yaml](https://github.com/openbao/openbao-helm/blob/main/charts/openbao/values.yaml) — `OnDelete` updateStrategy default; transit/static config patterns
- [bank-vaults discussion #2594](https://github.com/orgs/bank-vaults/discussions/2594) — no OpenBao operator on roadmap (last activity May 2026)
- [stderr.at OpenBao Part 6](https://blog.stderr.at/openshift-platform/security/secrets-management/openbao/2026-03-05-openbao-part-6-auto-unsealing/) — production transit pattern recommending out-of-cluster placement
- [iximiuz Labs: OpenBao/Vault Auto-Unseal with Transit](https://labs.iximiuz.com/tutorials/openbao-vault-auto-unseal-transit-82d2a212) — worked example of transit deployment
