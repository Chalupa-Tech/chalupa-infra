# scripts/dashboards/ — dashboards as code

Source of truth for chalupa-infra Grafana dashboards. Built with
[grafana-foundation-sdk Go](https://github.com/grafana/grafana-foundation-sdk).

The committed JSONs at

- `k8s/apps/schwab/go-paper-trader/files/paper-trading.json`
- `k8s/apps/schwab/go-schwab-feed/files/schwab-feed.json`
- `k8s/apps/schwab/go-schwab-feed/files/schwab-rollouts.json`

are **generated artifacts**.
Do not hand-edit them.
CI fails any PR where a checked-in JSON drifts from `bash scripts/dashboards/build.sh`.
The set of guarded paths is the `RENDERS` array in `build.sh` plus the
matching `pull_request.paths` + drift-loop entries in
`.github/workflows/validate-dashboards.yml` — onboard a new dashboard by editing all three.

## Layout

```
scripts/dashboards/
  go.mod
  build.sh                              # iterates RENDERS, generates each JSON
  cmd/paper-trader/main.go              # `go run ./cmd/paper-trader > out.json`
  cmd/schwab-feed/main.go               # `go run ./cmd/schwab-feed > out.json`
  cmd/schwab-rollouts/main.go           # `go run ./cmd/schwab-rollouts > out.json`
  internal/
    common/                             # reused across dashboards
      datasources/                      # VictoriaMetrics, TimescaleDB UIDs
      thresholds/                       # named threshold tables
      cte/                              # shared SQL CTEs
      banner/                           # PAPER TRADING safety banner
      sql/                              # Postgres dataquery (SDK has none)
      layout/                           # GridPos sugar + row helpers
      normalize/                        # canonical-key JSON encoder
    papertrading/                       # the paper-trading dashboard
      build.go templating.go
      rows/                             # one Go file per dashboard row
        summary.go risk.go ...
    schwabfeed/                         # the schwab market feed dashboard
      build.go templating.go
      rows/feed_health.go rows/market_data.go
    schwabrollouts/                     # the schwab argo rollouts dashboard
      build.go templating.go panels.go  # 6 panels, no rows in source
```

`internal/` keeps each dashboard's row builders private. Cross-cutting
panel patterns belong under `internal/common/`.

## Workflow

```bash
# Generate JSON.
bash scripts/dashboards/build.sh

# Or run the binary directly to inspect output without writing the file.
cd scripts/dashboards
go run ./cmd/paper-trader | less

# Run twice and confirm bit-equivalent (also enforced by build.go's
# self-test).
go run ./cmd/paper-trader > /tmp/a.json
go run ./cmd/paper-trader > /tmp/b.json
diff -q /tmp/a.json /tmp/b.json    # must be silent
```

## Editing a panel

1. Find the row module under `internal/<dashboard>/rows/<row>.go`
   (e.g., `internal/papertrading/rows/summary.go`,
   `internal/schwabfeed/rows/feed_health.go`).
2. Edit the panel-builder factory (e.g., `equityVsStartingCash`).
3. `bash scripts/dashboards/build.sh` to regenerate JSON.
4. Commit Go sources + regenerated JSON in one PR.

## CI gate

`.github/workflows/validate-dashboards.yml` runs on PRs that touch
`scripts/dashboards/**` or the rendered JSON. It re-runs the build
and fails on any drift.

## Version pin

`go.mod` pins `grafana-foundation-sdk/go` to a build matching the
Grafana 11.6 schema, even though the cluster runs Grafana 12.4. The
classic dashboard schema is backwards-compatible; bumping to a v12.x
SDK when one ships is a planned re-baseline operation, not a routine
bump. See `docs/dashboards-as-code.md` for the runbook.

## Local Grafana preview

```bash
kubectl -n observability port-forward svc/observability-grafana 3000:80
# Open http://localhost:3000, Import dashboard, paste the rendered
# JSON, set UID to "paper-trading-preview" so the live dashboard is
# untouched.
```
