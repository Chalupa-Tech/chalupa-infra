# GitHub Actions telemetry token — rotation

Rotates the bearer token that GitHub Actions uses to push metrics and
logs into the cluster via `vm-write.chalupatech.com` and
`vlogs-write.chalupatech.com`.

## When to rotate

- Scheduled: every 180 days, or whenever an operator or repo with
  access leaves the org.
- Ad-hoc: immediately on suspected leak (a token appearing in a public
  log line, a repo with the org-level secret being made public, etc.).

## What the token is

- Raw bearer at `secret/platform/telemetry/gh-actions.token` in OpenBao.
- Mirrored as an apr1 htpasswd line at
  `secret/platform/telemetry/gh-actions.htpasswd`, materialised into the
  K8s Secret `observability/gh-actions-telemetry-users` by ESO, and
  consumed by the Traefik `gh-actions-telemetry-auth` BasicAuth
  Middleware.
- Stored org-wide in GitHub as the repo/org secret `TELEMETRY_TOKEN`.

## Procedure

1. **Trigger a fresh mint.** The `gh-actions-telemetry-mint` ArgoCD
   PostSync hook is idempotent and skips when a token already exists.
   To force rotation, set `ROTATE=true` on the next run:

   ```bash
   kubectl -n observability create job \
     --from=job/gh-actions-telemetry-mint \
     gh-actions-telemetry-mint-rotate-$(date +%s) \
     --dry-run=client -o yaml \
     | kubectl set env --local -f - ROTATE=true -o yaml \
     | kubectl apply -f -
   ```

   Alternatively, re-sync the `observability` ArgoCD Application after
   deleting the stored entry at `secret/platform/telemetry/gh-actions`
   — the hook re-mints when the path is empty.

2. **Wait for ESO refresh (≤1h) or force it.** Verify the new htpasswd
   lands in the cluster Secret:

   ```bash
   kubectl -n observability get secret gh-actions-telemetry-users \
     -o jsonpath='{.data.users}' | base64 -d
   ```

   The `users` value should match
   `bao kv get -field=htpasswd secret/platform/telemetry/gh-actions`.
   To force an immediate refresh, annotate the ExternalSecret:

   ```bash
   kubectl -n observability annotate externalsecret \
     gh-actions-telemetry-users \
     force-sync="$(date +%s)" --overwrite
   ```

3. **Read the new raw token.**

   ```bash
   bao kv get -field=token secret/platform/telemetry/gh-actions
   ```

4. **Update the org-level GitHub secret.** Paste the new token into the
   `TELEMETRY_TOKEN` org secret at
   <https://github.com/organizations/Chalupa-Tech/settings/secrets/actions>.
   Scope: repos that call `gemini-review.yml` (chalupa-infra plus
   anything whitelisted per the Gemini coverage map).

5. **Verify from a canary workflow run.** After the secret is updated,
   re-run any gemini-review job and confirm the telemetry emit step
   (phase-39c) pushes without a 401.

## Verifying end-to-end

Once phase-39c is wired, a freshly rotated token should:

- Reject requests without `Authorization: Basic …` → 401.
- Reject `GET` / non-write paths → 404 (Traefik path match miss).
- Accept `POST /api/v1/write` and `POST /insert/jsonline` with the
  correct bearer → 204.

Until 39c lands, smoke-test manually from a Tailscale-joined host:

```bash
TOKEN=$(bao kv get -field=token secret/platform/telemetry/gh-actions)
curl -u "gh-actions:$TOKEN" -sS -o /dev/null -w '%{http_code}\n' \
  -X POST https://vm-write.chalupatech.com/api/v1/write \
  -H 'Content-Type: application/x-protobuf' \
  --data-binary ''
# Expected: 400 (empty body) — anything else points at auth or routing.
```

## Failure modes

- **Mint Job fails:** check `kubectl -n observability logs
  job/gh-actions-telemetry-mint`. Usual suspects: OpenBao quorum loss,
  `gh-actions-telemetry-minter-role` TTL expired without refresh (only
  affects in-flight Job runs — reapply via ansible-playbook
  `playbooks/openbao-only.yml`).
- **ESO doesn't pick up the rotation:** inspect
  `kubectl -n observability describe externalsecret
  gh-actions-telemetry-users`. The `Status.Conditions` block lists the
  last sync result and refresh time.
- **Traefik still serves old creds after rotation:** Traefik watches
  the backing Secret via the CRD informer; a stuck router usually
  indicates Traefik didn't observe the Secret update. Roll the Traefik
  pods (`kubectl -n kube-system rollout restart deploy/traefik`).
