# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Changed (repo-hygiene phase-6 — auto-release-promotion)

- **`release-reusable.yml` learns tier-2 auto-promote.**
  Two-tier behavior on every workflow run.
  Tier 1 (unchanged): if CHANGELOG's first section is `## [X.Y.Z]`,
  use it as-is.
  Tier 2 (new): if the first section is `## [Unreleased]`, derive the
  next version from conventional-commit subjects in `LAST_TAG..HEAD`
  (`feat:`/`feat(scope):` → minor; `fix:`/`fix(scope):` → patch;
  any type with `!:` or `BREAKING CHANGE:` in body → major; only
  `chore:`/`docs:`/`ci:`/`test:`/`refactor:` → skip release), rewrite
  CHANGELOG.md (`## [Unreleased]` → `## [Unreleased]\n\n## [X.Y.Z] -
  DATE`), commit + push to main with `chore: promote Unreleased to
  vX.Y.Z`, then continue to the existing tag → image → chart-bump
  pipeline. Replaces the manual "hotfix: cut vX.Y.Z" PR pattern that
  produced two reactive trader releases (v0.10.0, v0.11.0) in the
  preceding week.
- **`release-deferred` label escape hatch.** If every commit since
  `LAST_TAG` is on a PR labeled `release-deferred`, the auto-promote
  exits without bumping. Same label honored by the existing
  check-changelog gate; now also honored on the release side.
- **Bot-commit loop guard.** If HEAD's author is
  `github-actions[bot]` AND subject matches `^chore: promote
  Unreleased to v`, the auto-promote exits immediately. Second line
  of defense: chore-only classification ensures the bot's commit
  alone since `LAST_TAG` produces no release.
- **Checkout now uses `GH_PAT`** so the auto-promote bot can push past
  the org "Protect Main" ruleset. Default `GITHUB_TOKEN` cannot
  bypass; existing `Create Tag` and `deploy` jobs already needed
  PAT-equivalent privileges. No change to the input/secret surface
  exposed to callers — auto-promote is internal to the reusable.

### Fixed (hotfix — paper-trader CrashLoopBackOff after phase-11c)

- **`go-paper-trader` chart bumped to `0.3.13`** with `appVersion`
  `"0.11.0"` and `image.tag` `v0.11.0`. Phase-11c (#443) added the
  `ddowell-buy-and-hold` book entry to `values.yaml` but the trader
  binary release that knows the `buy_and_hold` strategy never shipped
  — the trader-side phase-11c PR
  (`Chalupa-Tech/go-schwab-accounts-and-trading#37`) merged without
  a `release-deferred` label and without a version bump (CI gate from
  `Chalupa-Tech/go-schwab-accounts-and-trading#35` should have caught
  it but didn't). Result: cluster ran the v0.10.0 binary against the
  v11c configmap for ~5 hours, crashing on
  `book ddowell-buy-and-hold: quantity must be > 0`. The companion
  trader hotfix at
  `Chalupa-Tech/go-schwab-accounts-and-trading#38` cuts v0.11.0;
  this PR bumps the chart to consume it. Recurrence of the pattern
  PR #424's predecessor (trader v0.10.0 hotfix #33) already fixed.

### Added (paper-trading-realism phase-11c — evaluation cleanup)

- **5th paper-trader book `ddowell-buy-and-hold`** in
  `k8s/apps/schwab/go-paper-trader/values.yaml`. `strategy:
  buy_and_hold`, `startingCash: 10000`, `session: regular`, full
  24-symbol watchlist matching `ddowell-alt-30s` /
  `ddowell-alt-60s` / `ddowell-sma-5x20` so the equity-multiple panel
  comparison is apples-to-apples. No `quantity` / cadence / window
  knobs — sizing is derived from `StartingCash` and the symbol set;
  the trader-side validate skips `quantity > 0` for `buy_and_hold`.
- **Equity-multiple-of-starting-cash panel** (id 607) under the Account
  row. Renders `paper_equity_usd / on(book_id) group_left()
  paper_starting_cash_usd`, palette-classic per-book coloring
  (memory `reference_grafana_multi_series_coloring`). The canonical
  "which strategy wins?" panel — absolute USD curves mislead when
  books start at different wall-clock times. Account row height grows
  from 34 → 42 grid units.

### Changed (paper-trading-realism phase-11c — evaluation cleanup)

- **Summary row's "Equity vs starting cash" stat** (panel 201) divides
  by `paper_starting_cash_usd{book_id}` instead of the hardcoded
  `10000` literal. Books that start with anything other than $10k
  (e.g. `ddowell-limit-example` at $5k) now render correctly instead
  of silently misreporting. Description annotates the migration so
  operators can audit.
- **Default-collapsed rows: `Simulator health` + `Fill realism`.** Both
  rows nest their child panels under a `Collapsed(true)` row builder
  so the operator-debug surface stays one click away during a 30-day
  evaluation window without dominating the landing view. Strategy
  evaluation rows (Summary, Risk, Strategy quality, Strategy
  comparison, Account, Positions & trades, Activity, Orders, Working
  orders, Reconciliation, Execution quality, Hygiene) remain expanded.
- **Variable display labels.** `book_id` → `Book`, `symbol` →
  `Symbol`, `strategy` → `Strategy` via
  `QueryVariableBuilder.Label(...)` in `templating.go`. Cosmetic but
  legible.
- **Tooltip text on 9 evaluation-flagship panels** that previously
  rendered with no `description`: 601 (Equity curve), 603 (Open
  positions), 604 (Cumulative realized P&L), 701 (Live positions),
  702 (Trade log), 805 (By-strategy notional), 901 (Quote age), 902
  (NATS dropped messages), 903 (Fill persist p99 latency).

### Added (paper-trading-realism phase-11a — Reconciliation dashboard row)

- **New `Reconciliation` row on `paper-trading.json`** (row id 920,
  panels 921-923), inserted directly below `Simulator health`. Three
  panels surface the per-book reconciler state shipped in
  `go-schwab-accounts-and-trading` phase-11a:
  - **Panel 921: Seconds since last reconcile sweep** (stat). Renders
    `time() - paper_reconcile_last_run_timestamp_seconds{book_id=~"$book_id"}`.
    Green at < 60s (default cadence), yellow at the next missed tick
    (60–120s), red past 2× cadence — a stalled reconciler goroutine
    surfaces here before the divergence counter goes silent.
  - **Panel 922: Healthy sweeps (last 15m)** (stat). Renders
    `sum by (book_id) (increase(paper_reconcile_healthy_sweeps_total[15m]))`.
    Red at zero (reconciler not running OR every sweep is divergent);
    green at ≥1.
  - **Panel 923: Divergence rate by kind** (timeseries,
    palette-classic, red threshold at 0.001). Renders
    `sum by (book_id, kind) (rate(paper_reconciliation_divergence_total[15m]))`.
    Sim's steady state is a flat zero line; any non-zero rate is a
    write-through ordering bug. Drilldown to the structured log line
    via VictoriaLogs explorer with `_msg:"reconciliation discrepancy"`
    is documented in each panel description.
- **`thresholds.ReconcileLiveness` and
  `thresholds.ReconcileDivergence`** in
  `scripts/dashboards/internal/common/thresholds/thresholds.go` for
  the new panels' threshold tables.

### Added (paper-trading-realism phase-10b — limit_example deploy)

- **4th paper-trader book `ddowell-limit-example`** in
  `k8s/apps/schwab/go-paper-trader/values.yaml`. Single-symbol
  (`AMD`) LIMIT-using book exercising the phase-10
  place / cancel / replace lifecycle. Knobs:
  `offsetBps: 10`, `maxRestDuration: 5m`, `rebalanceBps: 5`,
  `quantity: 5`, `startingCash: 5000`, `session: regular`.
  Exists so the phase-10 "Working orders" dashboard row populates
  with real acknowledged LIMITs during market hours and the
  EOD-cancel + matching-engine paths see live traffic — phase-10
  shipped the surface but no shipping strategy exercised it.
- **`go-paper-trader` chart bumped to `0.3.12`** with image tag
  `v0.10.0`, picking up the trader-side `limit_example` strategy
  package and `cmd/paper-trader` wiring. The book added above
  references `strategy: limit_example`, which only exists at
  `appVersion >= 0.10.0`; the chart bump and book add land in the
  same PR so ArgoCD never reconciles a state where the YAML
  references a strategy the running image cannot construct.

### Added (paper-trading-realism phase-10 — working-orders dashboard row)

- **New `Working orders (phase-10)` row** on `paper-trading.json`,
  inserted directly below the existing `Orders` row (id 850 → 870).
  Three panels:
  - **Panel 871: Open orders by type** (timeseries, stacked,
    palette-classic). Renders
    `sum by (order_type) (paper_orders_by_type{state=~"acknowledged|partial", book_id=~"$book_id"})`
    so a sudden climb in resting LIMITs / STOPs is visible without
    hunting through individual queries. Steady state for the
    market-only alternator + SMA strategies is flat zero;
    limit-using strategies oscillate as orders rest and fill.
  - **Panel 872: Limit-fill price improvement (bps)** (heatmap on
    `paper_limit_fill_improvement_bps_bucket`). v1 sim fills at the
    limit price exactly, so the current steady state is mass at
    the 0-bps bucket; phase-14 (SchwabAdapter) or a future
    improvement model populates the right tail. Sub-zero buckets
    are a regression signal — sim must never fill worse than the
    limit demanded.
  - **Panel 873: Working orders** (Postgres-backed table). Lists
    every `paper_orders` row in `state='acknowledged'` for the
    selected `$book_id`, including `client_order_id`, `symbol`,
    `side`, `order_type`, `time_in_force`, `limit_price`,
    `stop_price`, `stop_triggered`, `quantity`, `filled_quantity`,
    `created_at`, and `NOW() - created_at AS age`. Sorted
    `created_at DESC` so the freshest are at the top; LIMIT 50.
    Drives the operator's "what's outstanding right now?" question
    without leaving the dashboard.
- **Source change** in `scripts/dashboards/internal/papertrading/`:
  new `rows/working_orders.go` registered after `rows.Orders` in
  `build.go`'s `orderedRows`. The row is 9 grid units tall (1 row
  header + 8 panel) and the three panels split the 24-wide grid
  evenly at x=0/8/16. Generated `paper-trading.json` is +166 / -14
  lines; CI's drift gate
  (`.github/workflows/validate-dashboards.yml`) is unchanged.

### Added (paper-trading-realism phase-9b — Risk row utilization + concentration panels)

- **Two new panels on the `paper-trading.json` Risk row**, both
  observability-shaped (not part of the gate-correctness path):
  - **Panel 307: Buying-power utilization per book** (timeseries,
    stacked, palette-classic, `percentunit`). Renders
    `1 - (paper_cash_usd / paper_starting_cash_usd)` per book — the
    fraction of the book's risk envelope currently in positions
    rather than parked cash. Approaching 1.0 = next BUY likely
    rejected with `reason=risk_buying_power`; persistently low =
    strategy under-decided or over-rejected upstream.
  - **Panel 308: Position concentration vs cap (per book, symbol)**
    (timeseries, stacked, palette-classic, `percentunit`). Renders
    `paper_position_qty / paper_risk_max_position_qty_per_symbol`
    per `(book, symbol)`, with the denominator filtered through
    `> 0` so books that didn't opt into the per-symbol cap don't
    render (rather than producing NaN/+Inf — dashboards shouldn't
    divide by zero). Series approaching 1.0 = next BUY of that
    symbol on that book will be rejected with
    `reason=risk_max_position`.
- **`riskHeight` 18 → 26** on `internal/papertrading/rows/risk.go`
  to fit the new sub-row at `yBase + 18`. Both panels are 12 wide ×
  8 high, side-by-side. `paper-trading.json` regenerated
  deterministically via `scripts/dashboards/build.sh`; the
  `validate-dashboards` CI gate (build twice → byte-identical, then
  drift check) passes locally.

### Added (paper-trading-realism phase-8 — Orders dashboard row)

- **New "Orders" row on `paper-trading.json`** rendering the phase-8
  order-lifecycle metrics from the trader. Three panels:
  - **Pending orders** (stat) — count of orders currently in
    `submitted | acknowledged | partial`. v1 sim collapses the full
    sequence inside one PlaceOrder so the value is typically zero;
    phase-10 limit orders will sit in `acknowledged` until the limit
    crosses the market.
  - **Rejection rate by reason** (timeseries) —
    `rate(paper_order_rejects_total[5m])` split by the bounded
    `reject_reason` enum (`stale_quote | market_closed |
    daily_loss_halt | insufficient_long | append_fill | validation`).
    Steady state flat-zero; any bar means orders the strategy fired
    could not land.
  - **State transitions** (timeseries) —
    `rate(paper_order_state_transitions_total[5m])` split by
    `(from_state → to_state)`. Healthy operation shows three
    matching curves (`→submitted`, `submitted→acknowledged`,
    `acknowledged→filled`); divergence between them flags
    validation rejecting work the strategy still thinks is
    succeeding.
- **Source-of-truth row** at
  `scripts/dashboards/internal/papertrading/rows/orders.go`; wired
  into `build.go`'s `orderedRows` between `Activity` and
  `SimulatorHealth`. Regenerated `paper-trading.json` artifact via
  `scripts/dashboards/build.sh`.

### Changed (platform-dashboards phase-4 — port `schwab-rollouts` to grafana-foundation-sdk)

- **Port `k8s/apps/schwab/go-schwab-feed/files/schwab-rollouts.json` to the SDK.** New `cmd/schwab-rollouts/` + `internal/schwabrollouts/` packages following the phase-2 schwab-feed layout (no `rows/` subdir — source has no row separators). Output path is unchanged so the existing chart's ConfigMap mount keeps working.
- **First SDK-builder usage of `state-timeline` and `barchart`.** Both reproduce the source dashboard's value-mapped color behavior. Like phase-2's piechart, every legend block requires explicit `ShowLegend(true)` — the SDK defaults `showLegend` to `false` even when `displayMode` is set, which would silently hide legends.
- **First use of `stat` panel value-mappings** (Rollout Phase, 6 mappings: Completed/Progressing/Paused/Abort/Error/Timeout) via `dashboard.ValueMap` + `MappingTypeValueToText`.
- **`$namespace` promoted from hidden constant to single-value query variable** backed by `label_values(rollout_phase, exported_namespace)`. Single-value (non-multi) keeps the dependent `$service` query (`exported_namespace="$namespace"`) sensible without rewriting it as `=~`. Aligns with the multi-tenant paper-trading direction.
- **4 deferred 7e3 palette-classic fixes shipped.** Panels 3–6 (Pod Up Status, Container Restarts, Canary vs Stable Restarts, Rollout Replicas) all multi-series via `pod` label — without palette-classic, every series collided on the same hash color.
- **Tag taxonomy cleanup.** Tags `[schwab, rollouts, canary]` → `[chalupa, platform, argo-rollouts]`. The pre-port `canary` tag was too narrow (the dashboard covers all rollout phases).
- **CI drift gate extended** to cover `schwab-rollouts.json`. `pull_request.paths`, the determinism loop (which already iterates `cmd/*/`), and the explicit `guarded` array all updated.
- **Three-render md5 stability** verified (`9f5b9ad0fa1d0f408b0431e57ec55877`). Cosmetic JSON diff is +257 lines, dominated by SDK default-emission (panel-option defaults like `repeatDirection: h`, `transparent: false`, expanded option/threshold structures).

### Changed (platform phase-9a — Schwab clients flipped from `openbao-active` to load-balanced `openbao` service)

- **Flip Vault client `addr` for `go-schwab-auth`, `go-schwab-feed`,
  `go-notify`, and the Schwab `base` chart from
  `openbao-active.openbao.svc.cluster.local` to
  `openbao.openbao.svc.cluster.local`.** The session-130 outage
  root-caused to every cluster-internal client pointing at the
  leader-only Service: when 2 of 3 OpenBao pods were sealed and the
  third couldn't claim leadership, `service_registration "kubernetes"`
  pulled `vault-active=true` off all pods, leaving `openbao-active`
  with zero endpoints and every client receiving `connect: connection
  refused`.
- **Why this is strictly safer.** The load-balanced `openbao` Service
  has a readiness-filtered selector (no `openbao-active=true`
  requirement). Standbys forward writes to the active leader (or
  return 307 if forwarding is disabled); the OpenBao Go client
  follows one redirect hop natively. During a routine leader
  election, clients now see ~1–4s of retry instead of the full
  election window of hard failure. Research backing the change at
  `docs/research/2026-05-08-openbao-client-service-choice.md`.
- **What this does NOT solve.** When ≥2 of 3 pods are sealed, the
  load-balanced Service still drops them from endpoints (readiness
  probe `bao status` exits 2 for sealed) — the surviving unsealed
  standby returns structured 503s instead of `connection refused`.
  Phase-8 (auto-unseal) eliminates the seal cascade itself; phase-9
  only narrows the leader-transition failure window. Phase-9b will
  flip ESO + Grafana OIDC + the remaining `observability/templates/*`
  client sites once 9a has baked.
- **Out of scope for this PR.** ESO `SecretStore`, Grafana OIDC
  endpoints, and the `gh-actions-telemetry` / `grafana-sa-mint` /
  `grafana-admin` external-secret + script-cm sites all still point
  at `openbao-active` and ship in phase-9b after 24h of bake time on
  9a.

### Added (platform-dashboards phase-2 — schwab-feed dashboard ported to the SDK)

- **Port `schwab-feed.json` (14 panels) from hand-written JSON to the
  grafana-foundation-sdk Go module.** New
  `scripts/dashboards/cmd/schwab-feed` binary plus
  `internal/schwabfeed/{build.go, templating.go, rows/{feed_health,
  market_data}.go}`. The committed JSON at
  `k8s/apps/schwab/go-schwab-feed/files/schwab-feed.json` is now a
  generated artifact; CI fails any PR that hand-edits it.
- **First SDK-builder usage of `bargauge` and `piechart` panel types.**
  Both reproduce the source dashboard's behaviour without
  escape-hatch overlays — neither resisted the SDK. Documented in
  `docs/dashboards-as-code.md` § Phase-7e split.
- **Generalize `scripts/dashboards/build.sh`** to iterate every binary
  in `cmd/*`, indexed by a `bin:relpath` `RENDERS` array. Onboard a
  new dashboard by adding `cmd/<name>/main.go`, one `RENDERS` entry,
  and one path to `.github/workflows/validate-dashboards.yml`'s
  `pull_request.paths` + drift loop.
- **Extend the CI drift gate** to cover `schwab-feed.json` alongside
  `paper-trading.json`. The determinism + drift checks now loop over
  every `cmd/*/` and an explicit `guarded` array.
- **Tag taxonomy cleanup.** Tags renumbered from `[schwab, feed,
  market]` → `[chalupa, schwab-feed, trading]` per the
  `briefs/dashboard-navigation-plan.md` tag-axis rule. The
  palette-classic fix shipped inline in 7e3 (panels 106 / 107) is
  preserved by the port.

### Changed (platform-dashboards phase-1a — Grafana SQLite persistence)

- **Enable PVC-backed persistence for Grafana** in `k8s/platform/observability/values.yaml`.
  Flip `victoria-metrics-k8s-stack.grafana.persistence.enabled` from `false` → `true`, 10Gi on `longhorn-single` to match the rest of the observability stack (VMSingle 20Gi, VictoriaLogs 10Gi, VictoriaTraces 8Gi, NATS JetStream 1Gi).
  ADR-008 §D6 documents the decision and why we chose PVC over Grafana Operator or external Postgres for a single-replica homelab Grafana.
- **Switch Grafana Deployment strategy from RollingUpdate to Recreate.**
  ReadWriteOnce PVC + `replicas: 1` cannot surge — the new pod would stay Pending on volume attach while the old pod still holds the PVC.
  Brief downtime during upgrade is acceptable for a single-user instance.
- **Why now.**
  Phase-1 (Image Renderer install) rolled the Grafana pod via a Helm template hash change, which wiped the SQLite DB and took the `claude-desktop` service account with it.
  The token in OpenBao then pointed at a SA-id that no longer existed; mcp-grafana's Bearer auth started 401-ing; the OIDC auto-redirect converted the 401 into HTML; mcp-grafana base64-encoded the HTML as a "PNG"; Anthropic's Vision API rejected it with HTTP 400.
  With persistence on, this class of bug goes away — and UI-saved dashboards, folders, alerts, and annotations now survive helm upgrades too.
- **Admin password drift protection unchanged.**
  `admin.existingSecret` plus `GF_SECURITY_ADMIN_*` env vars still reset the SQLite admin hash on every boot — defense in depth in case the PVC is ever recreated.
- **Out of scope for this PR.**
  Re-minting the `claude-desktop` SA into the now-persistent DB and rotating its token in OpenBao + Claude Desktop is phase-1a PR #2.
  Bumping mcp-grafana to a release with the [#745](https://github.com/grafana/mcp-grafana/issues/745) fix (visible error on non-`image/*` `/render` responses) is phase-1b PR #3.

### Added (paper-trading-realism phase-7b)

- **Paper Trading dashboard — research-grade upgrade for
  promotion-decision support.** `go-paper-trader` chart bump to
  `0.3.11`; `paper-trading.json` only (no app/image change). The
  dashboard now opens with a four-panel KPI strip (`Equity vs
  starting cash`, `Today P&L per book`, `Max drawdown (session)`,
  `Win rate per book`) before the existing Risk row, so an operator
  can answer `is this strategy ready to promote from sandbox to
  live?` without scrolling. Two new rows below Risk —
  `Strategy quality` (Sharpe / Sortino 30d-rolling annualized,
  Profit factor, Avg win / Avg loss) and `Strategy comparison`
  (Equity by strategy, Fills/hr by strategy, Slippage tax by
  strategy, Realized-P&L distribution heatmap by strategy) — surface
  every metric the sandbox-live-separation brief commits to as a
  promotion gate. A new `Rolling return %` panel at the bottom of
  the Account row chains 7d / 30d returns geometrically against the
  daily-returns SQL CTE shared with Sharpe/Sortino. The Fill realism
  row gains a fourth panel: heatmap of `paper_slippage_vs_decision_bps`
  (new metric in `go-schwab-accounts-and-trading`), measuring total
  cost of trading from signal-time including latency drift —
  distinct from the existing `Fill-price advantage lost` heatmap
  which compares pricing models. KPI stat tiles use per-tile
  thresholds (pass/fail semantics); multi-book timeseries panels use
  `palette-classic` per the multi-series coloring rule. Daily-returns
  uses query-time SQL against `paper_cash` + `paper_positions` rather
  than a Timescale continuous aggregate (paper_fills is intentionally
  NOT a hypertable; see go-schwab-accounts-and-trading CLAUDE.md
  §Safety-7). The transformation is checked in as
  `scripts/phase-7b-dashboard-upgrade.py` for traceability — the
  y-reflow logic is the kind of thing future phases would otherwise
  re-derive painfully.

### Changed (paper-trading-realism phase-7a)

- **Paper Trading dashboard — audit remediation (truthful banner,
  evergreen row names, panel descriptions, risk-scaled equity
  thresholds, continuous drawdown).** `go-paper-trader` chart
  `0.3.9 → 0.3.10`; appVersion / image unchanged
  (`paper-trading.json` only). Surfaced by an operator audit of the
  live dashboard during 2026-04-23 market open (session 116).
  - Banner replaced. Old text claimed "mid-quote ± 5bps slippage,"
    which has been wrong since phase-4 swapped to ask-on-BUY /
    bid-on-SELL with a 100ms constant submit-to-fill latency.
    New text describes the actual fill model and notes
    `ExtraSlippageBps = 0` in production.
  - Row titles dropped phase suffixes: `Risk (phase-6) — daily
    max-loss guard` → `Risk`; `Realism metrics (phase-2)` →
    `Execution quality`; `Realism metrics (phase-4)` → `Fill
    realism`; `Hygiene (phase-3)` → `Hygiene`. Internal
    construction-history notes don't belong on an operator surface.
    Panel-level `description:` strings retain `phase-N` for code
    traceability.
  - Activity row stat panels (Total fills / Buys / Sells / Notional
    traded) gained `description:` hover-text matching the
    phase-2/4/6 panels' style; previously only the realism-metrics
    panels had hover docs.
  - `Current equity by book` thresholds re-scaled. Was: red below
    $10,000, green at-or-above. New: `red <$9,000` / `orange
    $9,000–$9,500` / `transparent $9,500–$10,500` / `green
    ≥$10,500`. The neutral-band uses `transparent` so the panel
    falls back to the default Grafana panel background (no one-off
    hex). Color scale matches the per-book daily-loss-guard floor
    of $500 (5% of the $10K starting cash); a 4% slippage-tax
    bleed is no longer painted as emergency.
  - `Drawdown % by book` switched datasource from Postgres
    (`paper_cash` JOIN `paper_positions`) to VictoriaMetrics
    (`paper_equity_usd`). The old SQL went sparse because the
    snapshot tables only emit on change; the new MetricsQL query
    `100 * (paper_equity_usd - running_max(paper_equity_usd)) /
    running_max(paper_equity_usd)` is continuous because
    `paper_equity_usd` is scraped every step. Verified live.

### Added (paper-trading-realism phase-5)

- **`ddowell-sma-5x20` paper book deployed — first non-alternator
  strategy in production.** `go-paper-trader` chart bumped to
  `0.3.8`; `values.yaml` `paperTrader.books` gains a third entry
  using `pkg/strategy/sma` (already shipped + unit-tested in
  paper-trading phase-3, never deployed). Long-only,
  single-position-per-symbol, edge-triggered on sign change of
  (fast - slow); 5m / 20m windows over the full 24-symbol
  `ddowell-individual` catalog with `quantity: 1`, `startingCash:
  10000`, `session: regular`, identical `fillModel` to the
  alternator books (`extraSlippageBps: 0`, `latencyMs: 100`). Same
  appVersion (`v0.8.0`) — pure config change; ConfigMap checksum
  annotation drives the rollout. Makes the Grafana `$strategy`
  dropdown show `alternator + sma` and unlocks cross-strategy
  comparison on the shared panels. Symbol-coverage lint iterates
  books generically; no workflow changes needed.

### Changed (ai-reviews phase-56c)

- **Gemini PR-review context priming: pre-inject + tool prune +
  positive framing.** Eliminates the redundant
  `mcp_github_pull_request_read` calls phase-55 telemetry flagged
  (~28K input tokens/review at the 4× cached-rate multiplier,
  ~$50/yr at current volume).
  - `compose_context` job gains a new `Fetch PR metadata for
    pre-inject` step that runs `gh pr view --json title,body,author,
    state,labels,baseRefName,headRefName,createdAt,files` and formats
    the result to `/tmp/pr_metadata.md` (body truncated at 10KB).
    The compose step emits that block before the diff block so the
    review model sees metadata → diff → rubric before reasoning.
  - `.github/commands/gemini-review.toml` Input Data section rewrites
    the three `pull_request_read.*` bullet points as "use the
    Additional Context block for all PR data; it is complete and
    canonical — do not attempt to re-fetch." Positive-framing
    language per Google's Gemini 3 prompting guide (blanket negatives
    are documented to drop or over-index).
  - `includeTools` drops `pull_request_read`, keeping only
    `add_comment_to_pending_review` and `pull_request_review_write`
    (the posting surface).
  - Rationale, citations, and supersede criteria:
    `docs/research/2026-04-22-pr-review-context-priming.md` in
    chalupa-brain. Phase-56a reverted the tool-prune-alone attempt
    after shell-fallback flailing (`gh pr view`, `curl api.github.com`,
    `cat $GITHUB_EVENT_PATH`); phase-56c coordinates the three changes
    so the model has no informational reason to probe for PR data.

### Changed

- **paper-trading market-hours gate + spread histogram —
  paper-trading-realism phase-2.** Paired with
  `go-schwab-accounts-and-trading` v0.6.0 (adds configurable session
  window + `paper_market_closed_rejects_total` +
  `paper_fill_spread_bps` histogram). `go-paper-trader` chart
  bumped to 0.3.5 + appVersion 0.6.0 + image tag v0.6.0. Values
  changes: every book in `paperTrader.books[]` gains
  `session: regular` (09:30-16:00 ET M-F ex US holidays). Rationale:
  in the 17.5h post-close window on 2026-04-20 the 30s-cadence
  alternator fired 384 off-hours fills, biasing the 30s-vs-60s
  cadence comparison against its 60s sibling.
  - ConfigMap template (`templates/configmap.yaml`) renders
    `session: "<value>"` when set; omits the line otherwise
    (binary falls back to `regular` inside `sim.ParseSession`).
  - Dashboard adds a new "Realism metrics (phase-2)" row with
    three panels in `files/paper-trading.json`:
    - Panel 21 — `rate(paper_market_closed_rejects_total[5m])` by
      book/strategy; non-zero during market hours flags a stale
      holiday calendar or a clock-skew pod.
    - Panel 22 — heatmap of `paper_fill_spread_bps_bucket` rate
      across all symbols, log Y-axis for resolution at both tight
      (1bps) and wide (1000bps) ends.
    - Panel 23 — P50/P95/P99 by book from the same histogram over
      a 15m window; drives phase-4's mid-vs-ask/bid fill-model
      decision.

- **paper-trading dashboard truth-up — paper-trading-realism
  phase-1.** Paired with `go-schwab-accounts-and-trading` v0.5.0
  (adds `book_id` label to every per-book `paper_*` Prom metric).
  `go-paper-trader` chart bumped to 0.3.4 + appVersion 0.5.0 +
  image tag v0.5.0. Dashboard edits in `k8s/apps/schwab/go-paper-
  trader/files/paper-trading.json`:
  - Panel 4 (`Current equity`) and panel 5 (`Open positions`)
    renamed to `by book`, re-scoped to
    `paper_equity_usd{book_id=~"$book_id"}` and
    `paper_open_positions{book_id=~"$book_id"}` with
    `legendFormat: "{{book_id}}"`. Both pods previously aggregated
    silently — the stat disagreed with the per-book SQL panels.
  - Panel 8 (`Live positions`) SQL gains `AND quantity != 0` so
    closed positions no longer linger in the table.
  - Panel 15 (`By-strategy notional`) SQL gains
    `AND book_id IN ($book_id)` so the `$book_id` dropdown
    actually scopes the bar chart.
  - `$book_id` template variable query constrained to the last
    7 days (`WHERE time > NOW() - INTERVAL '7 days'`) so orphan
    `book_id`s from long-retired deployments stop appearing in
    the dropdown.

- **Gemini review cost gates — phase-56 Tier 1** (new `filter_review`
  job in `gemini-dispatch.yml`). Auto-triggered PR reviews
  (`opened` / `synchronize`) now skip when the PR is draft, under
  20 lines, or touches only exempt paths (CHANGELOG, docs, lock
  files, generated code). New `skip-review` and `force-review`
  labels let operators override; `@gemini-cli /review` comments
  bypass all gates. Expected volume reduction ~30%, moving projected
  monthly spend from ~$35 toward ~$24. Labels and gate behavior
  documented in new `CONTRIBUTING.md`.
  - `pull_request_read` remains in the GitHub MCP server allowlist.
    Phase-56 initially pruned it to eliminate phase-55's observed
    redundant fetches (~28K redundant fresh-input tokens, ~$0.006
    wasted/review). Live validation on this PR showed Gemini
    falling back to `gh pr view` (fails — no shell auth) and
    `cat $GITHUB_EVENT_PATH` (hung 5min on empty-var expansion) or
    running up to 40 turns of shell/curl/git probing — net more
    expensive than the redundant MCP call. Eliminating the
    redundancy properly requires a coordinated prompt rewrite
    (`.github/commands/gemini-review.toml` directs Gemini to call
    `pull_request_read.{get,get_files,get_diff}` — the directive
    has to move with the tool); tracked as phase-56c.
- **Gen-AI metrics use delta temporality — queries switch from
  `rate()` to `sum_over_time()`** (phase-55, Gemini PR #403 review
  catch). The stateless GitHub Actions ingester emits per-run
  totals, not cumulative counters. `rate()`/`increase()` interpret
  each smaller sample as a counter reset and undercount by ~50%.
  Dashboard and recording rules now use `sum_over_time()` for
  window-aggregate math, and per-run cost uses the raw sample
  (one dot per review).
- **Cost-per-review panel restored with delta-temp math + $1 cap**
  (phase-55). Replaced the intermediate "Tokens per API call" pivot
  with a bounded per-run cost scatter: `min: 0`, `max: 1.00`,
  thresholds `$0.25` (yellow) / `$0.50` (red). Worst-case pricing
  (`gemini-2.5-pro-long` on a pathological review) tops out
  ~$3.50; any single review at $0.50 is already "investigate."
  Pre-fix the panel auto-scaled to $100 — two orders of magnitude
  past reality.
- **Gen-AI metrics reshape to OTel SemConv Histogram** (phase-55).
  Pre-phase-55 emitted `gen_ai_client_token_usage_total` as a counter
  with absolute-per-run values and a unique `github_run_id` label on
  every sample. This broke `rate()` (one sample per series), broke
  `increase(...[30d])` in the `:monthly` / `:per_run` recording rules
  (returning 0 for every run — verified 10/10 over 30d pre-fix), and
  grew series cardinality unbounded. Phase-55 emits
  `gen_ai_client_token_usage_{bucket,sum,count}` as a **Histogram**
  with OTel-mandated token-count boundaries, and drops `github_run_id`
  from both token and duration metrics. Per-run drill-down is deferred
  to phase-44's VictoriaLogs event fan-out.
  - `scripts/ingest-gemini-telemetry.py build_samples()` — emits
    histogram observations (one per api_response event × token_type)
    instead of per-run absolute-total counter samples. Docstring
    notes delta temporality explicitly.
  - `k8s/platform/observability/templates/gen-ai-pricing-rules.yaml`
    — `:monthly` aggregates with `sum_over_time(_sum[30d])` (see
    first bullet for temporality rationale); `:per_run` retired (no
    successor in this phase); `:age_days` retired (never produced a
    sample, same-group cross-rule dependency bug).
  - `k8s/platform/observability/templates/gen-ai-alert-rules.yaml` —
    `GeminiPricingTableStale` now computes age inline
    (`(time() - gen_ai_pricing_table_as_of_timestamp) / 86400`)
    instead of consuming the retired `:age_days` recording rule.
  - `k8s/platform/observability/files/gemini-review-spend.json` —
    panel 2 (Tokens) uses `sum_over_time(_sum[$__rate_interval])`;
    panel 3 (Latency p50/p95) uses
    `histogram_quantile(sum by(le)(sum_over_time(_bucket[...])))`;
    panel 4 (Cost per review) stays as USD, one dot per run with
    `min: 0 / max: 1.00` and yellow/red thresholds at $0.25/$0.50;
    panel 5 inlines the pricing-table age expression. Template
    variables switch from `label_values(..._total, ...)` to
    `label_values(..._sum, ...)`.
- **Review workflow ceilings aligned** (phase-55). In
  `.github/workflows/gemini-review.yml`: `timeout_minutes: 4 → 8`
  (covers observed 6min P95 wallclock with ~2min headroom);
  `maxSessionTurns: 75` kept (observed max 69 turns over 30d, ~6
  turns of headroom). Pairing rationale: at observed ~5s/turn, 75
  turns × 5s = 375s, under the new 480s timeout. Accepts Gemini's
  PR #402 🟡 inline suggestion.

### Added

- **Artifact-based ingest of Gemini review telemetry to VictoriaMetrics**
  (phase-51c). Replaces the Alloy OTLP sidecar path (never actually
  delivered metrics — see phase-51 retro) with a post-review parse of
  `.gemini/telemetry.log` and POST to VM `/api/v1/import/prometheus`.
  Emits `gen_ai_client_token_usage_total` (counter, per token type) and
  `gen_ai_client_operation_duration_seconds_{bucket,sum,count}`
  (histogram) with the labels the phase-39c recording rules expect
  (`gen_ai_system`, `gen_ai_request_model`, `gen_ai_token_type`,
  `github_repository`, `github_run_id`). Runs with `if: always()` so
  `FatalTurnLimitedError` runs still emit tokens (cost observability
  does not depend on review success). Alloy sidecar retirement is a
  follow-up PR (phase-51c AI3).
  - `scripts/ingest-gemini-telemetry.py` — parser + pusher.
  - `.github/workflows/gemini-cli-reusable.yml` — new
    `Ingest telemetry to VictoriaMetrics` step before the artifact
    upload, gated on `SIDECAR_STARTED` (piggybacks on the sidecar's
    Tailscale join).
  - `k8s/platform/observability/templates/vm-write-ingress.yaml` —
    second Traefik route adds `POST /api/v1/import/prometheus` under
    the existing `gh-actions-telemetry-auth` middleware.

### Fixed

- **Gemini reviews all failing with `Invalid telemetry target: otlp`** (phase-51 PR 1).
  `gemini-cli-reusable.yml` was rewriting `settings.telemetry.target` to
  `"otlp"` when the Alloy sidecar came up, but Gemini CLI v0.38.2's
  `TelemetryTarget` enum is `{local, gcp, genkit}` — `otlp` was never
  valid, and upstream PR #22282 (shipped in v0.38.x) tightened
  validation so `settings.json` with `target: "otlp"` now fails loudly
  at load. OTLP export is layered on top of `target=local` via
  `useCollector: true` + `otlpEndpoint`, not a separate target.
  PR 1 normalizes to `target=local` + strips OTLP fields universally,
  so reviews run green again against the `.gemini/telemetry.log`
  outfile (file-based telemetry). Restoring live OTLP export to the
  Alloy sidecar is deferred to phase-51b / PR 2. Requires
  `vars.GEMINI_CLI_VERSION=0.38.2` pinned at the org level to prevent
  implicit `'latest'` drift. (`.github/workflows/gemini-cli-reusable.yml`)

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
