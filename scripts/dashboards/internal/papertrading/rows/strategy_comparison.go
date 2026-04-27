// strategy_comparison.go — cross-book equity curves + slippage heatmap.
// Phase-7b panel ids: row 120, panels 121-124.
// Phase-7e slice: ports id=124 ("Realized P&L distribution by
// strategy") — the SQL-calc heatmap canary.
package rows

import (
	"github.com/grafana/grafana-foundation-sdk/go/common"
	"github.com/grafana/grafana-foundation-sdk/go/dashboard"
	"github.com/grafana/grafana-foundation-sdk/go/heatmap"

	"github.com/Chalupa-Tech/chalupa-infra/scripts/dashboards/internal/common/datasources"
	"github.com/Chalupa-Tech/chalupa-infra/scripts/dashboards/internal/common/layout"
	"github.com/Chalupa-Tech/chalupa-infra/scripts/dashboards/internal/common/sql"
)

const (
	strategyComparisonRowID    = 120
	strategyComparisonRowTitle = "Strategy comparison"
	strategyComparisonHeight   = 5 // 1 (row header) + 4 (heatmap h)
)

func StrategyComparison(db *dashboard.DashboardBuilder, yBase int) int {
	db.WithRow(layout.Row(strategyComparisonRowID, yBase, strategyComparisonRowTitle))
	db.WithPanel(realizedPLDistribution(yBase + 1))
	return strategyComparisonHeight
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
		Id(124).
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
