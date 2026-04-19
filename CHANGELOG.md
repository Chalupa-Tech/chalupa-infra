# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- **Gemini review telemetry emit + dashboard + alerts** (phase-39c). Gemini
  CLI emits OTLP metrics over a backgrounded Grafana Alloy sidecar
  (`grafana/alloy:v1.15.1`) launched in `gemini-review.yml`; Alloy fans
  out to VictoriaMetrics `remote_write` at `vm-write.chalupatech.com`
  via the BasicAuth ingress from phase-39b. Log fan-out (finding
  classification events) and extraction into a reusable composite
  action are deferred to phase-44.
  - `.github/alloy/gemini-review.alloy` — Alloy River config
    (OTLP receiver → resource-detect → batch → prometheus.remote_write).
  - `k8s/platform/observability/templates/gen-ai-pricing-rules.yaml` —
    VMRule with per-model pricing rates and monthly/per-run cost
    recording rules (`gen_ai_review_cost_usd:monthly`,
    `gen_ai_review_cost_usd:per_run`), plus a
    `gen_ai_pricing_table_age_days` staleness metric.
  - `k8s/platform/observability/templates/gen-ai-alert-rules.yaml` —
    four VMRules: `GeminiSpendSoftCap80` (info, >$16/mo),
    `GeminiSpendSoftCap100` (warning, >$20/mo, `auto_comment_pr`
    label), `GeminiSpendHardCap` (critical, >$50/mo, `page` label),
    `GeminiPricingTableStale` (warning, >180d).
  - `k8s/platform/observability/templates/gemini-review-spend-dashboard.yaml`
    + `files/gemini-review-spend.json` — Grafana dashboard (monthly
    spend gauge, token-type stacked timeseries, latency p50/p95,
    cost per review, pricing-table age).
  - `docs/runbooks/gemini-review-spend-cap.md` — operator runbook
    including the manual `gh api` hard-cap halt procedure pending
    the alertmanager→GitHub Actions bridge in phase-44.

### Changed

- **Gemini Dispatch gate on `GEMINI_DISPATCH_ENABLED` org var**
  (phase-39c). `gemini-dispatch.yml` now refuses to spawn
  review/triage/invoke jobs when the org var is set to `false`.
  Undefined or any other value keeps dispatch enabled (backward
  compatible). Intended to be flipped by `GeminiSpendHardCap` once
  the alertmanager webhook bridge lands.
- `go-paper-trader` chart `0.3.0` → `0.3.1`: `paper-trading.json` multi-series colouring fix. The
  drawdown % panel and the cumulative realized P&L panel both still carried single-series
  `color.mode: thresholds`, which paints every series by value instead of by identity — with
  three books (`ddowell-alt-30s`, `ddowell-alt-60s`, `ddowell-alt-v03`) clustered near 0 %
  drawdown all three rendered in the same green shade and were visually indistinguishable.
  Switched both panels to `color.mode: palette-classic` (the default Grafana classic palette) so
  each book gets its own colour — matches the equity-curve panel's behaviour. Dropped the
  threshold colour stops that were no longer doing anything useful on a multi-series chart.
  (paper-trading phase-5a follow-up)
- `go-paper-trader` chart `0.2.0` → `0.3.0`: `paper-trading.json` dashboard split by `book_id`. New
  `$book_id` multi-select template variable (populated from `SELECT DISTINCT book_id FROM paper_fills`).
  Equity curve + drawdown % panels now render one series per selected book (GROUP BY / PARTITION BY
  `book_id`). New `Cumulative realized P&L by book` timeseries panel (id 20) placed directly under the
  equity curve for side-by-side research-question comparison (alternator-30s vs alternator-60s). Live
  positions table + trade-log table gain a `book_id` column and `WHERE book_id IN ($book_id)` filter;
  activity stats (total fills / buys / sells / notional) add the same filter. Pod-wide Prometheus
  stats (current equity, open positions, simulator-health row) are left unfiltered — their backing
  metrics do not carry a `book_id` label today; per-book equity is visible on the curve. PAPER TRADING
  banner retained (safety guarantee #6); `paper-dashboard-banner-lint.yml` passes. Image/appVersion
  unchanged. (paper-trading phase-5a)
- `go-paper-trader` deploy: image tag `v0.3.0` → `v0.4.0` (paper-trading phase-5 multi-book). Chart bumped
  to `0.2.0`. `values.yaml` replaces scalar `paperTrader.strategy`/`.symbol`/`.quantity`/`.startingCash`/
  `.alternatorCadence` with a `paperTrader.books:` list. New `templates/configmap.yaml` renders
  `books.yaml` from the list; mounted at `/etc/paper-trader/books.yaml`. Deployment passes
  `--config=/etc/paper-trader/books.yaml`; the scalar flags are gone. Default config runs two alternator
  books (`ddowell-alt-30s`, `ddowell-alt-60s`) on the 23-symbol watchlist (24 minus AAPL, which is not
  in the feed's watchlist), each starting with $10k cash. Pod resource bumps 32Mi→48Mi requests,
  64Mi→96Mi limits, 10m→20m CPU requests (two adapters + per-book NATS connections). The v0.4.0 image
  ships a schema migration that ALTERs `paper_fills` / `paper_positions` / `paper_cash` to add
  `book_id TEXT NOT NULL`, backfilling pre-phase-5 rows with the sentinel `ddowell-alt-v03`, then
  dropping the DEFAULT so future INSERTs must specify book_id. `UNIQUE(order_id)` replaced with
  `UNIQUE(book_id, order_id)` on `paper_fills`. Migration runs on pod start via
  `store.EnsureSchema` — same path phase-3a and phase-4b used. (paper-trading phase-5)
- `go-paper-trader` deploy: image tag `v0.2.0` → `v0.3.0` (bundles phase-4 hotfix + phase-4a + phase-4b). The
  one-shot `PAPER_TRADER_ALLOW_REALIZED_PL_BACKFILL=1` env was used to roll pre-phase-4a NULL `realized_pl`
  SELL rows through `BackfillMissingRealizedPL` on the cluster `market` DB, then removed once `null_sell == 0`
  was verified on the cluster. Backfill outcome: `scanned=49 updated=24 null_before=24 null_after=0`.
  (paper-trading phase-4c)

### Added

- `base-images/chalupa-base-go`: Tier 1 shared base image built `FROM scratch` with CA certificates
  and nonroot user (UID 65532). Replaces the repeated `FROM scratch` + CA cert copy pattern across
  all Go services.
- `base-images/chalupa-base-job`: Tier 2 shared base image extending `chalupa-base-go` with static
  `bao` (OpenBao CLI v2.2.0), `nats` (natscli v0.2.3), and `jq` (official static v1.7.1) binaries
  for job/CronJob workloads.
- `.github/workflows/build-base-images.yml`: CI pipeline that builds and pushes both base images to
  the Gitea registry. Triggers on `base-images/**` path changes to main, weekly schedule (Monday
  4am UTC for security updates), manual dispatch, and semver git tags (`v*`). `chalupa-base-job`
  build depends on `chalupa-base-go` completing first.
- `renovate.json`: Added `dockerfile` manager and `hostRules` for `gitea.tailbecff0.ts.net`.
  Patch/minor base image bumps auto-merge; major bumps require dashboard approval.
- Extended `build-push-reusable.yml` with optional `context` input (default `.`) so callers can
  specify a subdirectory build context without duplicating buildx/login logic.

### Changed

- `go-paper-trader` dashboard (`paper-trading.json`): `Total fills`, `Buys`, `Sells` stat panels
  now query Timescale `paper_fills` instead of Prometheus `paper_fills_total`. Matches the data
  source of the adjacent `Notional traded` panel, so activity-row counters no longer reset on
  pod restart while notional continues showing lifetime totals.
- `timescaledb` chart: `papertrader` role is now declaratively provisioned via CNPG
  `managed.roles` + `timescaledb-init-roles` bootstrap SQL + new `timescaledb-papertrader-secret`
  ExternalSecret. Grants `CREATE, USAGE ON SCHEMA public` (no blanket `pg_read_all_data` —
  least-privilege; papertrader owns the execution-state tables it creates via `EnsureSchema`).
  Fresh-cluster bootstrap no longer requires the manual `psql -c "CREATE ROLE papertrader"` +
  `GRANT` operator steps that phase-4 deferred; the only remaining prereq is seeding the
  `secret/schwab-ddowell/timescaledb/papertrader` vault subpath with `APP_PASSWORD` before
  ArgoCD sync (same shape as the existing `app_password` / `grafana_password` prereqs).
