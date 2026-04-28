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

## Rendering panels (Image Renderer)

Programmatic PNG export of any panel uses the Image Renderer side-pod
installed in phase-1 (`grafana.imageRenderer.enabled` in
`k8s/platform/observability/values.yaml`). Audit workflows should reach
for this before falling back to a port-forward browser session.

- **Entry point (Claude Code MCP):**
  `mcp__grafana__get_panel_image dashboardUid=<uid> panel_id=<n>`
  returns `image/png` bytes. `scale=2` produces hi-DPI output suitable
  for PR descriptions.
- **Health check:**
  `kubectl -n observability get pod -l app.kubernetes.io/name=grafana-image-renderer`
  should show one Ready replica. The Grafana pod's
  `GF_RENDERING_SERVER_URL` env var (auto-wired by the chart) points at
  `http://observability-grafana-image-renderer.observability:8081/render`.
- **Resource sizing.** Sized for occasional single-user snapshots
  (128Mi req / 512Mi limit), not the production-scale rendering load
  the upstream docs target. If a dense dashboard OOMs, raise the
  per-renderer limit or skip that panel rather than over-provisioning.

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

## Audit findings (7e3)

Phase-7e3 audit covered the paper-trading port (post-7e2) plus the five
hand-written sibling dashboards.
The audit was programmatic — JSON structure, color modes, override
correctness, gridPos overlap detection, live-PromQL existence checks via
the Grafana MCP — because the cluster's Grafana lacks the Image Renderer
plugin (see Capability roadmap below). Pixel-level visual rendering is
deferred to the operator's local port-forward browser session.

| Dashboard | Panels | Issues found | Fix shape | Disposition |
| --- | ---: | --- | --- | --- |
| paper-trading (SDK, 7e2 baseline) | 52 | Panel 902 NATS-drops timeseries had no `or vector(0)` for steady-state empty rendering (ambiguous "panel broken" vs "no drops") | Inline (Go source) | **shipped this PR** |
| schwab-trading | 24 | None — every multi-series timeseries already uses palette-classic, 18 panel overrides honored, gridPos clean. Largest sibling (1360 lines). | None | **queue for SDK port** (`phase-7e<n>-port-schwab-trading`) |
| schwab-feed | 14 | 2 multi-series timeseries missing palette-classic (Poll Duration, NATS Publish Rate) — without it, all `by (type)` series rendered the same color | Inline JSON `+3 lines × 2` | **shipped this PR** |
| schwab-rollouts | 6 | 4 multi-series panels missing palette-classic; `$namespace` is a `constant` variable (not user-pickable — intentional? unclear) | Trivial in source, but JSON uses compact-style nested objects → round-trip patch produces +237 lines of formatting churn for 4 substantive lines. Skip inline. | **queue for SDK port** (palette-classic + namespace var ride along) |
| schwab-auth-lifecycle | 11 | (a) 4 multi-series panels missing palette-classic. (b) **3 referenced metrics absent from registry**: `schwab_auth_refresh_total`, `schwab_auth_refresh_duration_seconds_bucket`, `schwab_auth_last_successful_refresh_timestamp_seconds`. `go-schwab-auth` v0.6.0 (phase-9) instrumented these on the *scheduler* refresh path; the in-cluster pod is emitting only `schwab_auth_token_expiry_timestamp_seconds`. Possible causes: (i) callback-path refresh updates vault but not metrics (this is exactly **alert-triage phase-15**'s scope, already queued); (ii) scheduler hasn't actually attempted a refresh since the last pod restart (the access-token expiry shows 2.6d in the past — gauge is stale, suggesting no scheduler-path success). | (a) Same compact-JSON churn problem; defer. (b) **Service-side fix tracked under alert-triage phase-15** — already queued (`phase-15-auth-callback-metric-emission.md`). 7e3 audit confirms the gap still exists in production today; phase-15 may need to widen scope to also fix the scheduler-path staleness. Not a dashboard fix; reference only. | (a) **queue for SDK port** (palette-classic rides along). (b) **already covered** by alert-triage phase-15; flag the scheduler-staleness sub-finding in that phase's prompt. |
| telemetry-mesh | 24 | 4 multi-series panels missing palette-classic; 1 templated datasource variable (`DS_VM` type=datasource) is a legacy pattern that should pin to `VictoriaMetrics` constant on port | Inline JSON `+3 lines × 4` | **shipped this PR** (palette only); DS_VM cleanup rides along with SDK port |

**Live-data sample.**
All paper-trading metrics observed emitting (3 books active). Notable
expected-empty metrics whose corresponding panels render correctly:
`paper_orphaned_position_usd` (panel 1201 uses `or vector(0)`),
`paper_slippage_vs_decision_bps_*` (heatmap 1103 — metric ships in the
next paper-trader image roll), `paper_daily_loss_halt_total` (panels
302/303 — counter only appears post-first-halt; 302 is `or vector(0)`-guarded).
All `schwab_feed_*`, `schwab_position_*`, `schwab_account_*`,
`telemetry_mesh_*`, `rollout_phase`, `analysis_run_phase` metrics are
emitting and series counts match expected cardinality.

**Out-of-scope finding (2026-04-28): Datasource UID by name.**
Every dashboard references the VictoriaMetrics datasource as
`{"type": "prometheus", "uid": "VictoriaMetrics"}` — but the actual
datasource UID is auto-generated (`P4169E866C3094E38`); the literal string
`"VictoriaMetrics"` is the *name*, not the UID. Today this resolves via
Grafana's name-fallback, but it's brittle to datasource rename or
addition of a second `prometheus`-type datasource named `VictoriaMetrics
(DS)` (which already exists, UID `PA144FC3F5C193807`). A canonical fix
would either (a) pin the datasource UID via Helm provisioning, or
(b) store the UID in `internal/common/datasources` and update all
dashboards on every cluster rebuild. Queue as
`phase-7e<n>-datasource-uid-canonicalization` *or* fold into the
SDK-port phases — the SDK ports already centralize the datasource ref
in `internal/common/datasources`, so a one-line constant change there
fixes every ported dashboard simultaneously.

## Capability roadmap (7e3)

What the Grafana stack and the foundation SDK can express that we are
not currently exploiting, plus what's blocked behind a version bump.

### Plugins (cluster Grafana 12.4)

| Feature | Reachable today | Blocked on | Priority | Notes |
| --- | --- | --- | --- | --- |
| **Image Renderer** (programmatic PNG export of panels/dashboards) | Yes (phase-1) | — | Shipped | Unblocks `mcp__grafana__get_panel_image` for future audit phases — replaces port-forward browser flow. Default-off in `kube-prometheus-stack`; Helm values bump under `grafana.imageRenderer.enabled`. Single ArgoCD sync. |
| **Boom Table** (table cells with embedded sparklines + cell expressions) | No | Plugin install | Low — defer until a clear use case (e.g., per-symbol mini-spark on the watchlist table) | Third-party (yesoreyeram); review supply-chain story. |
| **Polystat** (multi-cell health overview tile) | No | Plugin install | Low — gauge panels + repeating cover most of the same ground | Third-party (grafana). |
| **Status History v2 / Business Charts** (Sankey, gauge groups) | No | Plugin install | Low — no current need | Third-party (volkovlabs); business-charts has a paid tier. |

### SDK builders we are not yet using

| Feature | Reachable today (SDK v0.0.12) | Notes |
| --- | --- | --- |
| Panel transformations beyond `organize` | Yes — `filterByName`, `calculateField`, `groupBy`, `seriesToColumns`, `joinByField` are all in the SDK's `common/DataTransformerConfig`. The 7e2 port wired only `organize`; the others remain unused. | Highest-yield first hit: `filterByName` to replace the per-book regex-filter PromQL on panels 605/606 (cleaner than fighting `book_id=~"$book_id"`). |
| Heatmap calculation modes (`heatmap_calc.HeatmapCalculationOptions`) | Yes | The 7e2 heatmaps (504, 1002, 1102, 1103) all use `bucket-bound from query`; SDK supports `from-data` modes. Not blocking — current modes work. |
| Threshold step icons (custom step-glyph rendering) | Yes — SDK has `thresholds.step.icon` | Useful for stat-panel readability ("✓ healthy" vs "✗ halted"). Stylistic; defer. |
| Library panels (panel-as-import) | Partial — SDK has no first-class library-panel API; you can emit `libraryPanel` JSON via the escape hatch | Defer until we have a concrete reuse target (banner is the obvious candidate). |
| Repeating panels / rows over a variable | Yes — `repeat`, `repeatDirection`, `repeatRowId` on PanelBuilder | We don't use this anywhere yet. Useful for per-book side-by-side comparison. |
| Annotations (cluster-wide event lines) | Yes — `dashboard.AnnotationContainerBuilder` | Wire for ArgoCD sync events, paper halts, OAuth refreshes. Requires cluster-side annotation source (Prometheus alert events, or a dedicated PostgreSQL table). |
| Drilldowns (data link templates) | Yes — `dashboard.DataLink` on panel options | Cleanest enabler for cross-dashboard navigation (see `briefs/dashboard-navigation-plan.md`). |
| Time-range presets (custom `time_options`) | Yes — `dashboard.DashboardBuilder.TimeOptions(...)` | Add "this trading day", "since last halt", "last NYSE session" once we have a holiday calendar source. |

### Blocked on SDK bump (Grafana 12.x features)

| Feature | Blocked on | Notes |
| --- | --- | --- |
| Gauge band coloring (12.x panel feature) | SDK v12 schema (no public release as of 2026-04) | Workaround: escape-hatch dict overlay (not yet implemented). Cost of overlay is per-call — defer until the second feature wants it. |
| Scenes panels | SDK v12 | Scenes is the Grafana-12 panel framework; SDK has no builder yet. |
| `business-charts` plugin types (Sankey etc.) | Plugin install + SDK v12 panel registration | Compound block; revisit when both unblock. |

### Decision: Image Renderer

**Recommendation:** install Image Renderer plugin in the cluster Grafana
to unblock `mcp__grafana__get_panel_image` for future audit phases. Cost
is one ArgoCD sync (`grafana.imageRenderer.enabled: true` in the
kube-prometheus-stack values), one extra Deployment with ~50 MB image,
and the network round-trip per panel render. Benefit is programmatic
visual audit replaces operator-driven port-forward, and unblocks a
future "dashboard screenshot in PR description" workflow. Queue as
`phase-7e<n>-grafana-image-renderer-install`.

## Usability inventory (7e3)

Each candidate is rated `value × effort` to prioritize follow-on phases.
Phase queueing recommendation in the last column.

| Feature | What it does | Value | Effort | Recommendation |
| --- | --- | --- | --- | --- |
| **Annotations: ArgoCD sync events** | Vertical line on every timeseries when an Application resyncs (paper-trader image rolls, dashboard CM updates, OAuth credential rotation). | High — most "what changed at 14:32?" questions resolve immediately. | Medium — needs an ArgoCD-events → Prometheus / annotation source. ArgoCD has a `notifications-controller` that can post to a webhook; pair with a small ingester. Or: use Grafana's built-in Prom annotations against `ALERTS{}` events. | **Queue** as `phase-<n>-argocd-annotation-source` once at least one SDK port has shipped (the SDK builder is `dashboard.AnnotationContainerBuilder`; annotations are dashboard-level, not panel-level). Highest-yield single board addition. |
| **Annotations: Paper halts** | Vertical line when `paper_daily_loss_halt_total` increments. | High — debug context for halt-related drawdowns. | Low — Prometheus annotation query: `changes(paper_daily_loss_halt_total[$__interval]) > 0`. | **Queue** with the ArgoCD-events phase; same builder, same dashboard. |
| **Time-range presets** | Custom timepicker entries: "this trading day", "since last halt", "last NYSE session". | Medium — operator currently uses `now-6h` and adjusts manually. | Low for static presets ("trading day" = 9:30 ET to 16:00 ET via offset math); medium for "last halt" (needs timestamp lookup). | **Defer** — `now-6h` works; reconsider after promotion-windows decide what "trading day" means. |
| **Repeating panels over `book_id`** | Replaces the per-book multi-series chart with N copies of a panel, one per book — side-by-side comparison. | Medium-high — easier per-book correlation than legend-color matching, especially when a book misbehaves and operator needs to focus. | Low — SDK supports `Repeat("book_id")`. | **Queue** as `phase-<n>-paper-trading-book-repeat-rows` after navigation impl lands. Best-fit panels: 605 (drawdown), 606 (rolling return), 1101 (slippage). |
| **Repeating rows over `strategy`** | Whole row repeats per strategy — "alternator-30s row" + "sma-5x20 row" stacked. | Low-medium — strategy comparison row (panels 501-504) already does this in summary form. Repeat would let operator drill into one strategy's full health. | Low — SDK row builder has `Repeat`. | **Defer** — single strategy-row already covers 80% of what repeats would expose. |
| **Library panels (banner)** | Hoist the PAPER TRADING banner to a Grafana library panel; siblings can `import` it. | Low — only paper-trading currently has a banner; the SDK port already deduplicates the banner via `internal/common/banner/banner.go`. Library panels would help if a sibling needs the same banner *before* its SDK port. | Medium — library panels are a Grafana-side construct (CM-provisioned), not first-class in the SDK. | **Defer** — revisit if/when a hand-written sibling needs the banner before its port. |
| **Drilldowns (panel data links)** | Click a heatmap cell or table row to jump to a filtered dashboard. | High — ties directly into the navigation plan brief; this is *the* enabler. | Low per-link (SDK `dashboard.PanelBuilder.Links([...])`); high cumulative if we wire all 10 flows. | **Queue** as `phase-<n>-paper-trading-drilldowns` (for the 5 paper-trading-rooted flows from the navigation brief). Cross-dashboard drilldowns ride along with each sibling's SDK port. |
| **Refresh interval policy** | Per-board refresh: `5s` for halt-watch, `30s` for daytime ops, `5m` for portfolio rolling-30d. | Low — the cluster Grafana defaults work; operators rarely care about second-level latency on slow-moving panels. | Low — single field on `DashboardBuilder`. | **Defer** — bundle with each sibling SDK port (decide refresh rate at port-time). |
| **`schwab_auth_refresh_*` metrics emission** | Three metrics referenced by schwab-auth-lifecycle but currently empty in production despite shipping in `go-schwab-auth` v0.6.0 (phase-9). | High — 4 of 8 functional panels on auth-lifecycle render empty until refresh events emit. | Already addressed by **alert-triage phase-15** (callback-path emission). 7e3 audit confirms the gap and surfaces a possible scheduler-staleness sub-finding to widen phase-15's scope. | **Reference** alert-triage phase-15; do not create a duplicate phase here. |

### Highest-yield phase ordering

If the operator commits to platform-dashboards as a workstream, the
sequence the audit suggests:

1. **platform-dashboards** `phase-1-grafana-image-renderer-install` —
   unblocks future audits.
2. **alert-triage** `phase-15-auth-callback-metric-emission`
   (already queued) — fixes 4 empty panels on auth-lifecycle without
   any dashboard touch.
3. **platform-dashboards** `phase-2-port-schwab-feed` — smallest
   sibling (695 lines), proves the migration playbook beyond
   paper-trading.
4. **platform-dashboards** `phase-3-paper-trading-drilldowns` — wires
   the 5 paper-trading-rooted navigation flows from the brief.
5. **platform-dashboards** `phase-4-port-schwab-rollouts` —
   second-smallest (290 lines).
6. **platform-dashboards** `phase-5-port-schwab-auth-lifecycle` — drops
   palette-classic fix in.
7. **platform-dashboards** `phase-6-port-telemetry-mesh` — drops
   `DS_VM` legacy var, retires the variable.
8. **platform-dashboards** `phase-7-port-schwab-trading` — largest
   (1360 lines), last.
9. **platform-dashboards** `phase-8-argocd-annotation-source` — adds
   annotations once the ports are stable.
10. **platform-dashboards** `phase-N-datasource-uid-canonicalization` —
    fold into the SDK port series as the datasources package is
    touched in each.
