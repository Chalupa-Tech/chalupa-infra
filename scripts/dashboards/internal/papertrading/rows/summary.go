// summary.go — at-a-glance KPI strip.
// Renumbered: row 200, KPI tiles 201-204.
// (Phase-7b ids: row 100, tiles 101-104.)
package rows

import (
	"github.com/grafana/grafana-foundation-sdk/go/common"
	"github.com/grafana/grafana-foundation-sdk/go/dashboard"
	"github.com/grafana/grafana-foundation-sdk/go/prometheus"
	"github.com/grafana/grafana-foundation-sdk/go/stat"

	"github.com/Chalupa-Tech/chalupa-infra/scripts/dashboards/internal/common/datasources"
	"github.com/Chalupa-Tech/chalupa-infra/scripts/dashboards/internal/common/layout"
	"github.com/Chalupa-Tech/chalupa-infra/scripts/dashboards/internal/common/sql"
	"github.com/Chalupa-Tech/chalupa-infra/scripts/dashboards/internal/common/thresholds"
)

const (
	summaryRowID    = 200
	summaryRowTitle = "Summary (at-a-glance)"
	summaryHeight   = 5 // 1 (row header) + 4 (KPI tile h)
)

// Summary appends the Summary row + its 4 KPI tiles to db.
func Summary(db *dashboard.DashboardBuilder, yBase int) int {
	db.WithRow(layout.Row(summaryRowID, yBase, summaryRowTitle))
	db.WithPanel(equityVsStartingCash(yBase + 1))
	db.WithPanel(todayPnLPerBook(yBase + 1))
	db.WithPanel(maxDrawdownSession(yBase + 1))
	db.WithPanel(winRatePerBook(yBase + 1))
	return summaryHeight
}

func equityVsStartingCash(y int) *stat.PanelBuilder {
	return stat.NewPanelBuilder().
		Id(201).
		Title("Equity vs starting cash").
		Description(
			"paper-trading-realism phase-7b: (paper_equity_usd / 10000) − 1. " +
				"Hardcodes $10k starting cash, which matches the per-book " +
				"values.yaml today; if startingCash ever varies per book this " +
				"becomes wrong silently and we'll need a paper_starting_cash_usd " +
				"metric. Green ≥ +1% (book is compounding); neutral ±1% (noise " +
				"floor); amber −1% to −5%; red < −5%.").
		GridPos(layout.Pos(0, y, 6, 4)).
		Datasource(datasources.Victoria()).
		WithTarget(prometheus.NewDataqueryBuilder().
			RefId("A").
			Expr(`paper_equity_usd{book_id=~"$book_id"} / 10000 - 1`).
			LegendFormat("{{book_id}}").
			Instant()).
		Unit("percentunit").
		Thresholds(thresholds.Build(thresholds.EquityReturn)).
		ColorMode(common.BigValueColorModeBackground).
		GraphMode(common.BigValueGraphModeNone).
		ReduceOptions(common.NewReduceDataOptionsBuilder().
			Calcs([]string{"lastNotNull"}).
			Fields("").
			Values(false))
}

func todayPnLPerBook(y int) *stat.PanelBuilder {
	return stat.NewPanelBuilder().
		Id(202).
		Title("Today P&L per book").
		Description(
			"paper-trading-realism phase-7b: paper_daily_pnl_usd (UTC day, " +
				"realized + unrealized). Same series as the Risk row's " +
				"timeseries, surfaced here for at-a-glance health. Resets at " +
				"UTC midnight.").
		GridPos(layout.Pos(6, y, 6, 4)).
		Datasource(datasources.Victoria()).
		WithTarget(prometheus.NewDataqueryBuilder().
			RefId("A").
			Expr(`paper_daily_pnl_usd{book_id=~"$book_id"}`).
			LegendFormat("{{book_id}}").
			Instant()).
		Unit("currencyUSD").
		Thresholds(thresholds.Build(thresholds.TodayPnL)).
		ColorMode(common.BigValueColorModeBackground).
		GraphMode(common.BigValueGraphModeNone).
		ReduceOptions(common.NewReduceDataOptionsBuilder().
			Calcs([]string{"lastNotNull"}).
			Fields("").
			Values(false))
}

func maxDrawdownSession(y int) *stat.PanelBuilder {
	rawSQL := "WITH equity_ts AS (" +
		"  SELECT pc.time, pc.book_id, pc.cash + COALESCE(pos.equity, 0) AS equity" +
		"  FROM paper_cash pc" +
		"  LEFT JOIN (" +
		"    SELECT time, book_id, SUM(quantity * mark_price) AS equity" +
		"    FROM paper_positions GROUP BY time, book_id" +
		"  ) pos ON pc.time = pos.time AND pc.book_id = pos.book_id" +
		"  WHERE pc.book_id IN ($book_id)" +
		"), with_peak AS (" +
		"  SELECT book_id, time, equity," +
		"         MAX(equity) OVER (PARTITION BY book_id ORDER BY time" +
		"                           ROWS BETWEEN UNBOUNDED PRECEDING AND CURRENT ROW) AS peak" +
		"  FROM equity_ts" +
		") SELECT book_id, MIN((equity - peak) / NULLIF(peak, 0)) AS max_dd FROM with_peak GROUP BY book_id"
	return stat.NewPanelBuilder().
		Id(203).
		Title("Max drawdown (session)").
		Description(
			"paper-trading-realism phase-7b: maximum peak-to-trough drawdown " +
				"observed for each book since first cash snapshot (whole session, " +
				"not just visible range). Computed in TimescaleDB so the value " +
				"survives Prom retention; matches the promotion-threshold " +
				"convention (≤ 3%) from sandbox-live-separation brief — amber at " +
				"−1% (worth watching), red at −3% (gates promotion).").
		GridPos(layout.Pos(12, y, 6, 4)).
		Datasource(datasources.Timescale()).
		WithTarget(sql.Table("A", rawSQL)).
		Unit("percentunit").
		Thresholds(thresholds.Build(thresholds.MaxDrawdown)).
		ColorMode(common.BigValueColorModeBackground).
		GraphMode(common.BigValueGraphModeNone).
		ReduceOptions(common.NewReduceDataOptionsBuilder().
			Calcs([]string{"lastNotNull"}).
			Fields("").
			Values(false))
}

func winRatePerBook(y int) *stat.PanelBuilder {
	rawSQL := "SELECT book_id," +
		"   COUNT(*) FILTER (WHERE realized_pl > 0)::float / NULLIF(COUNT(*), 0) AS win_rate" +
		" FROM paper_fills" +
		" WHERE side = 'SELL' AND realized_pl IS NOT NULL" +
		"   AND book_id IN ($book_id) AND $__timeFilter(time)" +
		" GROUP BY book_id"
	return stat.NewPanelBuilder().
		Id(204).
		Title("Win rate per book").
		Description(
			"paper-trading-realism phase-7b: COUNT(SELL fills with realized_pl " +
				"> 0) / COUNT(SELL fills) — fraction of trades that closed at a " +
				"profit, scoped to selected books. Promotion-relevant (the " +
				"brief's positive-return-days rule implies a comparable win/lose " +
				"cadence). Green ≥ 55%; amber 45–55%; red < 45%.").
		GridPos(layout.Pos(18, y, 6, 4)).
		Datasource(datasources.Timescale()).
		WithTarget(sql.Table("A", rawSQL)).
		Unit("percentunit").
		Thresholds(thresholds.Build(thresholds.WinRate)).
		ColorMode(common.BigValueColorModeBackground).
		GraphMode(common.BigValueGraphModeNone).
		ReduceOptions(common.NewReduceDataOptionsBuilder().
			Calcs([]string{"lastNotNull"}).
			Fields("").
			Values(false))
}
