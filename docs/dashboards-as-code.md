# Dashboards as code

chalupa-infra Grafana dashboards are generated from Go sources under
`scripts/dashboards/` using the [grafana-foundation-sdk Go
library](https://github.com/grafana/grafana-foundation-sdk). The
committed JSON is a build artifact, gated by a CI drift check.

## Why

Phase-7b shipped a 52-panel paper-trader dashboard as 1762 lines of
hand-edited JSON. The PR was unreviewable by eye, broke Gemini review at
MAX_ARG_STRLEN (146 KB review context > 128 KB env-var ceiling), and
left a one-shot Python script (`scripts/phase-7b-dashboard-upgrade.py`)
that nobody could safely modify.

Phase-7e replaces hand-edited JSON with a Go module. Trade-offs:

- **+** ~30-line Python diffs for typical dashboard changes (vs ~800
  lines of nested JSON).
- **+** Compile-time errors for misspelled panel field names, wrong
  enum values, etc.
- **+** Native to the chalupa-tech ecosystem (every other service is
  Go). One toolchain across the org.
- **+** SDK ships an `UnknownDataquery` map type for plugin queries
  (Postgres) — no custom shim required.
- **−** Verbose: ~30-40% more lines per panel than the Python
  equivalent. Acceptable cost for the operator-experience and
  ecosystem-fit wins.

## Why not grafonnet / grafanalib / Terraform / CRDs?

- **grafonnet (Jsonnet):** [deprecated](https://github.com/grafana/grafonnet) by Grafana Labs in favour of the foundation SDK.
- **grafanalib (Python, Weaveworks):** unofficial, low cadence.
- **Terraform `grafana` provider:** verbose for dashboard-only repos; better fit when also managing datasources / RBAC / plugins.
- **Grafana operator + Dashboard CRDs:** requires the operator and a CRD lifecycle. We already provision via Helm `.Files.Get` → ConfigMap → kiwigrid sidecar; no operator needed.

## Layout

```
scripts/dashboards/
  go.mod
  build.sh                              # generate JSON into the Helm chart
  cmd/paper-trader/main.go              # one binary per dashboard
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

`internal/` keeps each dashboard's row builders private to its package;
shared building blocks live under `common/`. Adding a new dashboard
means a new `cmd/<name>/main.go` + `internal/<name>/` package.

## Adding or editing a panel

1. Edit the relevant row file under
   `internal/papertrading/rows/<row>.go`. Each row exports a `func
   Row(db *dashboard.DashboardBuilder, yBase int) int` that appends
   its panels and returns the y-units the row consumed.
2. Run `bash scripts/dashboards/build.sh`. It runs
   `go run ./cmd/paper-trader > k8s/.../paper-trading.json.tmp` then
   atomically renames the output into place.
3. Inspect the diff: `git diff
   k8s/apps/schwab/go-paper-trader/files/paper-trading.json`. For a
   typical change, expect ~30 lines of JSON delta.
4. Commit Go source + regenerated JSON in the same PR.

## CI gate

`.github/workflows/validate-dashboards.yml` triggers on PRs touching
`scripts/dashboards/**` or the rendered JSON. It:

1. Compiles the Go module (`go vet`, `go build`).
2. Runs `go run ./cmd/paper-trader` twice and asserts the outputs are
   byte-identical (determinism check).
3. **(Phase 7e2 will add)** re-runs the build script and runs `git
   diff --exit-code` against the committed JSON.

The complementary `paper-dashboard-banner-lint.yml` gate (independent
of this workflow) greps the committed JSON for the literal
"PAPER TRADING" substring. The banner is constant in
`internal/common/banner/banner.go` with a startup `init()` assertion
that fails the build if the literal is ever lost.

## Determinism strategy

Layered, fail-fast:

1. **Go's stdlib `json.Marshal`** orders keys by struct field
   declaration order. Our `internal/common/normalize/normalize.go`
   re-marshals through a recursive sorted-key encoder.
2. **Volatile root keys stripped** (`version`, `iteration`, `id`,
   `gnetId`) — Grafana auto-fills them server-side.
3. **`tags` arrays sorted** before encode — Grafana treats `[a, b]`
   and `[b, a]` as semantically identical.
4. **Self-test in `papertrading.Render()`:** builds twice in process,
   `bytes.Equal` check. Fails loud with a build error before writing.
5. **CI mirror:** `validate-dashboards.yml` runs the binary twice and
   asserts byte-identical output. Catches anything that slips past
   the in-process self-test.

If determinism ever breaks, debug by removing layers in order
1 → 2 → 3 — the first one to remove and still see drift is the
culprit.

## VM-stack ↔ SDK pin runbook

The cluster Grafana version is whatever
`victoria-metrics-k8s-stack` 0.72.5 ships — **Grafana 12.4.1** as of
phase-7e. The Go SDK pin in `scripts/dashboards/go.mod` targets
**v0.0.12 (Grafana 11.6 schema)** because:

- Foundation SDK has no v12 schema branch on PyPI/Go modules as of
  2026-04.
- Grafana 12.4 reads v1 schema dashboards (the schema is
  backwards-compatible). Our committed JSON uses v1, so the cluster
  renders the SDK output cleanly.

**When you bump `victoria-metrics-k8s-stack`:**

1. Read the new chart's appVersion / Grafana subchart version.
2. If Grafana's major version changed, also bump
   `grafana-foundation-sdk/go` to a matching SDK build. Treat as a
   **planned re-baseline**: regenerate JSON, expect a (possibly
   large) cosmetic diff, commit both the SDK bump and the regenerated
   JSON in a single PR.
3. Spot-check the dashboard renders correctly via local port-forward
   before merging.

## Renovate policy

`renovate.json` does not configure a Go-mod manager (only `helmv3`
and `dockerfile`). SDK pin bumps are operator-driven by design — every
bump regenerates JSON and is a planned re-baseline, not a routine
auto-PR. Do not add a Go manager to renovate.json without first
working through the rebase/CI implications.

## Local Grafana preview

```bash
kubectl -n observability port-forward svc/observability-grafana 3000:80
# Open http://localhost:3000.
# Top right: Dashboards → Import → paste the rendered JSON.
# Set UID to "paper-trading-preview" so the live dashboard is not
# overwritten.
```

## Migration plan for the other custom dashboards

| Dashboard | File | Lines | Status |
| --- | --- | --- | --- |
| Paper trading | `k8s/apps/schwab/go-paper-trader/files/paper-trading.json` | ~2086 | **Phase 7e1** (slice ported) → **Phase 7e2** (full port + cutover) |
| Schwab trading | `k8s/apps/schwab/go-schwab-feed/files/schwab-trading.json` | 1360 | TBD |
| Schwab feed | `k8s/apps/schwab/go-schwab-feed/files/schwab-feed.json` | 695 | TBD |
| Schwab Argo Rollouts | `k8s/apps/schwab/go-schwab-feed/files/schwab-rollouts.json` | 290 | TBD |
| Schwab auth lifecycle | `k8s/apps/schwab/go-schwab-auth/files/schwab-auth-lifecycle.json` | 329 | TBD |
| Telemetry mesh | `k8s/apps/telemetry-mesh/files/telemetry-mesh-dashboard.json` | 972 | TBD |

Each migration gets its own phase informed by phase-7e's lessons. The
package layout (`internal/<dashboard>/`) is designed to absorb new
dashboards as siblings without restructuring `common/`.

## Phase-7e split

| Phase | Deliverable | Status |
| --- | --- | --- |
| **7e1** | Go skeleton + 7-panel vertical slice exercising every panel type + CI determinism gate + this doc | **shipped** |
| **7e2** | Port remaining 38 panels, deterministic ID renumbering, `paper-trading.json` cutover, CI drift gate flipped on, `phase-7b-dashboard-upgrade.py` deleted | **shipped** |

## Panel ID renumbering (7e2)

7e2 renumbered every panel ID to `row_index * 100 + child_index_within_row`,
which (a) self-documents row layout from the panel ID alone and
(b) resolves the duplicate id=20 carried over from phase-7b (one
panel + one row both at id=20). The banner stays at `id=1`.

Old → new mapping (for anyone holding URL bookmarks):

| Old | New | Title |
| ---:| ---:| --- |
| 1 | 1 | banner |
| 100 | 200 | _row_ Summary (at-a-glance) |
| 101 | 201 | Equity vs starting cash |
| 102 | 202 | Today P&L per book |
| 103 | 203 | Max drawdown (session) |
| 104 | 204 | Win rate per book |
| 30 | 300 | _row_ Risk |
| 31 | 301 | Daily P&L per book (USD, UTC day) |
| 32 | 302 | Books halted (today) |
| 33 | 303 | Halt reject rate per book (5m) |
| 34 | 304 | Daily loss limit per book |
| 110 | 400 | _row_ Strategy quality |
| 111 | 401 | Sharpe ratio (30d rolling) |
| 112 | 402 | Sortino ratio (30d rolling) |
| 113 | 403 | Profit factor |
| 114 | 404 | Avg win / Avg loss (per day) |
| 120 | 500 | _row_ Strategy comparison |
| 121 | 501 | Equity by strategy |
| 122 | 502 | Fills per hour by strategy |
| 123 | 503 | Slippage tax by strategy (USD) |
| 124 | 504 | Realized P&L distribution by strategy |
| 2 | 600 | _row_ Account |
| 3 | 601 | Equity curve (cash + mark) by book |
| 4 | 602 | Current equity by book |
| 5 | 603 | Open positions by book |
| 20 (timeseries) | 604 | Cumulative realized P&L by book |
| 6 | 605 | Drawdown % by book (peak-to-trough) |
| 130 | 606 | Rolling return % by book (7d / 30d) |
| 7 | 700 | _row_ Positions & trades |
| 8 | 701 | Live positions |
| 9 | 702 | Trade log (last 50 fills) |
| 10 | 800 | _row_ Activity |
| 11 | 801 | Total fills |
| 12 | 802 | Buys |
| 13 | 803 | Sells |
| 14 | 804 | Notional traded |
| 15 | 805 | By-strategy notional |
| 16 | 900 | _row_ Simulator health |
| 17 | 901 | Quote age per symbol |
| 18 | 902 | NATS dropped messages (rate/5m) |
| 19 | 903 | Fill persist p99 latency |
| 20 (row) | 1000 | _row_ Execution quality |
| 21 | 1001 | Market-closed rejects by book |
| 22 | 1002 | Fill spread distribution (bps) |
| 23 | 1003 | Fill spread P50 / P95 / P99 by book |
| 27 | 1100 | _row_ Fill realism |
| 28 | 1101 | Fill-price advantage lost — P50 / P95 |
| 29 | 1102 | Fill-price advantage lost — distribution |
| 140 | 1103 | Slippage vs decision-time mid (bps) |
| 24 | 1200 | _row_ Hygiene |
| 25 | 1201 | Orphaned position $ (all books) |
| 26 | 1202 | Orphaned positions by (book, symbol) |

## Known limitations

- **Cosmetic diff vs hand-written JSON.** The SDK emits more default
  fields than Grafana's web UI (e.g., `transparent: false`,
  `repeatDirection: h`, `fiscalYearStartMonth: 0`). These are benign;
  Grafana ignores them. The committed JSON post-7e2 is the new
  baseline.
- **No round-trip from JSON.** The SDK has no `Dashboard.fromJSON()`
  helper, so porting hand-written panels is manual translation. The
  phase-7b script's panel-builder factories were the structural seed
  for the row modules; future porting follows the same pattern.
- **SDK tracks Grafana 11.6 schema, cluster is Grafana 12.4.** Until a
  v12 SDK ships, panels using Grafana-12-only features can't be
  reached natively. Workaround: deep-merge a raw dict overlay onto
  the SDK panel via an escape-hatch helper (not yet added —
  introduce when needed, document each call site, remove when SDK
  catches up).
