// fill_realism.go — mid-fill bias quantiles, distribution, decision-time slippage.
// Renumbered: row 1100, panels 1101-1103.
// (Phase-7b ids: row 27, panels 28-29, 140.)
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
	fillRealismRowID    = 1100
	fillRealismRowTitle = "Fill realism"
	fillRealismHeight   = 17 // 1 + 8 (P50/P95 + heatmap) + 8 (slippage heatmap)
)

func FillRealism(db *dashboard.DashboardBuilder, yBase int) int {
	db.WithRow(layout.Row(fillRealismRowID, yBase, fillRealismRowTitle))
	db.WithPanel(midFillBiasQuantiles(yBase + 1))
	db.WithPanel(midFillBiasDistribution(yBase + 1))
	db.WithPanel(slippageVsDecisionMid(yBase + 9))
	return fillRealismHeight
}

func midFillBiasQuantiles(y int) *timeseries.PanelBuilder {
	return timeseries.NewPanelBuilder().
		Id(1101).
		Title("Fill-price advantage lost (bps) — P50 / P95 by book (15m)").
		Description(
			"paper-trading-realism phase-4: per-book quantiles of " +
				"paper_mid_fill_bias_bps = signed (new ask/bid fill - old mid ± " +
				"5bps fill) / mid * 10_000. Positive = the phase-4 model costs " +
				"the book MORE than the pre-phase-4 model would have (you now " +
				"pay the real spread). Expected P50 ~= (spread_bps - 10)/2 per " +
				"symbol: if P50 spread was 12bps, P50 bias should be ~1bps; if " +
				"P50 spread was 5bps, P50 bias should be slightly negative " +
				"(rare — phase-4 is cheaper than legacy only when half-spread < " +
				"5bps).").
		GridPos(layout.Pos(0, y, 12, 8)).
		Datasource(datasources.Victoria()).
		WithTarget(prometheus.NewDataqueryBuilder().
			RefId("A").
			Expr(`histogram_quantile(0.50, sum by (le, book_id) (rate(paper_mid_fill_bias_bps_bucket{book_id=~"$book_id"}[15m])))`).
			LegendFormat("P50 {{book_id}}")).
		WithTarget(prometheus.NewDataqueryBuilder().
			RefId("B").
			Expr(`histogram_quantile(0.95, sum by (le, book_id) (rate(paper_mid_fill_bias_bps_bucket{book_id=~"$book_id"}[15m])))`).
			LegendFormat("P95 {{book_id}}")).
		Unit("short").
		ColorScheme(dashboard.NewFieldColorBuilder().Mode(dashboard.FieldColorModeIdPaletteClassic))
}

func midFillBiasDistribution(y int) *heatmap.PanelBuilder {
	return heatmap.NewPanelBuilder().
		Id(1102).
		Title("Fill-price advantage lost (bps) — distribution all books").
		Description(
			"paper-trading-realism phase-4: heatmap of paper_mid_fill_bias_bps " +
				"across every (book, symbol). Y-axis is bucket upper bound in " +
				"bps. Mass concentrated right of 0 = phase-4 routinely costs " +
				"more than the pre-phase-4 proxy (expected on thin / off-hours " +
				"names). Mass left of 0 = the old model was a tax, not a gift " +
				"(rare). Empty panel = not enough fills to populate the " +
				"histogram yet; check again after a full session.").
		GridPos(layout.Pos(12, y, 12, 8)).
		Datasource(datasources.Victoria()).
		WithTarget(prometheus.NewDataqueryBuilder().
			RefId("A").
			Expr(`sum by (le) (rate(paper_mid_fill_bias_bps_bucket{book_id=~"$book_id",symbol=~"$symbol"}[5m]))`).
			LegendFormat("{{le}}").
			Format(prometheus.PromQueryFormatHeatmap)).
		YAxis(heatmap.NewYAxisConfigBuilder().Unit("short")).
		Calculate(false).
		ScaleDistribution(common.NewScaleDistributionConfigBuilder().
			Type(common.ScaleDistributionLog).
			Log(10))
}

func slippageVsDecisionMid(y int) *heatmap.PanelBuilder {
	return heatmap.NewPanelBuilder().
		Id(1103).
		Title("Slippage vs decision-time mid (bps)").
		Description(
			"paper-trading-realism phase-7b: paper_slippage_vs_decision_bps — " +
				"signed bps between fill price and the decision-time mid. " +
				"Positive = book paid more (BUY) / received less (SELL) than " +
				"the strategy's signal-time view of the market. Captures " +
				"latency drift + half-spread + slippage cushion in one number — " +
				"distinct from the existing `Fill-price advantage lost` heatmap " +
				"(which compares pricing models). Heatmap with cumulative-le " +
				"buckets. Populates only after the next paper-trader image " +
				"rolls (the metric is new in this phase; pre-rollout the panel " +
				"will be empty).").
		GridPos(layout.Pos(0, y, 24, 8)).
		Datasource(datasources.Victoria()).
		WithTarget(prometheus.NewDataqueryBuilder().
			RefId("A").
			Expr(`sum by (le) (rate(paper_slippage_vs_decision_bps_bucket[$__rate_interval]))`).
			Format(prometheus.PromQueryFormatHeatmap).
			LegendFormat("{{le}}")).
		Unit("short").
		YAxis(heatmap.NewYAxisConfigBuilder().Unit("short")).
		Calculate(false).
		ScaleDistribution(common.NewScaleDistributionConfigBuilder().
			Type(common.ScaleDistributionLinear)).
		Color(heatmap.NewHeatmapColorOptionsBuilder().
			Scheme("Spectral").
			Mode(heatmap.HeatmapColorModeScheme).
			Steps(64))
}
