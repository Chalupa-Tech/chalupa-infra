---
date: 2026-05-08
session: 130
phase: platform/phase-9-openbao-client-load-balanced-service
---

# OpenBao client connection: load-balanced vs leader-only service

Quick research backing the phase-9 decision to switch every cluster-internal OpenBao client from `openbao-active` to the load-balanced `openbao` service.
Goal here is to verify the load-balanced service really does narrow the failure window for the four scenarios that matter, and to surface any surprises *before* flipping seven config sites.

## TL;DR

- **Leader-transition failures (the dominant case): switching helps.** The load-balanced service routes the request to a standby, which transparently forwards to the new leader once election completes. The Go client follows one redirect hop on 307. Expected user-visible degradation is ~1–4s of retry, not the full election window.
- **Quorum-loss-with-sealed-pods (the session-130 cascade): switching helps less than you'd hope.** The load-balanced service still presents zero endpoints if all server pods are sealed, because the chart's default readiness probe (`bao status`) fails for sealed pods (exit code 2). With only one unsealed standby surviving, clients get `503` with a structured "local node not active" error instead of `connection refused` — better signal, similar functional outage. **Phase-8 (auto-unseal) is the actual fix for this case.**
- **No worse than today.** In every scenario, the load-balanced service is strictly equal or better than `openbao-active`. There is no failure mode where this change regresses behavior.

## Claim 1 — Vault/OpenBao Go client follows 307 redirects by default

**Verified.** OpenBao's API client is a fork of Vault's; the redirect logic in `api/client.go` is identical:

- `NewConfig()` sets `HttpClient.CheckRedirect = func(...) error { return http.ErrUseLastResponse }`, suppressing Go stdlib's default redirect-following.
- The client's own `RawRequestWithContext` then handles 301/302/307 manually, allowing exactly **one** redirect hop (gated by `redirectCount == 0`).
- Default `MaxRetries: 2` (3 total attempts) on 5xx, with `MinRetryWait: 1000ms` and `MaxRetryWait: 1500ms`.
- `DefaultRetryPolicy` retries on 5xx and 412 (the latter for `X-Vault-Index` consistency).

**Implication for us:** when a Schwab service hits the load-balanced `openbao` service and lands on a standby, the standby's 307 (or transparent forward, in default config) is followed automatically.
The "redirect storm" risk only materializes if all nodes share one `api_addr` behind a load balancer — which is *not* our config (we use the per-pod headless `openbao-internal` for `api_addr`).

Sources:
- [vault/api/client.go](https://github.com/hashicorp/vault/blob/main/api/client.go)
- [vault Go client godoc](https://pkg.go.dev/github.com/hashicorp/vault/api)

## Claim 2 — Sealed pod returns structured 503, not connection failure

**Verified.** OpenBao returns:
- `/sys/health` → `503` for sealed, `429` for standby, `200` for active.
- Regular API calls against a sealed node → structured 503 with `{"errors":["Vault is sealed"]}` (Vault's well-documented sealed-node response, inherited by OpenBao).

**But this only matters if the sealed pod is reachable.** Our chart uses the upstream `openbao-helm` default readiness probe — `bao status -tls-skip-verify`, which exits 2 for sealed pods. K8s endpoint controller drops `NotReady` pods from Service endpoints. So in normal operation, sealed pods are *not* in the `openbao` Service's endpoint list at all.

**The interaction that bit us in session-130:** the `openbao-active` service has selector `openbao-active: "true"`. The `service_registration "kubernetes"` plugin sets that label per-pod based on leader status. When the surviving pod is unsealed but cannot claim leadership (no quorum), the plugin drops the label everywhere → `openbao-active` has zero endpoints → `connection refused`.

The load-balanced `openbao` service uses selector `app.kubernetes.io/name=openbao, component=server` — readiness-filtered, but no `openbao-active` label requirement. So the surviving unsealed standby remains an endpoint, returning structured 503s instead of connection failure.

Sources:
- [openbao/concepts/seal](https://openbao.org/docs/concepts/seal/)
- [openbao-helm server-statefulset.yaml readinessProbe](https://github.com/openbao/openbao-helm/blob/main/charts/openbao/templates/server-statefulset.yaml)
- [openbao-helm server-ha-active-service.yaml selector](https://github.com/openbao/openbao-helm/blob/main/charts/openbao/templates/server-ha-active-service.yaml)

## Claim 3 — ESO retry behavior on 5xx / 307

**Partially verified.** Our `SecretStore` has no `retrySettings` configured, so the only retry layer is whatever the underlying Vault Go client provides:

- 307 → followed once by the Go client (Claim 1).
- 5xx → retried 3 times with 1–1.5s waits (Claim 1).
- After Go-client retries exhaust, ESO's reconcile loop bubbles the error and the `ExternalSecret` enters a failed state until the next `refreshInterval` (default `1h` if unset).

**For our cluster:** none of our `ExternalSecret` resources set `refreshInterval`, which means default-1h retries on permanent failure. **Mid-sync transient failures during a leader election should be invisible to consumers**, because pod startup blocks on a successful sync (managed Secret either exists or doesn't).

**Caveat:** this analysis assumes the ESO controller pod itself isn't restarted mid-outage. ESO handles restart by re-syncing every `ExternalSecret`; if Vault is fully unreachable during that window, every workload that depends on a managed Secret could face delayed pod startup. Phase-9 doesn't change this; phase-8 does.

Sources:
- [external-secrets/external-secrets vault provider](https://github.com/external-secrets/external-secrets/blob/main/docs/provider/hashicorp-vault.md)
- [external-secrets API spec](https://external-secrets.io/latest/api/spec/)

## Claim 4 — Standby behavior under quorum loss

**Verified empirically (session 130) and consistent with docs.** When raft has lost quorum:
- No pod has `vault-active: "true"` (the registration plugin clears it).
- A standby that receives a write request cannot forward (no leader to forward to) and cannot redirect (no leader address). It returns 503 with an error like `"local node not active in cluster"`.
- A standby that receives a read request behaves the same way for tokenized reads (which require leader-mediated lease validation in our config).

**Cross-checked against:** [openbao/concepts/ha](https://openbao.org/docs/concepts/ha/) — confirms standbys forward to active by default, fall back to 307 redirect; doesn't explicitly cover the no-leader case, but the empirical session-130 behavior is consistent ("no leader → no forward target → 503").

## Surprise / finding to flag

**The load-balanced service does not eliminate the session-130 failure mode.** It converts:

| Scenario | `openbao-active` (today) | `openbao` (after phase-9) |
|---|---|---|
| Leader transition (all pods unsealed) | hard fail until election completes | ~1–4s retry, then transparent recovery |
| 1 of 3 sealed | hard fail if sealed pod was leader | transparent — readiness drops sealed pod |
| 2 of 3 sealed (session-130 root cause) | `connection refused` (zero endpoints) | structured 503 from surviving standby |
| 3 of 3 sealed | `connection refused` | `connection refused` (no Ready endpoints) |

For rows 3 and 4, **clients still hard-fail**; they just get a different error shape.
**Phase-8 (auto-unseal) is what eliminates the 2-of-3 and 3-of-3 cases** by preventing the seal cascade in the first place.

This is consistent with the phase-9 prompt's explicit caveat ("does not eliminate the seal cascade — phase-8 does that"); calling it out here so future-us doesn't misremember phase-9 as a complete fix on its own.

## Conclusion

Proceed with the switch. Every scenario is equal-or-better with the load-balanced service.
The change is also load-bearing for phase-8: once auto-unseal is in place, leader transitions during planned maintenance become routine — and `openbao-active` would otherwise force a brief outage on every transition.

## What changes (final scope)

| File | Change |
|---|---|
| `k8s/apps/schwab/go-schwab-auth/values.yaml` | `openbao-active` → `openbao` (PR-9a) |
| `k8s/apps/schwab/go-schwab-feed/values.yaml` | `openbao-active` → `openbao` (PR-9a) |
| `k8s/apps/schwab/go-notify/values.yaml` | `openbao-active` → `openbao` (PR-9a) |
| `k8s/apps/schwab/base/values.yaml` | `openbao-active` → `openbao` (PR-9a) |
| `k8s/platform/observability/templates/secret-store.yaml` | `openbao-active` → `openbao` (PR-9b) |
| `k8s/platform/observability/values.yaml` | OIDC `token_url` + `api_url` → `openbao` (PR-9b) |
| `k8s/platform/observability/templates/grafana-sa-mint-script-cm.yaml` | `BAO_ADDR` default → `openbao` (PR-9b) |
| `k8s/platform/observability/templates/grafana-admin-secret.yaml` | ExternalSecret server → `openbao` (PR-9b) |
| `k8s/platform/observability/templates/gh-actions-telemetry-externalsecret.yaml` | ExternalSecret server → `openbao` (PR-9b) |
| `k8s/platform/observability/templates/gh-actions-telemetry-mint-script-cm.yaml` | `BAO_ADDR` default → `openbao` (PR-9b) |

**Not changed:** `k8s/platform/openbao/templates/ingressroute.yaml` references `name: openbao-active` as a Traefik route target — that's the upstream-defined Service name, not a client URL, and the prompt's Notes section explicitly says to leave the `openbao-active` Service in place for niche cases (monitoring, integrations bypassing redirects).
