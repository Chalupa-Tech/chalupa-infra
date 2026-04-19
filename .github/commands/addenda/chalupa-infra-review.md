# Review preamble — chalupa-infra

This repo is primarily Helm charts, ArgoCD ApplicationSets, Grafana
dashboards, vm-k8s-stack config, and GitHub Actions workflows.
Application Go code is rare here.

## Priorities for this repo

When reviewing a chalupa-infra PR, focus on — in this order:

1. **Rolling-upgrade safety of chart changes.** Any chart touching
   a Deployment/StatefulSet that also changes a ConfigMap or Secret
   the pod mounts MUST have a checksum annotation on the pod
   template. Flag missing
   `checksum/config: {{ include (print $.Template.BasePath
   "/configmap.yaml") . | sha256sum }}` (or equivalent) as HIGH —
   it silently skips rolling restarts when config changes and is
   the #1 source of "config change merged but pods never picked it
   up" incidents.

2. **ArgoCD sync-wave + hook correctness.** Look for
   `argocd.argoproj.io/sync-wave` annotations that skip values (a
   gap in the sequence is usually a bug, not intentional). PostSync
   Job hooks with `hook-delete-policy: HookSucceeded` + short
   `ttlSecondsAfterFinished` leave no trace after success — don't
   require the Job object to "prove" a hook ran; verify via Events
   + observable side-effects.

3. **ExternalSecrets path tier.** Secrets for end-user apps live
   under `apps/*` OpenBao paths; platform service accounts under
   `platform/*`. Flag any new ExternalSecret reading from a path
   that doesn't match its consumer's tier.

4. **vm-k8s-stack alerting discipline.** Alerting is vmalert-only
   by design — flag any PR proposing Grafana-native alerting
   wiring (`alerting.enabled: true` on Grafana chart, Grafana alert
   rule YAML, contact point configs outside of vmalert). The
   canonical firing-alert query is `ALERTS{alertstate="firing"}`;
   flag queries like `ALERTS_FOR_STATE` or
   `sum by(alertname)(ALERTS)` that bypass the right engine.

## Gotchas specific to this repo

- **`FROM scratch` static-binary trap.** Dockerfiles that take an
  apk-sourced binary (e.g. from `alpine:3.x`) and COPY it into
  `FROM scratch` will segfault — apk binaries aren't statically
  linked. Flag any `COPY --from=<alpine>` into `scratch`. The fix
  is an official static release (`jqlang/jq`, `openbao/bao`,
  `natscli`).

- **build-push tag-path-filter bug.** `paths:` filter under
  `on.push` in build-push workflows suppresses tag builds for tag
  pushes that only touch CHANGELOG.md. Release PRs will create the
  tag but no image. If reviewing a workflow change that
  adds/narrows `paths:` under `on.push` with `tags:` also listed,
  flag the interaction.

- **Grafana multi-series coloring.** Any Grafana panel JSON that
  uses template variables like `$book_id`, `$symbol`,
  `$registration` in the query must have
  `fieldConfig.defaults.color.mode = "palette-classic"`.
  Thresholds-mode collapses same-value series to the same color,
  producing unreadable dashboards. Flag `"mode": "thresholds"` on
  multi-series-varied panels as MEDIUM.

- **kube_pod_labels not scraped.** The cluster's Prometheus
  relabeling scrapes `kube_pod_info` only; `kube_pod_labels` is
  unavailable. Any PromQL that joins on `kube_pod_labels` won't
  work — flag queries using
  `* on(namespace,pod) group_left(label_*) kube_pod_labels`
  against the cluster's metrics.

- **vm-k8s-stack dashboard disable key.** Disabling upstream
  dashboards uses
  `defaultDashboards.dashboards.<name>.enabled: false` where
  `<name>` = chart filename minus `.yaml` (not the dashboard
  title). Easy to get wrong silently.

- **Kubelet job `node` label.** `up{job="kubelet"}` already
  carries `node` via VM operator relabeling. No `label_replace`
  needed for node-keyed inhibition rules — flag any such
  scaffolding as unnecessary.

- **Rolling-upgrade-safe DDL (rare here, but flag if seen).** Any
  migration adding a NOT NULL column with DEFAULT must be split:
  add in release N, DROP DEFAULT in release N+1 after every
  writer is past N. Combined migrations are CRITICAL.

## Default to LOW severity for

- YAML formatting / whitespace / key ordering (unless it breaks
  Helm templating).
- Comment style.
- Existing-tech-debt-adjacent nits unless the diff introduced new
  debt.

## Do not flag

- Absence of unit tests in chalupa-infra PRs. This repo doesn't
  carry unit tests; verification is via ArgoCD/Helm template +
  live cluster observation.
- Missing error handling in Helm templates (Go templates don't
  have exceptions; the pattern is `required "..."` for required
  values).
- Shell-style concerns in CI workflow files unless they're
  shell-injection vectors from untrusted input.
