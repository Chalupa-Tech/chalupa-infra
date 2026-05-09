// account.go — cash, equity, exposure, realized P&L, drawdown, rolling return per book.
// Renumbered: row 600, panels 601-606.
// (Phase-7b ids: row 2, panels 3-6, 20 (timeseries — duplicate id with row 20), 130.)
package rows

import (
	"github.com/grafana/grafana-foundation-sdk/go/cog"
	"github.com/grafana/grafana-foundation-sdk/go/common"
	"github.com/grafana/grafana-foundation-sdk/go/dashboard"
	"github.com/grafana/grafana-foundation-sdk/go/prometheus"
	"github.com/grafana/grafana-foundation-sdk/go/stat"
	"github.com/grafana/grafana-foundation-sdk/go/timeseries"

	"github.com/Chalupa-Tech/chalupa-infra/scripts/dashboards/internal/common/cte"
	"github.com/Chalupa-Tech/chalupa-infra/scripts/dashboards/internal/common/datasources"
	"github.com/Chalupa-Tech/chalupa-infra/scripts/dashboards/internal/common/layout"
	"github.com/Chalupa-Tech/chalupa-infra/scripts/dashboards/internal/common/links"
	"github.com/Chalupa-Tech/chalupa-infra/scripts/dashboards/internal/common/sql"
	"github.com/Chalupa-Tech/chalupa-infra/scripts/dashboards/internal/common/thresholds"
)

const (
	accountRowID    = 600
	accountRowTitle = "Account"
	accountHeight   = 34 // 1 + 9 (equity curve) + 8*3 (cum / drawdown / rolling)
)

func Account(db *dashboard.DashboardBuilder, yBase int) int {
	db.WithRow(layout.Row(accountRowID, yBase, accountRowTitle))
	db.WithPanel(equityCurveByBook(yBase + 1))
	db.WithPanel(currentEquityByBook(yBase + 1))
	db.WithPanel(openPositionsByBook(yBase + 5))
	db.WithPanel(cumulativeRealizedPL(yBase + 10))
	db.WithPanel(drawdownPctByBook(yBase + 18))
	db.WithPanel(rollingReturnByBook(yBase + 26))
	return accountHeight
}

func equityCurveByBook(y int) *timeseries.PanelBuilder {
	rawSQL := "SELECT pc.time AS \"time\", pc.book_id AS metric, pc.cash + COALESCE(pos.equity, 0) AS equity" +
		" FROM paper_cash pc" +
		" LEFT JOIN (SELECT time, book_id, SUM(quantity * mark_price) AS equity" +
		" FROM paper_positions GROUP BY time, book_id) pos" +
		" ON pc.time = pos.time AND pc.book_id = pos.book_id" +
		" WHERE $__timeFilter(pc.time) AND pc.book_id IN ($book_id)" +
		" ORDER BY pc.time"
	return timeseries.NewPanelBuilder().
		Id(601).
		Title("Equity curve (cash + mark) by book").
		GridPos(layout.Pos(0, y, 16, 9)).
		Datasource(datasources.Timescale()).
		WithTarget(sql.TimeSeries("A", rawSQL)).
		Unit("currencyUSD").
		LineWidth(2).
		FillOpacity(10).
		ColorScheme(dashboard.NewFieldColorBuilder().Mode(dashboard.FieldColorModeIdPaletteClassic)).
		Legend(common.NewVizLegendOptionsBuilder().
			DisplayMode(common.LegendDisplayModeList).
			Placement(common.LegendPlacementBottom))
}

func currentEquityByBook(y int) *stat.PanelBuilder {
	return stat.NewPanelBuilder().
		Id(602).
		Title("Current equity by book").
		Description(
			"Latest paper_equity_usd per book (cash + sum(qty × mark_price)). " +
				"Color scale matches the per-book risk window: ≥+5% (≥$10,500) " +
				"green / ±5% gray neutral / −5% to −10% amber attention / <−10% " +
				"(<$9,000) red — the daily max-loss guard floor sits at $500 (5% " +
				"of starting cash), so coloring tracks risk, not every " +
				"slippage-tax bleed.").
		GridPos(layout.Pos(16, y, 8, 4)).
		Datasource(datasources.Victoria()).
		WithTarget(prometheus.NewDataqueryBuilder().
			RefId("A").
			Expr(`paper_equity_usd{book_id=~"$book_id"}`).
			LegendFormat("{{book_id}}").
			Instant()).
		Unit("currencyUSD").
		Thresholds(thresholds.Build(thresholds.EquityByBook)).
		ColorMode(common.BigValueColorModeBackground).
		GraphMode(common.BigValueGraphModeNone).
		ReduceOptions(common.NewReduceDataOptionsBuilder().
			Calcs([]string{"lastNotNull"}).
			Fields("").
			Values(false))
}

func openPositionsByBook(y int) *stat.PanelBuilder {
	return stat.NewPanelBuilder().
		Id(603).
		Title("Open positions by book").
		GridPos(layout.Pos(16, y, 8, 5)).
		Datasource(datasources.Victoria()).
		WithTarget(prometheus.NewDataqueryBuilder().
			RefId("A").
			Expr(`paper_open_positions{book_id=~"$book_id"}`).
			LegendFormat("{{book_id}}").
			Instant()).
		Unit("short").
		ColorScheme(dashboard.NewFieldColorBuilder().Mode(dashboard.FieldColorModeIdPaletteClassic)).
		ColorMode(common.BigValueColorModeValue).
		GraphMode(common.BigValueGraphModeNone).
		ReduceOptions(common.NewReduceDataOptionsBuilder().
			Calcs([]string{"lastNotNull"}).
			Fields("").
			Values(false))
}

func cumulativeRealizedPL(y int) *timeseries.PanelBuilder {
	rawSQL := "SELECT time, book_id AS metric," +
		" SUM(realized_pl) OVER (PARTITION BY book_id ORDER BY time) AS cumulative_realized_pl" +
		" FROM paper_fills" +
		" WHERE realized_pl IS NOT NULL AND $__timeFilter(time) AND book_id IN ($book_id)" +
		" ORDER BY time"
	return timeseries.NewPanelBuilder().
		Id(604).
		Title("Cumulative realized P&L by book").
		GridPos(layout.Pos(0, y, 24, 8)).
		Datasource(datasources.Timescale()).
		WithTarget(sql.TimeSeries("A", rawSQL)).
		Unit("currencyUSD").
		LineWidth(2).
		FillOpacity(10).
		ColorScheme(dashboard.NewFieldColorBuilder().Mode(dashboard.FieldColorModeIdPaletteClassic)).
		Legend(common.NewVizLegendOptionsBuilder().
			DisplayMode(common.LegendDisplayModeList).
			Placement(common.LegendPlacementBottom))
}

func drawdownPctByBook(y int) *timeseries.PanelBuilder {
	return timeseries.NewPanelBuilder().
		Id(605).
		Title("Drawdown % by book (peak-to-trough)").
		Description(
			"Current drawdown from the running peak of paper_equity_usd " +
				"within the visible time range. Computed in VictoriaMetrics " +
				"(continuous scrape) rather than from paper_cash/paper_positions " +
				"snapshots, which only emit on change and produced sparse data " +
				"in the SQL version. Per series: 100 × (equity − " +
				"running_max(equity)) / running_max(equity). 0% means " +
				"at-or-above peak; negative values are the live drawdown.").
		GridPos(layout.Pos(0, y, 24, 8)).
		Datasource(datasources.Victoria()).
		WithTarget(prometheus.NewDataqueryBuilder().
			RefId("A").
			Expr(`100 * (paper_equity_usd{book_id=~"$book_id"} - running_max(paper_equity_usd{book_id=~"$book_id"})) / running_max(paper_equity_usd{book_id=~"$book_id"})`).
			LegendFormat("{{book_id}}")).
		Unit("percent").
		LineWidth(2).
		FillOpacity(10).
		ColorScheme(dashboard.NewFieldColorBuilder().Mode(dashboard.FieldColorModeIdPaletteClassic)).
		// Drilldown #1 (paper-trading-realism phase-7e3 navigation
		// plan): drawdown spikes often coincide with stale quotes. Hop
		// to the same-board quote-age panel with the affected book
		// pre-selected, time window preserved.
		DataLinks([]cog.Builder[dashboard.DashboardLink]{
			links.To(
				"Quote age (panel 901)",
				"/d/paper-trading/?from=${__from}&to=${__to}&viewPanel=901&var-book_id=${__field.labels.book_id}",
			),
		})
}

func rollingReturnByBook(y int) *timeseries.PanelBuilder {
	// Single CTE chain (declared once at top) shared by both UNION arms.
	// PostgreSQL grammar disallows `WITH ... UNION ALL WITH ...`; the
	// previous form concatenated DailyReturnsCTE on both sides, which
	// rendered an SQLSTATE 42601 syntax error and a permanently empty
	// panel. Each arm inlines its own window frame because a named
	// WINDOW clause cannot span the UNION boundary either.
	rawSQL := cte.DailyReturnsCTE +
		"SELECT trade_date AS time, book_id || '-7d'  AS metric," +
		"   EXP(SUM(LN(1 + COALESCE(ret, 0))) OVER (PARTITION BY book_id" +
		"                ORDER BY trade_date" +
		"                ROWS BETWEEN 6  PRECEDING AND CURRENT ROW))  - 1 AS r" +
		" FROM returns" +
		" UNION ALL" +
		" SELECT trade_date AS time, book_id || '-30d' AS metric," +
		"   EXP(SUM(LN(1 + COALESCE(ret, 0))) OVER (PARTITION BY book_id" +
		"                ORDER BY trade_date" +
		"                ROWS BETWEEN 29 PRECEDING AND CURRENT ROW)) - 1 AS r" +
		" FROM returns" +
		" ORDER BY time"
	return timeseries.NewPanelBuilder().
		Id(606).
		Title("Rolling return % by book (7d / 30d)").
		Description(
			"paper-trading-realism phase-7b: compounded return over rolling " +
				"windows. Series suffix `-7d` / `-30d`. Computed via daily " +
				"geometric chaining over the daily-returns CTE. QTD intentionally " +
				"omitted from v1 — a clean QTD requires a session-aware " +
				"date-trunc that is not yet load-bearing; pencil it in for " +
				"phase-7c if/when promotion windows shorten.").
		GridPos(layout.Pos(0, y, 24, 8)).
		Datasource(datasources.Timescale()).
		WithTarget(sql.TimeSeries("A", rawSQL)).
		Unit("percentunit").
		LineWidth(2).
		FillOpacity(10).
		ColorScheme(dashboard.NewFieldColorBuilder().Mode(dashboard.FieldColorModeIdPaletteClassic)).
		Legend(common.NewVizLegendOptionsBuilder().
			DisplayMode(common.LegendDisplayModeList).
			Placement(common.LegendPlacementBottom))
}
