// execution_quality.go — market-closed rejects, fill spread heatmap, fill spread quantiles.
// Renumbered: row 1000, panels 1001-1003.
// (Phase-7b ids: row 20 — DUPLICATE id with timeseries id=20 in Account; resolved by renumber.)
package rows

import (
	"github.com/grafana/grafana-foundation-sdk/go/common"
	"github.com/grafana/grafana-foundation-sdk/go/dashboard"
	"github.com/grafana/grafana-foundation-sdk/go/heatmap"
	"github.com/grafana/grafana-foundation-sdk/go/prometheus"
	"github.com/grafana/grafana-foundation-sdk/go/timeseries"

	"github.com/Chalupa-Tech/chalupa-infra/scripts/dashboards/internal/common/datasources"
	"github.com/Chalupa-Tech/chalupa-infra/scripts/dashboards/internal/common/layout"
)

const (
	executionRowID    = 1000
	executionRowTitle = "Execution quality"
	executionHeight   = 9 // 1 + 8 (heatmap / timeseries h)
)

func ExecutionQuality(db *dashboard.DashboardBuilder, yBase int) int {
	db.WithRow(layout.Row(executionRowID, yBase, executionRowTitle))
	db.WithPanel(marketClosedRejects(yBase + 1))
	db.WithPanel(fillSpreadHeatmap(yBase + 1))
	db.WithPanel(fillSpreadQuantilesByBook(yBase + 1))
	return executionHeight
}

func marketClosedRejects(y int) *timeseries.PanelBuilder {
	return timeseries.NewPanelBuilder().
		Id(1001).
		Title("Market-closed rejects by book (rate/5m)").
		Description(
			"paper-trading-realism phase-2: orders refused by the session " +
				"gate (config books[].session). Non-zero off-hours is desired " +
				"steady state for session=regular; non-zero during market hours " +
				"= holiday calendar stale or clock wrong.").
		GridPos(layout.Pos(0, y, 8, 8)).
		Datasource(datasources.Victoria()).
		WithTarget(prometheus.NewDataqueryBuilder().
			RefId("A").
			Expr(`rate(paper_market_closed_rejects_total{book_id=~"$book_id"}[5m])`).
			LegendFormat("{{book_id}} / {{strategy}}")).
		Unit("short").
		ColorScheme(dashboard.NewFieldColorBuilder().Mode(dashboard.FieldColorModeIdPaletteClassic))
}

func fillSpreadHeatmap(y int) *heatmap.PanelBuilder {
	return heatmap.NewPanelBuilder().
		Id(1002).
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

func fillSpreadQuantilesByBook(y int) *timeseries.PanelBuilder {
	return timeseries.NewPanelBuilder().
		Id(1003).
		Title("Fill spread P50 / P95 / P99 by book (bps, 15m)").
		Description(
			"paper-trading-realism phase-2: per-book spread quantiles. P50 " +
				"<10bps on liquid names is expected; P95 spikes indicate thin-name " +
				"or off-hours exposure that phase-4's ask/bid fill model will " +
				"make material.").
		GridPos(layout.Pos(16, y, 8, 8)).
		Datasource(datasources.Victoria()).
		WithTarget(prometheus.NewDataqueryBuilder().
			RefId("A").
			Expr(`histogram_quantile(0.50, sum by (le, book_id) (rate(paper_fill_spread_bps_bucket{book_id=~"$book_id"}[15m])))`).
			LegendFormat("P50 {{book_id}}")).
		WithTarget(prometheus.NewDataqueryBuilder().
			RefId("B").
			Expr(`histogram_quantile(0.95, sum by (le, book_id) (rate(paper_fill_spread_bps_bucket{book_id=~"$book_id"}[15m])))`).
			LegendFormat("P95 {{book_id}}")).
		WithTarget(prometheus.NewDataqueryBuilder().
			RefId("C").
			Expr(`histogram_quantile(0.99, sum by (le, book_id) (rate(paper_fill_spread_bps_bucket{book_id=~"$book_id"}[15m])))`).
			LegendFormat("P99 {{book_id}}")).
		Unit("short").
		ColorScheme(dashboard.NewFieldColorBuilder().Mode(dashboard.FieldColorModeIdPaletteClassic))
}
