// strategy_comparison.go — cross-strategy equity / cadence / slippage / P&L distribution.
// Renumbered: row 500, panels 501-504.
// (Phase-7b ids: row 120, panels 121-124.)
package rows

import (
	"github.com/grafana/grafana-foundation-sdk/go/barchart"
	"github.com/grafana/grafana-foundation-sdk/go/common"
	"github.com/grafana/grafana-foundation-sdk/go/dashboard"
	"github.com/grafana/grafana-foundation-sdk/go/heatmap"
	"github.com/grafana/grafana-foundation-sdk/go/prometheus"
	"github.com/grafana/grafana-foundation-sdk/go/stat"
	"github.com/grafana/grafana-foundation-sdk/go/timeseries"

	"github.com/Chalupa-Tech/chalupa-infra/scripts/dashboards/internal/common/datasources"
	"github.com/Chalupa-Tech/chalupa-infra/scripts/dashboards/internal/common/layout"
	"github.com/Chalupa-Tech/chalupa-infra/scripts/dashboards/internal/common/sql"
)

const (
	strategyComparisonRowID    = 500
	strategyComparisonRowTitle = "Strategy comparison"
	strategyComparisonHeight   = 13 // 1 + 8 (equity / cadence) + 4 (tax / heatmap)
)

func StrategyComparison(db *dashboard.DashboardBuilder, yBase int) int {
	db.WithRow(layout.Row(strategyComparisonRowID, yBase, strategyComparisonRowTitle))
	db.WithPanel(equityByStrategy(yBase + 1))
	db.WithPanel(fillsPerHourByStrategy(yBase + 1))
	db.WithPanel(slippageTaxByStrategy(yBase + 9))
	db.WithPanel(realizedPLDistribution(yBase + 9))
	return strategyComparisonHeight
}

func equityByStrategy(y int) *timeseries.PanelBuilder {
	rawSQL := "WITH equity_ts AS (" +
		"  SELECT pc.time, pc.book_id, pc.cash + COALESCE(pos.equity, 0) AS equity" +
		"  FROM paper_cash pc" +
		"  LEFT JOIN (" +
		"    SELECT time, book_id, SUM(quantity * mark_price) AS equity" +
		"    FROM paper_positions GROUP BY time, book_id" +
		"  ) pos ON pc.time = pos.time AND pc.book_id = pos.book_id" +
		"  WHERE $__timeFilter(pc.time)" +
		"), book_strategy AS (" +
		"  SELECT DISTINCT ON (book_id) book_id, strategy" +
		"  FROM paper_fills ORDER BY book_id, time DESC" +
		")" +
		" SELECT eq.time AS time, bs.strategy AS metric, SUM(eq.equity) AS equity" +
		" FROM equity_ts eq JOIN book_strategy bs ON eq.book_id = bs.book_id" +
		" WHERE bs.strategy IN ($strategy)" +
		" GROUP BY eq.time, bs.strategy" +
		" ORDER BY eq.time"
	return timeseries.NewPanelBuilder().
		Id(501).
		Title("Equity by strategy").
		Description(
			"paper-trading-realism phase-7b: SUM(equity) of every book " +
				"running each strategy, summed across books of the same " +
				"strategy. Answers `which strategy has the books up?`. Strategy " +
				"mapping uses each book's most-recent paper_fills.strategy " +
				"(books don't re-strategy in v1).").
		GridPos(layout.Pos(0, y, 12, 8)).
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

func fillsPerHourByStrategy(y int) *barchart.PanelBuilder {
	return barchart.NewPanelBuilder().
		Id(502).
		Title("Fills per hour by strategy").
		Description(
			"paper-trading-realism phase-7b: rate(paper_fills_total[1h]) × " +
				"3600 summed by strategy. Reveals cadence differences between " +
				"strategies — alternator-30s should be ~120/hr per book, sma 1m " +
				"crossover should be much sparser.").
		GridPos(layout.Pos(12, y, 12, 8)).
		Datasource(datasources.Victoria()).
		WithTarget(prometheus.NewDataqueryBuilder().
			RefId("A").
			Expr(`sum by (strategy) (rate(paper_fills_total{strategy=~"$strategy"}[1h])) * 3600`).
			LegendFormat("{{strategy}}")).
		Unit("short").
		ColorScheme(dashboard.NewFieldColorBuilder().Mode(dashboard.FieldColorModeIdPaletteClassic)).
		Legend(common.NewVizLegendOptionsBuilder().
			DisplayMode(common.LegendDisplayModeList).
			Placement(common.LegendPlacementBottom))
}

func slippageTaxByStrategy(y int) *stat.PanelBuilder {
	rawSQL := "WITH vwap AS (" +
		"  SELECT book_id, symbol," +
		"    time_bucket('1 day', time) AS trade_date," +
		"    SUM(price * quantity) / NULLIF(SUM(quantity), 0) AS vwap_px" +
		"  FROM paper_fills" +
		"  WHERE $__timeFilter(time) AND book_id IN ($book_id)" +
		"  GROUP BY book_id, symbol, trade_date" +
		")" +
		" SELECT bs.strategy AS metric," +
		"   SUM(ABS(pf.price - v.vwap_px) * pf.quantity) AS slippage_usd" +
		" FROM paper_fills pf" +
		" JOIN vwap v ON pf.book_id = v.book_id AND pf.symbol = v.symbol" +
		"   AND time_bucket('1 day', pf.time) = v.trade_date" +
		" JOIN (" +
		"  SELECT DISTINCT ON (book_id) book_id, strategy" +
		"  FROM paper_fills ORDER BY book_id, time DESC" +
		") bs ON pf.book_id = bs.book_id" +
		" WHERE $__timeFilter(pf.time) AND pf.book_id IN ($book_id)" +
		" GROUP BY bs.strategy"
	return stat.NewPanelBuilder().
		Id(503).
		Title("Slippage tax by strategy (USD)").
		Description(
			"paper-trading-realism phase-7b: |fill_price − " +
				"daily_VWAP_for_book_symbol| × quantity, summed per strategy. " +
				"Approximates execution cost (no mark-at-fill column on " +
				"paper_fills today; the daily-VWAP proxy is biased toward zero " +
				"for symbols with few fills/day, so treat absolute values " +
				"comparatively, not as ground truth). Expect alternator-30s to " +
				"dominate cumulative tax purely on volume.").
		GridPos(layout.Pos(0, y, 12, 4)).
		Datasource(datasources.Timescale()).
		WithTarget(sql.Table("A", rawSQL)).
		Unit("currencyUSD").
		Thresholds(dashboard.NewThresholdsConfigBuilder().
			Mode(dashboard.ThresholdsModeAbsolute).
			Steps([]dashboard.Threshold{
				{Color: "transparent", Value: nil},
			})).
		ColorMode(common.BigValueColorModeBackground).
		GraphMode(common.BigValueGraphModeNone).
		ReduceOptions(common.NewReduceDataOptionsBuilder().
			Calcs([]string{"lastNotNull"}).
			Fields("").
			Values(false))
}

func realizedPLDistribution(y int) *heatmap.PanelBuilder {
	rawSQL := "SELECT pf.time AS time, bs.strategy AS metric, pf.realized_pl AS value " +
		"FROM paper_fills pf " +
		"JOIN (" +
		"  SELECT DISTINCT ON (book_id) book_id, strategy " +
		"  FROM paper_fills ORDER BY book_id, time DESC" +
		") bs ON pf.book_id = bs.book_id " +
		"WHERE pf.realized_pl IS NOT NULL AND $__timeFilter(pf.time) " +
		"  AND pf.book_id IN ($book_id) AND bs.strategy IN ($strategy) " +
		"ORDER BY pf.time"
	return heatmap.NewPanelBuilder().
		Id(504).
		Title("Realized P&L distribution by strategy").
		Description(
			"paper-trading-realism phase-7b: realized_pl bucketed per " +
				"strategy. Fat right tail = healthy (a few big wins carry the " +
				"curve); fat left tail = concerning. Compare alternator vs " +
				"sma shape, not absolute counts.").
		GridPos(layout.Pos(12, y, 12, 4)).
		Datasource(datasources.Timescale()).
		WithTarget(sql.TimeSeries("A", rawSQL)).
		Unit("short").
		Calculate(true).
		Calculation(common.NewHeatmapCalculationOptionsBuilder().
			YBuckets(common.NewHeatmapCalculationBucketConfigBuilder().
				Mode(common.HeatmapCalculationModeCount).
				Value("20"))).
		YAxis(heatmap.NewYAxisConfigBuilder().Unit("currencyUSD")).
		Color(heatmap.NewHeatmapColorOptionsBuilder().
			Scheme("Spectral").
			Mode(heatmap.HeatmapColorModeScheme).
			Steps(64))
}
