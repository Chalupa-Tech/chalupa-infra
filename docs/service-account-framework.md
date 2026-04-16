---
name: service-account-framework
type: brief
description: Cross-repo framework for provisioning platform service accounts (Grafana, GitHub, etc.) via OpenBao + ExternalSecrets. Target: chalupa-infra/docs/service-account-framework.md.
target_repo: chalupa-infra
target_path: docs/service-account-framework.md
created: 2026-04-15
---

# Service Account Framework

**Status:** Active
**Last updated:** 2026-04-15
**Applies to:** chalupa-infra platform and apps running on the K3s cluster

> **Brain note:** This brief is the source of truth. Phase-2a and
> phase-2b prompts deliver it into `chalupa-infra/docs/` as part of
> their PRs. Edit it here, not there.

---

## When to use this doc

Read this when the request is "I need a service account for X" — where X
is an external system (Grafana, GitHub, Discord, etc.) that manages its
own identity and issues bearer tokens. Goal: a repeatable path from
"operator asks" to "credential minted, stored, delivered, rotatable,
revocable" in under 30 minutes.

If the request is about a **Kubernetes** ServiceAccount (pod → k8s API,
or pod → OpenBao via K8s auth), this doc is not the right reference —
see `ansible/roles/openbao_setup/tasks/app_setup.yml` for the
`managed_apps` pattern.

---

## 1. Identity taxonomy

Three distinct identity planes. Never conflate them.

| Identity | Purpose | Example |
|---|---|---|
| **K8s ServiceAccount** (+ projected token) | Pod identity inside the cluster. Exchanged for a Vault token via OpenBao K8s auth. | `default` SA in `observability` → `observability-role` |
| **Platform service account** | Account on an external system (Grafana SA, GitHub app, Discord bot). External system owns the identity; we store the opaque token. | `claude-desktop` Grafana SA, Viewer role |
| **OpenBao AppRole** | Non-K8s automation identity (Ansible, bootstrap jobs, CI outside cluster). | Currently unused; reserved for future CI agents |

This doc is about **platform service accounts**.

---

## 2. Canonical flow

```
┌──────────────┐   mint      ┌─────────────┐   store   ┌──────────┐
│ External     │◀───────────▶│  Mint Job   │──────────▶│ OpenBao  │
│ system       │   HTTP API  │  (K8s Job,  │   KV v2   │ platform/│
│ (Grafana,    │             │   PostSync) │           │ <svc>/   │
│  GitHub, …)  │             └─────────────┘           └────┬─────┘
└──────────────┘                                            │
                                                            │ sync
                                                            ▼
                                                      ┌──────────┐
                                                      │ External │
                                                      │ Secret   │
                                                      │ (only if │
                                                      │ in-cluster│
                                                      │ consumer)│
                                                      └────┬─────┘
                                                           │
                                                           ▼
                                                     k8s Secret → Pod
```

Each arrow is a separate identity boundary. Mint Job auths to Grafana
with admin creds (separate secret). Mint Job auths to OpenBao via K8s
auth. ExternalSecret auths to OpenBao via K8s auth of the consuming
namespace. Never reuse an identity across planes.

---

## 3. Naming conventions

### OpenBao paths

```
secret/data/platform/<service>/<credential>    # platform-level (this doc)
secret/data/apps/<app>/<credential>            # application-level (future)
secret/data/<legacy-service>/<credential>      # pre-framework; leave in place
```

- **Platform-level:** shared infrastructure credentials (Grafana tokens,
  GitHub apps, Discord bots, admin passwords). Policy scope belongs to
  platform admins.
- **Application-level:** per-app credentials scoped to an app's namespace.
- **Legacy:** `secret/data/velero/*`, `secret/data/observability/*`,
  `secret/data/schwab-*/*`. **Do not migrate** unless trivial — the new
  convention applies to new work only.

### Kubernetes objects

| Object | Name pattern | Example |
|---|---|---|
| `SecretStore` | `<purpose>-openbao` (default: `vault-backend`) | `grafana-admin-store` |
| `ExternalSecret` | `<service>-<credential>` | `grafana-admin-secret` |
| Minted k8s Secret | matches ExternalSecret name | `grafana-admin-secret` |
| Mint `Job` | `<service>-sa-mint-<purpose>` | `grafana-sa-mint-claude-desktop` |

### External-system SA names

Name after the consumer, not the scope. `claude-desktop`, `grafana-ci`,
`github-renovate` — not `grafana-viewer-1`.

---

## 4. Consumer shapes

### Shape A — External consumer (desktop app, personal script, off-cluster tool)

Token lives only in OpenBao. No ExternalSecret, no k8s Secret. Operator
retrieves on demand:

```bash
vault kv get -field=token secret/platform/grafana/claude-desktop-token
```

Use when the consumer is a laptop, CI running outside the cluster, or a
one-off debugging tool.

### Shape B — In-cluster consumer (workload)

Four steps, performed *when the consumer exists*:

1. **SecretStore** in the consumer's namespace, if one doesn't already
   have read access to the path.
2. **ExternalSecret** referencing `secret/platform/<service>/<credential>`,
   `refreshInterval: 1h`.
3. **Mount** the k8s Secret into Deployment/Job as env var or volume.
4. **Reloader annotation** (`reloader.stakater.com/auto: "true"`) if the
   consumer can't re-read rotated creds without a restart. **Reloader
   is not yet deployed** — revisit when the first in-cluster SA consumer
   lands.

Do **not** pre-wire ExternalSecrets for consumers that don't exist yet.
Invisible infra bitrots.

---

## 5. Scope and permissions

### Grafana role selection

| Role | Use for |
|---|---|
| `Viewer` | Read-only query consumers (Claude Desktop, dashboards-as-code readers, reporting) |
| `Editor` | Dashboard or alert provisioning tools that manage their own resources |
| `Admin` | Bootstrap/management only; avoid for runtime consumers |

One SA per consumer. Narrowest role that works. Never share tokens.

### OpenBao policy: minter (what the Mint Job uses)

```hcl
# Read admin credential for Grafana API auth
path "secret/data/platform/grafana/admin-password" {
  capabilities = ["read"]
}

# Write minted SA tokens
path "secret/data/platform/grafana/*" {
  capabilities = ["create", "update", "patch", "read"]
}
path "secret/metadata/platform/grafana/*" {
  capabilities = ["read", "list"]
}
```

### OpenBao policy: consumer (what an in-cluster workload uses)

```hcl
path "secret/data/platform/grafana/<token-name>" {
  capabilities = ["read"]
}
```

Attach to a dedicated K8s auth role bound to the workload's SA +
namespace. No wildcard reads on `secret/data/platform/*`.

### K8s auth role binding (minter)

```bash
bao write auth/kubernetes/role/grafana-sa-minter-role \
  bound_service_account_names=grafana-sa-minter \
  bound_service_account_namespaces=observability \
  policies=grafana-sa-minter-policy \
  ttl=10m
```

TTL is short (10m) because the Job runs once per mint — no need for a
long-lived token.

---

## 6. Rotation

Static KV tokens have no automated rotation — OpenBao does not rotate
opaque bearer tokens issued by other systems. Procedure:

1. Rerun the Mint Job with `ROTATE=true`, or delete the KV path and
   rerun (the Job is idempotent).
2. Job calls `POST /api/serviceaccounts/{id}/tokens` for a new token.
3. Job deletes the old token via
   `DELETE /api/serviceaccounts/{id}/tokens/{oldTokenId}`.
4. Job writes new value to the same OpenBao path (overwrites).
5. ExternalSecret resyncs within `refreshInterval` (1h default).
6. Consumer picks up on next restart — or immediately if Reloader is
   watching.

Cadence: manual, operator-driven. A future phase may add a CronJob to
rotate `platform/*` quarterly.

---

## 7. Offboarding (revocation)

1. Delete the token at the external system
   (`DELETE /api/serviceaccounts/{id}/tokens/{tokenId}` for Grafana).
2. Delete the KV path:
   `vault kv delete secret/platform/<service>/<credential>`.
3. If an ExternalSecret consumed it: delete the ExternalSecret. ESO
   cleans the materialized k8s Secret automatically (default
   `deletionPolicy: Delete`).
4. Restart consumer pods with the old value cached:
   `kubectl rollout restart deploy/<name>`. Without Reloader, this is
   manual.
5. Verify:
   - OpenBao audit log shows the `delete`.
   - Grafana audit log (if enabled) shows the token delete.
   - `curl -H "Authorization: Bearer $OLD_TOKEN" https://grafana.chalupatech.com/api/datasources` → 401.

---

## 8. Troubleshooting

| Symptom | Likely cause | Check |
|---|---|---|
| Mint Job 401 from Grafana | Admin password drift between Helm secret and SQLite | Pod restarted since `adminPassword` change? `GF_SECURITY_ADMIN_PASSWORD` env var present? See Appendix A. |
| ExternalSecret `SecretSyncedError` | Policy doesn't grant read on path | `bao policy read <policy>`; compare to ExternalSecret `remoteRef.key` |
| Consumer uses old token after rotation | Env-var mounts don't auto-reload | `kubectl rollout restart`; or deploy Reloader |
| `vault kv get` returns 403 for operator | OIDC-derived user policies don't include `platform/*` | Use `bao login -method=userpass` for break-glass; fix `ansible/roles/openbao_setup/templates/policy-user.hcl.j2` |

---

## 9. Reference implementations (legacy paths)

Existing flows predate this framework. They work and are stable — do
not migrate unless trivial. Use as shape reference:

| Service | Path | ExternalSecret | Consumer shape |
|---|---|---|---|
| Velero (S3 creds) | `secret/data/velero/minio` | `velero-s3-credentials` | Helm `existingSecret` + mounted file |
| Schwab tenants | `secret/data/schwab-<user>/*` | `schwab-secrets`, `timescaledb-*` | env vars; CNPG auto-reload via `cnpg.io/reload` label |
| Alertmanager Discord | `secret/data/observability/discord` | `alertmanager-discord-config` | ConfigMap injection |
| Grafana OIDC client | `secret/data/observability/grafana-oidc` | `grafana-oidc-secret` | `envFromSecret` into Grafana pod |

---

## References

- `docs/adr/ADR-008-service-account-framework.md` — decision record
- `ansible/roles/openbao_setup/tasks/app_setup.yml` — K8s-SA provisioning for `managed_apps`
- [Grafana Service Accounts HTTP API](https://grafana.com/docs/grafana/latest/developers/http_api/serviceaccount/)
- [External Secrets Operator — SecretStore](https://external-secrets.io/latest/api/secretstore/)

---

# Appendix A — Grafana admin-password drift fix recipe

**Why this exists:** The current observability Helm values do not set
`grafana.adminPassword`. The victoria-metrics-k8s-stack subchart
auto-generates one at install and stores it in a k8s Secret, but OIDC
login and later admin password edits cause drift between the Helm
secret and Grafana's SQLite `admin_user` row. Once drifted, any client
using Helm-secret admin creds (including the Mint Job) gets 401.

The fix: pin the admin password to an OpenBao-backed secret, and wire
it as `admin.existingSecret` so the subchart sets
`GF_SECURITY_ADMIN_USER`/`GF_SECURITY_ADMIN_PASSWORD` env vars on the
pod. Grafana resets the SQLite admin hash from those env vars on every
boot → drift cannot persist.

### Steps (phase-2a owns execution)

1. **Seed OpenBao** with a generated admin password at
   `secret/data/platform/grafana/admin-password` with keys `admin-user`
   (value: `admin`) and `admin-password` (generated). Add to Ansible
   Vault as `vault_grafana_admin_password`, propagate via vars.yml,
   seed via openbao_setup role (new task or extend `app_setup.yml`).

2. **Dedicated SecretStore** in `observability` namespace
   (`grafana-admin-store`) using a new K8s auth role
   `grafana-admin-reader-role` bound to the Grafana pod's SA. Policy:
   read on `secret/data/platform/grafana/admin-password` only.

3. **ExternalSecret** `grafana-admin-secret` in `observability` ns,
   `refreshInterval: 1h`, targets `grafana-admin-store`, maps
   `admin-user` → `admin-user` and `admin-password` → `admin-password`.

4. **values.yaml** edit:
   ```yaml
   grafana:
     admin:
       existingSecret: grafana-admin-secret
       userKey: admin-user
       passwordKey: admin-password
   ```
   (Remove any `adminUser`/`adminPassword` top-level if present.)

5. **Force pod restart** after ArgoCD sync:
   `kubectl rollout restart deploy/observability-grafana -n observability`.

6. **Verify**:
   ```bash
   ADMIN_PASS=$(vault kv get -field=admin-password secret/platform/grafana/admin-password)
   curl -s -u admin:$ADMIN_PASS https://grafana.chalupatech.com/api/org \
     | jq -e '.id == 1'
   ```
   Returns 200 with org id 1. OIDC login still works after this (Grafana
   allows both local admin and OIDC users simultaneously).

### Known gotchas

- **Sidecar `skipReload: true` comment** (`values.yaml:68-72`) was added
  as a workaround for the 401 drift. After the fix, `skipReload` can
  stay (file-watcher provisioner is the right default) but the
  *reason* comment should be updated to "dashboard watcher is
  sufficient; reload API not needed" rather than "admin password
  drifts."
- **OIDC users are unaffected**: they authenticate via OpenBao, not
  against the admin password. Changing `admin-password` does not log
  anyone out.
- **Rollback**: if the fix breaks admin login, `vault kv delete
  secret/platform/grafana/admin-password` and revert the values.yaml
  change. Next pod restart reverts to subchart-generated password (in
  the observability-grafana Secret).

---

# Appendix B — Grafana SA Mint Job (canonical example)

**Target:** `chalupa-infra/k8s/platform/observability/templates/grafana-sa-mint-*.yaml`

### B.1 ServiceAccount + RBAC

```yaml
apiVersion: v1
kind: ServiceAccount
metadata:
  name: grafana-sa-minter
  namespace: observability
---
# No in-cluster RBAC needed — Job auths to OpenBao via projected SA
# token, and to Grafana via admin creds pulled from OpenBao. No k8s
# API calls from the Job itself.
```

### B.2 OpenBao policy + K8s auth role (Ansible-seeded)

Add to `openbao_setup` role as a new task (e.g., `platform_setup.yml`,
invoked from `main.yml`), since this isn't a `managed_app`:

```yaml
- name: Write grafana-sa-minter policy
  ansible.builtin.uri:
    url: "{{ openbao_url }}/v1/sys/policies/acl/grafana-sa-minter-policy"
    method: PUT
    headers:
      X-Vault-Token: "{{ openbao_setup_root_token }}"
    body_format: json
    body:
      policy: |
        path "secret/data/platform/grafana/admin-password" {
          capabilities = ["read"]
        }
        path "secret/data/platform/grafana/*" {
          capabilities = ["create", "update", "patch", "read"]
        }
        path "secret/metadata/platform/grafana/*" {
          capabilities = ["read", "list"]
        }
    status_code: [200, 204]
    validate_certs: false
  no_log: true

- name: Write grafana-sa-minter K8s auth role
  ansible.builtin.shell: |
    kubectl exec -n openbao openbao-0 -- sh -c "
      BAO_TOKEN={{ openbao_setup_root_token }} bao write auth/kubernetes/role/grafana-sa-minter-role \
        bound_service_account_names=grafana-sa-minter \
        bound_service_account_namespaces=observability \
        policies=grafana-sa-minter-policy \
        ttl=10m
    "
  environment:
    KUBECONFIG: /etc/rancher/k3s/k3s.yaml
  changed_when: true
  no_log: true
```

### B.3 Mint script (ConfigMap-mounted)

```bash
#!/bin/sh
# mint-grafana-sa.sh — idempotent Grafana SA provisioner
# Env: SA_NAME (e.g. claude-desktop), SA_ROLE (Viewer|Editor|Admin),
#      KV_PATH (e.g. platform/grafana/claude-desktop-token),
#      ROTATE (true to force token regeneration even if path exists)
set -eu

GRAFANA_URL="${GRAFANA_URL:-http://observability-grafana.observability.svc.cluster.local}"
BAO_ADDR="${BAO_ADDR:-http://openbao-active.openbao.svc.cluster.local:8200}"

# 1. Authenticate to OpenBao via K8s SA token
SA_JWT=$(cat /var/run/secrets/kubernetes.io/serviceaccount/token)
BAO_TOKEN=$(curl -sf -X POST "$BAO_ADDR/v1/auth/kubernetes/login" \
  -d "{\"role\":\"grafana-sa-minter-role\",\"jwt\":\"$SA_JWT\"}" \
  | jq -r '.auth.client_token')
[ -n "$BAO_TOKEN" ] && [ "$BAO_TOKEN" != "null" ] || { echo "bao auth failed"; exit 1; }

# 2. Fetch Grafana admin creds
ADMIN_RESP=$(curl -sf -H "X-Vault-Token: $BAO_TOKEN" \
  "$BAO_ADDR/v1/secret/data/platform/grafana/admin-password")
ADMIN_USER=$(echo "$ADMIN_RESP" | jq -r '.data.data["admin-user"]')
ADMIN_PASS=$(echo "$ADMIN_RESP" | jq -r '.data.data["admin-password"]')

# 3. Skip if KV path already has a token and ROTATE is not set
if [ "${ROTATE:-false}" != "true" ]; then
  EXIST=$(curl -sf -H "X-Vault-Token: $BAO_TOKEN" \
    "$BAO_ADDR/v1/secret/data/$KV_PATH" 2>/dev/null \
    | jq -r '.data.data.token // empty')
  if [ -n "$EXIST" ]; then
    echo "token already present at $KV_PATH; skipping (set ROTATE=true to force)"
    exit 0
  fi
fi

# 4. Find or create the Grafana SA
SA_ID=$(curl -sf -u "$ADMIN_USER:$ADMIN_PASS" \
  "$GRAFANA_URL/api/serviceaccounts/search?query=$SA_NAME" \
  | jq -r ".serviceAccounts[] | select(.name==\"$SA_NAME\") | .id")

if [ -z "$SA_ID" ]; then
  SA_ID=$(curl -sf -u "$ADMIN_USER:$ADMIN_PASS" \
    -H "Content-Type: application/json" \
    -X POST "$GRAFANA_URL/api/serviceaccounts" \
    -d "{\"name\":\"$SA_NAME\",\"role\":\"$SA_ROLE\",\"isDisabled\":false}" \
    | jq -r '.id')
fi
[ -n "$SA_ID" ] && [ "$SA_ID" != "null" ] || { echo "SA create failed"; exit 1; }

# 5. Delete any existing tokens on this SA (rotation / re-mint)
curl -sf -u "$ADMIN_USER:$ADMIN_PASS" \
  "$GRAFANA_URL/api/serviceaccounts/$SA_ID/tokens" \
  | jq -r '.[].id' \
  | while read -r TID; do
      curl -sf -u "$ADMIN_USER:$ADMIN_PASS" \
        -X DELETE "$GRAFANA_URL/api/serviceaccounts/$SA_ID/tokens/$TID" >/dev/null
    done

# 6. Mint a new token
TOKEN=$(curl -sf -u "$ADMIN_USER:$ADMIN_PASS" \
  -H "Content-Type: application/json" \
  -X POST "$GRAFANA_URL/api/serviceaccounts/$SA_ID/tokens" \
  -d "{\"name\":\"$SA_NAME-$(date +%s)\"}" \
  | jq -r '.key')
[ -n "$TOKEN" ] && [ "$TOKEN" != "null" ] || { echo "token mint failed"; exit 1; }

# 7. Store in OpenBao
curl -sf -H "X-Vault-Token: $BAO_TOKEN" \
  -H "Content-Type: application/json" \
  -X POST "$BAO_ADDR/v1/secret/data/$KV_PATH" \
  -d "{\"data\":{\"token\":\"$TOKEN\",\"sa_id\":\"$SA_ID\",\"role\":\"$SA_ROLE\"}}"

echo "minted SA $SA_NAME (id=$SA_ID), role=$SA_ROLE, stored at $KV_PATH"
```

### B.4 Job manifest (ArgoCD PostSync hook)

```yaml
apiVersion: batch/v1
kind: Job
metadata:
  name: grafana-sa-mint-claude-desktop
  namespace: observability
  annotations:
    argocd.argoproj.io/hook: PostSync
    argocd.argoproj.io/hook-delete-policy: BeforeHookCreation
spec:
  ttlSecondsAfterFinished: 600
  backoffLimit: 2
  template:
    metadata:
      labels:
        app: grafana-sa-minter
    spec:
      serviceAccountName: grafana-sa-minter
      restartPolicy: OnFailure
      containers:
        - name: mint
          image: alpine:3.20
          command: ["/bin/sh", "/scripts/mint-grafana-sa.sh"]
          env:
            - name: SA_NAME
              value: "claude-desktop"
            - name: SA_ROLE
              value: "Viewer"
            - name: KV_PATH
              value: "platform/grafana/claude-desktop-token"
            # ROTATE left unset → idempotent; set to "true" via
            # `kubectl set env job/... ROTATE=true` to force rotation
          volumeMounts:
            - name: scripts
              mountPath: /scripts
          resources:
            requests:
              cpu: 10m
              memory: 32Mi
            limits:
              memory: 64Mi
      initContainers:
        - name: install-deps
          image: alpine:3.20
          command: ["/bin/sh", "-c", "apk add --no-cache curl jq && cp /usr/bin/curl /usr/bin/jq /deps/"]
          volumeMounts:
            - name: deps
              mountPath: /deps
      volumes:
        - name: scripts
          configMap:
            name: grafana-sa-mint-script
            defaultMode: 0755
        - name: deps
          emptyDir: {}
```

Simpler alternative (chosen in phase-2b's shipped implementation):
single-container `alpine:3.20` with inline `apk add --no-cache curl jq`
in the container command. The initContainer pattern above is retained
as a reference for cases where Job startup latency or mirror
availability are concerns — e.g., high-frequency rotation CronJobs.
For once-per-sync PostSync hooks the inline install is preferable.

### B.5 ConfigMap

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: grafana-sa-mint-script
  namespace: observability
data:
  mint-grafana-sa.sh: |
    # contents of B.3 script
```

### Verification (phase-2b owns execution)

1. ArgoCD syncs observability → Job runs to completion
   (`kubectl get job -n observability grafana-sa-mint-claude-desktop -o jsonpath='{.status.succeeded}'` = 1).
2. `vault kv get secret/platform/grafana/claude-desktop-token`
   returns a token.
3. `curl -H "Authorization: Bearer $TOKEN" https://grafana.chalupatech.com/api/datasources`
   returns 200.
4. Grafana UI → Admin → Service Accounts → `claude-desktop` visible,
   Viewer role, one active token.
5. Rotation smoke test: rerun Job with `ROTATE=true` → new token works,
   old returns 401.
