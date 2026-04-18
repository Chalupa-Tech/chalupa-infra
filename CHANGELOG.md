# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Changed

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
