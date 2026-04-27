// execution_quality.go — cumulative realized P&L, slippage histogram heatmap.
// Phase-7b panel ids: row 20 (DUPLICATE — see ID renumbering note), panels 21-23.
// Phase-7e slice: ports id=22 ("Fill spread distribution") — the
// Prometheus-histogram heatmap canary, distinct heatmap mode from the
// SQL-calc heatmap in strategy_comparison.
package rows

import (
	"github.com/grafana/grafana-foundation-sdk/go/common"
	"github.com/grafana/grafana-foundation-sdk/go/dashboard"
	"github.com/grafana/grafana-foundation-sdk/go/heatmap"
	"github.com/grafana/grafana-foundation-sdk/go/prometheus"

	"github.com/Chalupa-Tech/chalupa-infra/scripts/dashboards/internal/common/datasources"
	"github.com/Chalupa-Tech/chalupa-infra/scripts/dashboards/internal/common/layout"
)

const (
	executionRowID    = 20 // NOTE duplicate id with timeseries panel id=20 in account.go
	executionRowTitle = "Execution quality"
	executionHeight   = 9 // 1 (row header) + 8 (heatmap h)
)

func ExecutionQuality(db *dashboard.DashboardBuilder, yBase int) int {
	db.WithRow(layout.Row(executionRowID, yBase, executionRowTitle))
	db.WithPanel(fillSpreadHeatmap(yBase + 1))
	return executionHeight
}

func fillSpreadHeatmap(y int) *heatmap.PanelBuilder {
	return heatmap.NewPanelBuilder().
		Id(22).
		Title("Fill spread distribution (bps) — all symbols").
		Description(
			"paper-trading-realism phase-2: observed (ask-bid)/mid in bps " +
				"at fill time. Heatmap Y-axis is bucket upper bound, value = " +
				"rate(_bucket[5m]). Wide distribution = material mid-fill " +
				"bias; phase-4 replaces mid±5bps with ask-on-BUY / bid-on-SELL.").
		GridPos(layout.Pos(8, y, 8, 8)).
		Datasource(datasources.Victoria()).
		WithTarget(prometheus.NewDataqueryBuilder().
			RefId("A").
			Expr(`sum by (le) (rate(paper_fill_spread_bps_bucket{book_id=~"$book_id",symbol=~"$symbol"}[5m]))`).
			LegendFormat("{{le}}").
			Format(prometheus.PromQueryFormatHeatmap)).
		YAxis(heatmap.NewYAxisConfigBuilder().Unit("short")).
		Calculate(false).
		ScaleDistribution(common.NewScaleDistributionConfigBuilder().
			Type(common.ScaleDistributionLog).
			Log(10))
}
