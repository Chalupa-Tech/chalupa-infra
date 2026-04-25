#!/usr/bin/env python3
"""Apply paper-trading-realism phase-7b dashboard upgrade.

Idempotent in spirit but not in fact: rerunning will duplicate panels.
Run once, commit the JSON. The script is checked in for two reasons:
(1) future readers can see the structural intent at a glance, and
(2) the y-reflow logic is the kind of thing that's painful to re-derive
when phase-7c wants to insert another row.
"""
from __future__ import annotations

import json
from collections import OrderedDict
from copy import deepcopy
from pathlib import Path

DASHBOARD = Path(__file__).resolve().parent.parent / "k8s/apps/schwab/go-paper-trader/files/paper-trading.json"

VICTORIA = {"type": "prometheus", "uid": "VictoriaMetrics"}
TIMESCALE = {"type": "grafana-postgresql-datasource", "uid": "timescaledb"}

# ---------------------------------------------------------------------------
# Panel builders. IDs start at 100 to avoid collision with existing 1..34
# (and the duplicate id=20 the dashboard already carries).
# ---------------------------------------------------------------------------


def row(panel_id: int, title: str) -> dict:
    return {
        "id": panel_id,
        "type": "row",
        "title": title,
        "gridPos": {"h": 1, "w": 24, "x": 0, "y": 0},
        "collapsed": False,
    }


def kpi_stat_prom(panel_id, title, description, expr, unit, thresholds, x):
    return {
        "id": panel_id,
        "type": "stat",
        "title": title,
        "description": description,
        "gridPos": {"h": 4, "w": 6, "x": x, "y": 0},
        "datasource": VICTORIA,
        "targets": [
            {"refId": "A", "expr": expr, "legendFormat": "{{book_id}}", "instant": True}
        ],
        "fieldConfig": {
            "defaults": {
                "unit": unit,
                "thresholds": {"mode": "absolute", "steps": thresholds},
            },
            "overrides": [],
        },
        "options": {
            "colorMode": "background",
            "graphMode": "none",
            "reduceOptions": {"calcs": ["lastNotNull"], "fields": "", "values": False},
        },
    }


def kpi_stat_sql(panel_id, title, description, sql, unit, thresholds, x):
    return {
        "id": panel_id,
        "type": "stat",
        "title": title,
        "description": description,
        "gridPos": {"h": 4, "w": 6, "x": x, "y": 0},
        "datasource": TIMESCALE,
        "targets": [{"refId": "A", "format": "table", "rawSql": sql}],
        "fieldConfig": {
            "defaults": {
                "unit": unit,
                "thresholds": {"mode": "absolute", "steps": thresholds},
            },
            "overrides": [],
        },
        "options": {
            "colorMode": "background",
            "graphMode": "none",
            "reduceOptions": {"calcs": ["lastNotNull"], "fields": "", "values": False},
        },
    }


def timeseries_sql(panel_id, title, description, sql, unit, x, y_off, w, h):
    return {
        "id": panel_id,
        "type": "timeseries",
        "title": title,
        "description": description,
        "gridPos": {"h": h, "w": w, "x": x, "y": y_off},
        "datasource": TIMESCALE,
        "targets": [{"refId": "A", "format": "time_series", "rawSql": sql}],
        "fieldConfig": {
            "defaults": {
                "unit": unit,
                "custom": {"lineWidth": 2, "fillOpacity": 10},
                "color": {"mode": "palette-classic"},
            },
            "overrides": [],
        },
        "options": {
            "legend": {"displayMode": "list", "placement": "bottom"},
        },
    }


def timeseries_prom(panel_id, title, description, expr, legend, unit, x, y_off, w, h):
    return {
        "id": panel_id,
        "type": "timeseries",
        "title": title,
        "description": description,
        "gridPos": {"h": h, "w": w, "x": x, "y": y_off},
        "datasource": VICTORIA,
        "targets": [{"refId": "A", "expr": expr, "legendFormat": legend}],
        "fieldConfig": {
            "defaults": {
                "unit": unit,
                "custom": {"lineWidth": 2, "fillOpacity": 10},
                "color": {"mode": "palette-classic"},
            },
            "overrides": [],
        },
        "options": {"legend": {"displayMode": "list", "placement": "bottom"}},
    }


def stat_sql_thresholded(panel_id, title, description, sql, unit, thresholds, x, y_off, w, h):
    return {
        "id": panel_id,
        "type": "stat",
        "title": title,
        "description": description,
        "gridPos": {"h": h, "w": w, "x": x, "y": y_off},
        "datasource": TIMESCALE,
        "targets": [{"refId": "A", "format": "table", "rawSql": sql}],
        "fieldConfig": {
            "defaults": {
                "unit": unit,
                "thresholds": {"mode": "absolute", "steps": thresholds},
            },
            "overrides": [],
        },
        "options": {
            "colorMode": "background",
            "graphMode": "none",
            "reduceOptions": {"calcs": ["lastNotNull"], "fields": "", "values": False},
        },
    }


def heatmap_prom(panel_id, title, description, expr, x, y_off, w, h):
    return {
        "id": panel_id,
        "type": "heatmap",
        "title": title,
        "description": description,
        "gridPos": {"h": h, "w": w, "x": x, "y": y_off},
        "datasource": VICTORIA,
        "targets": [
            {
                "refId": "A",
                "expr": expr,
                "format": "heatmap",
                "legendFormat": "{{le}}",
            }
        ],
        "fieldConfig": {
            "defaults": {"unit": "short", "custom": {"scaleDistribution": {"type": "linear"}}},
            "overrides": [],
        },
        "options": {
            "calculate": False,
            "yAxis": {"unit": "short"},
            "color": {"scheme": "Spectral", "mode": "scheme"},
        },
    }


def heatmap_sql(panel_id, title, description, sql, x, y_off, w, h):
    return {
        "id": panel_id,
        "type": "heatmap",
        "title": title,
        "description": description,
        "gridPos": {"h": h, "w": w, "x": x, "y": y_off},
        "datasource": TIMESCALE,
        "targets": [{"refId": "A", "format": "time_series", "rawSql": sql}],
        "fieldConfig": {
            "defaults": {"unit": "short"},
            "overrides": [],
        },
        "options": {
            "calculate": True,
            "calculation": {"yBuckets": {"mode": "count", "value": "20"}},
            "yAxis": {"unit": "currencyUSD"},
            "color": {"scheme": "Spectral", "mode": "scheme"},
        },
    }


def barchart_prom(panel_id, title, description, expr, legend, x, y_off, w, h):
    return {
        "id": panel_id,
        "type": "barchart",
        "title": title,
        "description": description,
        "gridPos": {"h": h, "w": w, "x": x, "y": y_off},
        "datasource": VICTORIA,
        "targets": [{"refId": "A", "expr": expr, "legendFormat": legend}],
        "fieldConfig": {
            "defaults": {
                "unit": "short",
                "color": {"mode": "palette-classic"},
            },
            "overrides": [],
        },
        "options": {"legend": {"displayMode": "list", "placement": "bottom"}},
    }


# ---------------------------------------------------------------------------
# Build new sections.
# ---------------------------------------------------------------------------


def kpi_strip_section() -> list[dict]:
    """Phase-7b summary KPI strip — 4 stat panels at top of dashboard."""
    panels = [row(100, "Summary (at-a-glance)")]

    # 1. Equity vs starting cash (%). Hardcodes $10k starting cash —
    # matches values.yaml; revisit when per-book starting cash diverges.
    panels.append(
        kpi_stat_prom(
            101,
            "Equity vs starting cash",
            "paper-trading-realism phase-7b: (paper_equity_usd / 10000) − 1. Hardcodes $10k starting cash, which matches the per-book values.yaml today; if startingCash ever varies per book this becomes wrong silently and we'll need a paper_starting_cash_usd metric. Green ≥ +1% (book is compounding); neutral ±1% (noise floor); amber −1% to −5%; red < −5%.",
            'paper_equity_usd{book_id=~"$book_id"} / 10000 - 1',
            "percentunit",
            [
                {"color": "red", "value": None},
                {"color": "orange", "value": -0.05},
                {"color": "transparent", "value": -0.01},
                {"color": "green", "value": 0.01},
            ],
            x=0,
        )
    )

    # 2. Today P&L (USD). Surfaces the same paper_daily_pnl_usd that
    # the Risk row already plots, but as a stat-tile glance signal.
    panels.append(
        kpi_stat_prom(
            102,
            "Today P&L per book",
            "paper-trading-realism phase-7b: paper_daily_pnl_usd (UTC day, realized + unrealized). Same series as the Risk row's timeseries, surfaced here for at-a-glance health. Resets at UTC midnight.",
            'paper_daily_pnl_usd{book_id=~"$book_id"}',
            "currencyUSD",
            [
                {"color": "red", "value": None},
                {"color": "transparent", "value": 0},
                {"color": "green", "value": 0.01},
            ],
            x=6,
        )
    )

    # 3. Max drawdown (session). SQL on paper_cash + paper_positions
    # so the result spans the whole book history, not just the
    # dashboard's time-range. Promotion threshold is ≤ 3%, so amber
    # at −1% / red at −3% match that gate (per
    # workstreams/paper-trading/briefs/sandbox-live-separation.md).
    panels.append(
        kpi_stat_sql(
            103,
            "Max drawdown (session)",
            "paper-trading-realism phase-7b: maximum peak-to-trough drawdown observed for each book since first cash snapshot (whole session, not just visible range). Computed in TimescaleDB so the value survives Prom retention; matches the promotion-threshold convention (≤ 3%) from sandbox-live-separation brief — amber at −1% (worth watching), red at −3% (gates promotion).",
            (
                "WITH equity_ts AS ("
                "  SELECT pc.time, pc.book_id, pc.cash + COALESCE(pos.equity, 0) AS equity"
                "  FROM paper_cash pc"
                "  LEFT JOIN ("
                "    SELECT time, book_id, SUM(quantity * mark_price) AS equity"
                "    FROM paper_positions GROUP BY time, book_id"
                "  ) pos ON pc.time = pos.time AND pc.book_id = pos.book_id"
                "  WHERE pc.book_id IN ($book_id)"
                "), with_peak AS ("
                "  SELECT book_id, time, equity,"
                "         MAX(equity) OVER (PARTITION BY book_id ORDER BY time"
                "                           ROWS BETWEEN UNBOUNDED PRECEDING AND CURRENT ROW) AS peak"
                "  FROM equity_ts"
                ") "
                "SELECT book_id, MIN((equity - peak) / NULLIF(peak, 0)) AS max_dd "
                "FROM with_peak GROUP BY book_id"
            ),
            "percentunit",
            [
                {"color": "red", "value": None},
                {"color": "orange", "value": -0.03},
                {"color": "transparent", "value": -0.01},
                {"color": "green", "value": -0.001},
            ],
            x=12,
        )
    )

    # 4. Win rate (%). SQL on paper_fills SELLs only (BUYs have
    # null realized_pl). Promotion is comfortable at ≥ 50% (positive
    # return-day rule from the brief implies a similar win/lose
    # cadence); amber 45-55%, red < 45%.
    panels.append(
        kpi_stat_sql(
            104,
            "Win rate per book",
            "paper-trading-realism phase-7b: COUNT(SELL fills with realized_pl > 0) / COUNT(SELL fills) — fraction of trades that closed at a profit, scoped to selected books. Promotion-relevant (the brief's positive-return-days rule implies a comparable win/lose cadence). Green ≥ 55%; amber 45–55%; red < 45%.",
            (
                "SELECT book_id, "
                "  COUNT(*) FILTER (WHERE realized_pl > 0)::float / NULLIF(COUNT(*), 0) AS win_rate "
                "FROM paper_fills "
                "WHERE side = 'SELL' AND realized_pl IS NOT NULL "
                "  AND book_id IN ($book_id) AND $__timeFilter(time) "
                "GROUP BY book_id"
            ),
            "percentunit",
            [
                {"color": "red", "value": None},
                {"color": "orange", "value": 0.45},
                {"color": "transparent", "value": 0.50},
                {"color": "green", "value": 0.55},
            ],
            x=18,
        )
    )

    return panels


# Daily-returns CTE (D1 decision: query-time SQL, no continuous aggregate).
# Reused across Sharpe / Sortino / rolling-returns. Centralised here so
# tweaks (e.g. timezone, sample rate) propagate without 4-place edits.
DAILY_RETURNS_CTE = (
    "WITH equity_ts AS ("
    "  SELECT pc.time, pc.book_id, pc.cash + COALESCE(pos.equity, 0) AS equity"
    "  FROM paper_cash pc"
    "  LEFT JOIN ("
    "    SELECT time, book_id, SUM(quantity * mark_price) AS equity"
    "    FROM paper_positions GROUP BY time, book_id"
    "  ) pos ON pc.time = pos.time AND pc.book_id = pos.book_id"
    "  WHERE pc.book_id IN ($book_id)"
    "), daily AS ("
    "  SELECT"
    "    book_id,"
    "    time_bucket('1 day', time) AS trade_date,"
    "    FIRST(equity, time) AS sod,"
    "    LAST(equity, time)  AS eod"
    "  FROM equity_ts"
    "  GROUP BY book_id, trade_date"
    "), returns AS ("
    "  SELECT book_id, trade_date, (eod - sod) / NULLIF(sod, 0) AS ret"
    "  FROM daily"
    ") "
)


def strategy_quality_section() -> list[dict]:
    """Phase-7b strategy-quality row — Sharpe, Sortino, profit factor, avg win/loss."""
    panels = [row(110, "Strategy quality")]

    # Sharpe (30d rolling, annualized). Risk-free rate = 0 per D4
    # (matches phase-6 daily-P&L convention; revisit when promotion
    # is real).
    sharpe_sql = (
        DAILY_RETURNS_CTE
        + "SELECT trade_date AS time, book_id AS metric, "
        "  AVG(ret) OVER w / NULLIF(STDDEV_SAMP(ret) OVER w, 0) * SQRT(252) AS sharpe "
        "FROM returns "
        "WINDOW w AS (PARTITION BY book_id ORDER BY trade_date "
        "             ROWS BETWEEN 29 PRECEDING AND CURRENT ROW) "
        "ORDER BY trade_date"
    )
    panels.append(
        timeseries_sql(
            111,
            "Sharpe ratio (30d rolling, annualized, rf=0)",
            "paper-trading-realism phase-7b: 30-day rolling Sharpe = mean(daily_return) / stddev(daily_return) × √252. Risk-free rate hardcoded to 0 (matches phase-6 UTC daily-P&L convention; revisit when a strategy is a real promotion candidate). Promotion threshold ≥ 1.0 per sandbox-live-separation brief. Renders NULL for books with < 30 days of returns.",
            sharpe_sql,
            "short",
            x=0,
            y_off=0,
            w=12,
            h=8,
        )
    )

    # Sortino (30d rolling, annualized). Same as Sharpe but only
    # negative returns in the denominator — answers "are the
    # drawdowns concentrated or spread across days?"
    sortino_sql = (
        DAILY_RETURNS_CTE
        + "SELECT trade_date AS time, book_id AS metric, "
        "  AVG(ret) OVER w / NULLIF(STDDEV_SAMP(ret) FILTER (WHERE ret < 0) OVER w, 0) * SQRT(252) AS sortino "
        "FROM returns "
        "WINDOW w AS (PARTITION BY book_id ORDER BY trade_date "
        "             ROWS BETWEEN 29 PRECEDING AND CURRENT ROW) "
        "ORDER BY trade_date"
    )
    panels.append(
        timeseries_sql(
            112,
            "Sortino ratio (30d rolling, annualized, rf=0)",
            "paper-trading-realism phase-7b: 30-day rolling Sortino = mean(daily_return) / stddev(daily_return WHERE ret<0) × √252. Same numerator as Sharpe; denominator only counts losing days, so Sortino > Sharpe means losses are concentrated (good). Renders NULL for books with no losing days in the window or < 30 days of returns.",
            sortino_sql,
            "short",
            x=12,
            y_off=0,
            w=12,
            h=8,
        )
    )

    # Profit factor (whole time-range, scoped to $book_id). > 1.2
    # is decent, > 2.0 is strong.
    pf_sql = (
        "SELECT book_id, "
        "  SUM(realized_pl) FILTER (WHERE realized_pl > 0) / "
        "  NULLIF(ABS(SUM(realized_pl) FILTER (WHERE realized_pl < 0)), 0) AS profit_factor "
        "FROM paper_fills "
        "WHERE realized_pl IS NOT NULL AND book_id IN ($book_id) AND $__timeFilter(time) "
        "GROUP BY book_id"
    )
    panels.append(
        stat_sql_thresholded(
            113,
            "Profit factor",
            "paper-trading-realism phase-7b: SUM(winning realized_pl) / |SUM(losing realized_pl)|, per book, over the visible time range. > 1.2 decent; > 2.0 strong (per sandbox-live-separation brief — promotion threshold is 1.2). Renders NULL for books with no losing trades in the window.",
            pf_sql,
            "short",
            [
                {"color": "red", "value": None},
                {"color": "orange", "value": 1.0},
                {"color": "transparent", "value": 1.2},
                {"color": "green", "value": 2.0},
            ],
            x=0,
            y_off=8,
            w=12,
            h=4,
        )
    )

    # Avg win / Avg loss — paired timeseries per book showing the
    # fat-tail shape. Bucketed by trade date so the panel reflects
    # how shape evolves over the time range.
    avg_winloss_sql = (
        "SELECT "
        "  time_bucket('1 day', time) AS time, "
        "  book_id || '-win'  AS metric, "
        "  AVG(realized_pl) FILTER (WHERE realized_pl > 0) AS v "
        "FROM paper_fills WHERE realized_pl IS NOT NULL AND book_id IN ($book_id) AND $__timeFilter(time) "
        "GROUP BY 1, book_id "
        "UNION ALL "
        "SELECT "
        "  time_bucket('1 day', time) AS time, "
        "  book_id || '-loss' AS metric, "
        "  AVG(realized_pl) FILTER (WHERE realized_pl < 0) AS v "
        "FROM paper_fills WHERE realized_pl IS NOT NULL AND book_id IN ($book_id) AND $__timeFilter(time) "
        "GROUP BY 1, book_id "
        "ORDER BY time"
    )
    panels.append(
        timeseries_sql(
            114,
            "Avg win / Avg loss (per day)",
            "paper-trading-realism phase-7b: per-book daily mean realized_pl on winning trades (positive series) vs losing trades (negative series). Captures fat-tail shape — even a 55%-win-rate strategy bleeds if avg loss > avg win in absolute terms. Series suffix `-win` / `-loss` so multi-book comparisons stay unambiguous.",
            avg_winloss_sql,
            "currencyUSD",
            x=12,
            y_off=8,
            w=12,
            h=4,
        )
    )

    return panels


def strategy_comparison_section() -> list[dict]:
    """Phase-7b strategy-comparison row — alternator vs sma side-by-side."""
    panels = [row(120, "Strategy comparison")]

    # Equity by strategy: sum of book equity per strategy. Strategy
    # joins via paper_fills.strategy → book_id mapping (latest-known
    # mapping per book; books rarely re-strategy in v1).
    eq_by_strat_sql = (
        "WITH equity_ts AS ("
        "  SELECT pc.time, pc.book_id, pc.cash + COALESCE(pos.equity, 0) AS equity"
        "  FROM paper_cash pc"
        "  LEFT JOIN ("
        "    SELECT time, book_id, SUM(quantity * mark_price) AS equity"
        "    FROM paper_positions GROUP BY time, book_id"
        "  ) pos ON pc.time = pos.time AND pc.book_id = pos.book_id"
        "  WHERE $__timeFilter(pc.time)"
        "), book_strategy AS ("
        "  SELECT DISTINCT ON (book_id) book_id, strategy"
        "  FROM paper_fills ORDER BY book_id, time DESC"
        ") "
        "SELECT eq.time AS time, bs.strategy AS metric, SUM(eq.equity) AS equity "
        "FROM equity_ts eq JOIN book_strategy bs ON eq.book_id = bs.book_id "
        "WHERE bs.strategy IN ($strategy) "
        "GROUP BY eq.time, bs.strategy "
        "ORDER BY eq.time"
    )
    panels.append(
        timeseries_sql(
            121,
            "Equity by strategy",
            "paper-trading-realism phase-7b: SUM(equity) of every book running each strategy, summed across books of the same strategy. Answers `which strategy has the books up?`. Strategy mapping uses each book's most-recent paper_fills.strategy (books don't re-strategy in v1).",
            eq_by_strat_sql,
            "currencyUSD",
            x=0,
            y_off=0,
            w=12,
            h=8,
        )
    )

    # Fills/hr by strategy via Prom — paper_fills_total carries
    # `strategy` as a label.
    panels.append(
        barchart_prom(
            122,
            "Fills per hour by strategy",
            "paper-trading-realism phase-7b: rate(paper_fills_total[1h]) × 3600 summed by strategy. Reveals cadence differences between strategies — alternator-30s should be ~120/hr per book, sma 1m crossover should be much sparser.",
            'sum by (strategy) (rate(paper_fills_total{strategy=~"$strategy"}[1h])) * 3600',
            "{{strategy}}",
            x=12,
            y_off=0,
            w=12,
            h=8,
        )
    )

    # Slippage tax by strategy — paper_fills carries no mark-at-fill
    # column today, so we approximate execution cost via deviation from
    # the symbol's daily VWAP: |price − daily_VWAP| × quantity, summed
    # per strategy. Risk #2 in the phase-7b plan documents the
    # bias (zero for symbols with few fills/day, since VWAP collapses
    # to the fill price). For comparing strategies the sign is right
    # even if the absolute value is loose.
    slip_tax_sql = (
        "WITH vwap AS ("
        "  SELECT book_id, symbol, "
        "    time_bucket('1 day', time) AS trade_date, "
        "    SUM(price * quantity) / NULLIF(SUM(quantity), 0) AS vwap_px "
        "  FROM paper_fills "
        "  WHERE $__timeFilter(time) AND book_id IN ($book_id) "
        "  GROUP BY book_id, symbol, trade_date"
        ") "
        "SELECT bs.strategy AS metric, "
        "  SUM(ABS(pf.price - v.vwap_px) * pf.quantity) AS slippage_usd "
        "FROM paper_fills pf "
        "JOIN vwap v ON pf.book_id = v.book_id AND pf.symbol = v.symbol "
        "  AND time_bucket('1 day', pf.time) = v.trade_date "
        "JOIN ("
        "  SELECT DISTINCT ON (book_id) book_id, strategy"
        "  FROM paper_fills ORDER BY book_id, time DESC"
        ") bs ON pf.book_id = bs.book_id "
        "WHERE $__timeFilter(pf.time) AND pf.book_id IN ($book_id) "
        "GROUP BY bs.strategy"
    )
    panels.append(
        stat_sql_thresholded(
            123,
            "Slippage tax by strategy (USD)",
            "paper-trading-realism phase-7b: |fill_price − daily_VWAP_for_book_symbol| × quantity, summed per strategy. Approximates execution cost (no mark-at-fill column on paper_fills today; the daily-VWAP proxy is biased toward zero for symbols with few fills/day, so treat absolute values comparatively, not as ground truth). Expect alternator-30s to dominate cumulative tax purely on volume.",
            slip_tax_sql,
            "currencyUSD",
            [{"color": "transparent", "value": None}],
            x=0,
            y_off=8,
            w=12,
            h=4,
        )
    )

    # Win-distribution heatmap by strategy — realized_pl bucketed
    # by strategy.
    win_dist_sql = (
        "SELECT pf.time AS time, bs.strategy AS metric, pf.realized_pl AS value "
        "FROM paper_fills pf "
        "JOIN ("
        "  SELECT DISTINCT ON (book_id) book_id, strategy"
        "  FROM paper_fills ORDER BY book_id, time DESC"
        ") bs ON pf.book_id = bs.book_id "
        "WHERE pf.realized_pl IS NOT NULL AND $__timeFilter(pf.time) "
        "  AND pf.book_id IN ($book_id) AND bs.strategy IN ($strategy) "
        "ORDER BY pf.time"
    )
    panels.append(
        heatmap_sql(
            124,
            "Realized P&L distribution by strategy",
            "paper-trading-realism phase-7b: realized_pl bucketed per strategy. Fat right tail = healthy (a few big wins carry the curve); fat left tail = concerning. Compare alternator vs sma shape, not absolute counts.",
            win_dist_sql,
            x=12,
            y_off=8,
            w=12,
            h=4,
        )
    )

    return panels


def rolling_returns_panel() -> dict:
    """Phase-7b rolling-returns timeseries — 7d / 30d / QTD per book."""
    sql = (
        DAILY_RETURNS_CTE
        + "SELECT trade_date AS time, book_id || '-7d'  AS metric, "
        "  EXP(SUM(LN(1 + COALESCE(ret, 0))) OVER w7)  - 1 AS r FROM returns "
        "WINDOW w7  AS (PARTITION BY book_id ORDER BY trade_date "
        "               ROWS BETWEEN 6  PRECEDING AND CURRENT ROW) "
        "UNION ALL "
        + DAILY_RETURNS_CTE
        + "SELECT trade_date AS time, book_id || '-30d' AS metric, "
        "  EXP(SUM(LN(1 + COALESCE(ret, 0))) OVER w30) - 1 AS r FROM returns "
        "WINDOW w30 AS (PARTITION BY book_id ORDER BY trade_date "
        "               ROWS BETWEEN 29 PRECEDING AND CURRENT ROW) "
        "ORDER BY time"
    )
    return timeseries_sql(
        130,
        "Rolling return % by book (7d / 30d)",
        "paper-trading-realism phase-7b: compounded return over rolling windows. Series suffix `-7d` / `-30d`. Computed via daily geometric chaining over the daily-returns CTE. QTD intentionally omitted from v1 — a clean QTD requires a session-aware date-trunc that is not yet load-bearing; pencil it in for phase-7c if/when promotion windows shorten.",
        sql,
        "percentunit",
        x=0,
        y_off=0,
        w=24,
        h=8,
    )


def slippage_heatmap_panel() -> dict:
    """Phase-7b slippage-vs-decision heatmap — Fill realism row."""
    return heatmap_prom(
        140,
        "Slippage vs decision-time mid (bps)",
        "paper-trading-realism phase-7b: paper_slippage_vs_decision_bps — signed bps between fill price and the decision-time mid. Positive = book paid more (BUY) / received less (SELL) than the strategy's signal-time view of the market. Captures latency drift + half-spread + slippage cushion in one number — distinct from the existing `Fill-price advantage lost` heatmap (which compares pricing models). Heatmap with cumulative-le buckets. Populates only after the next paper-trader image rolls (the metric is new in this phase; pre-rollout the panel will be empty).",
        'sum by (le) (rate(paper_slippage_vs_decision_bps_bucket[$__rate_interval]))',
        x=0,
        y_off=0,
        w=24,
        h=8,
    )


# ---------------------------------------------------------------------------
# y-reflow: pack panels top-to-bottom, respecting in-row relative offsets.
# ---------------------------------------------------------------------------


def reflow_y(panels: list[dict]) -> list[dict]:
    """Recompute gridPos.y for every panel.

    Algorithm: walk panels in array order. Group into (row, [children]).
    Panels before the first row are header/banner — stack them. Each
    row is laid out at cursor; children keep their relative y offset
    inside the row (so multi-column stacking is preserved). The row's
    bottom = max(child.y + child.h); cursor advances to that.
    """
    out = []
    i = 0
    cursor = 0
    n = len(panels)

    # Header (panels before first row).
    while i < n and panels[i].get("type") != "row":
        p = panels[i]
        p["gridPos"]["y"] = cursor
        cursor += p["gridPos"]["h"]
        out.append(p)
        i += 1

    # Row groups.
    while i < n:
        rp = panels[i]
        assert rp.get("type") == "row", f"expected row at index {i}, got {rp.get('type')}"
        rp["gridPos"]["y"] = cursor
        cursor += 1
        out.append(rp)
        i += 1

        # Collect children until next row or end.
        children = []
        while i < n and panels[i].get("type") != "row":
            children.append(panels[i])
            i += 1

        if not children:
            continue

        base_y = min(c["gridPos"]["y"] for c in children)
        bottom = cursor
        for c in children:
            relative = c["gridPos"]["y"] - base_y
            c["gridPos"]["y"] = cursor + relative
            this_bottom = c["gridPos"]["y"] + c["gridPos"]["h"]
            if this_bottom > bottom:
                bottom = this_bottom
            out.append(c)
        cursor = bottom

    return out


# ---------------------------------------------------------------------------
# Insert helpers.
# ---------------------------------------------------------------------------


def insert_after_row(panels: list[dict], row_id: int, new_panels: list[dict]) -> list[dict]:
    """Insert new_panels immediately after the row with id == row_id and
    its child panels (i.e., before the NEXT row). Returns a new list."""
    out = []
    i = 0
    n = len(panels)
    inserted = False
    while i < n:
        p = panels[i]
        out.append(p)
        i += 1
        if not inserted and p.get("type") == "row" and p["id"] == row_id:
            # Append children of this row (until next row).
            while i < n and panels[i].get("type") != "row":
                out.append(panels[i])
                i += 1
            for np in new_panels:
                out.append(np)
            inserted = True
    if not inserted:
        raise ValueError(f"row id {row_id} not found")
    return out


def insert_before_row(panels: list[dict], row_id: int, new_panels: list[dict]) -> list[dict]:
    """Insert new_panels immediately before the row with id == row_id."""
    out = []
    inserted = False
    for p in panels:
        if not inserted and p.get("type") == "row" and p["id"] == row_id:
            for np in new_panels:
                out.append(np)
            inserted = True
        out.append(p)
    if not inserted:
        raise ValueError(f"row id {row_id} not found")
    return out


def insert_panel_in_row(panels: list[dict], row_id: int, new_panel: dict) -> list[dict]:
    """Insert new_panel as the LAST child of the row with id == row_id.

    'Last child' means: visually below all existing children of that
    row, regardless of array position. Sets new_panel.gridPos.y to
    max(existing_child.y + existing_child.h) so the post-reflow
    relative offset stacks the new panel beneath siblings."""
    out = []
    i = 0
    n = len(panels)
    inserted = False
    while i < n:
        p = panels[i]
        out.append(p)
        i += 1
        if not inserted and p.get("type") == "row" and p["id"] == row_id:
            # Walk children, capturing geometry, and append as we go.
            existing_children = []
            while i < n and panels[i].get("type") != "row":
                existing_children.append(panels[i])
                out.append(panels[i])
                i += 1
            if existing_children:
                stack_y = max(c["gridPos"]["y"] + c["gridPos"]["h"] for c in existing_children)
                new_panel["gridPos"]["y"] = stack_y
            out.append(new_panel)
            inserted = True
    if not inserted:
        raise ValueError(f"row id {row_id} not found")
    return out


# ---------------------------------------------------------------------------
# Main.
# ---------------------------------------------------------------------------


def main():
    with DASHBOARD.open() as f:
        d = json.load(f, object_pairs_hook=OrderedDict)

    panels = list(d["panels"])

    # 1. KPI strip at top: insert before the Risk row (id=30).
    panels = insert_before_row(panels, 30, kpi_strip_section())

    # 2. Strategy quality + Strategy comparison rows: insert before
    #    Account row (id=2). Order matters — Strategy quality first,
    #    then Strategy comparison.
    new_rows = strategy_quality_section() + strategy_comparison_section()
    panels = insert_before_row(panels, 2, new_rows)

    # 3. Rolling returns panel: insert as last child of Account row
    #    (id=2). Sits after Drawdown (id=6) before the Positions &
    #    trades row.
    panels = insert_panel_in_row(panels, 2, rolling_returns_panel())

    # 4. Slippage-vs-decision heatmap: insert as last child of Fill
    #    realism row (id=27).
    panels = insert_panel_in_row(panels, 27, slippage_heatmap_panel())

    # 5. Reflow y so the new layout actually packs cleanly.
    panels = reflow_y(panels)

    # 6. Update top-level description.
    d["description"] = (
        "Paper-trading sandbox dashboard. See "
        "workstreams/paper-trading-realism/briefs/research-platform-architecture.md "
        "and workstreams/paper-trading-realism/briefs/sandbox-live-separation.md "
        "for what `research-grade` means here, and the phase-7b retro for "
        "decisions behind the row layout (KPI strip on top, strategy-quality "
        "and strategy-comparison rows for promotion-decision support)."
    )

    d["panels"] = panels

    with DASHBOARD.open("w") as f:
        json.dump(d, f, indent=2)
        f.write("\n")
    print(f"updated {DASHBOARD}: {len(panels)} panels")


if __name__ == "__main__":
    main()
