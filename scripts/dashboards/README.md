# scripts/dashboards/ — dashboards as code

Source of truth for chalupa-infra Grafana dashboards. Built with
[grafana-foundation-sdk Go](https://github.com/grafana/grafana-foundation-sdk).

The committed JSON at
`k8s/apps/schwab/go-paper-trader/files/paper-trading.json`
is a **generated artifact**. Do not hand-edit it. CI fails any PR
where the checked-in JSON drifts from `bash scripts/dashboards/build.sh`.

## Layout

```
scripts/dashboards/
  go.mod
  build.sh                              # generate JSON into the Helm chart
  cmd/paper-trader/main.go              # `go run ./cmd/paper-trader > out.json`
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

1. Find the row module under `internal/papertrading/rows/<row>.go`.
2. Edit the panel-builder factory (e.g., `equityVsStartingCash`).
3. `bash scripts/dashboards/build.sh` to regenerate JSON.
4. Commit Python sources + regenerated JSON in one PR.

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
