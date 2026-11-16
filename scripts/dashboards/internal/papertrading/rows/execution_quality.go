// execution_quality.go — market-closed rejects, fill spread heatmap and quantiles, fill vs mid, order-to-fill.
// Renumbered: row 1000, panels 1001-1003; phase-13b adds 1004-1005.
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
	executionHeight   = 17 // 1 + 8 (first line) + 8 (phase-13 fill-quality line)
)

func ExecutionQuality(db *dashboard.DashboardBuilder, yBase int) int {
	db.WithRow(layout.Row(executionRowID, yBase, executionRowTitle))
	db.WithPanel(marketClosedRejects(yBase + 1))
	db.WithPanel(fillSpreadHeatmap(yBase + 1))
	db.WithPanel(fillSpreadQuantilesByBook(yBase + 1))
	db.WithPanel(fillVsMidQuantilesByBook(yBase + 9))
	db.WithPanel(orderToFillQuantilesByBookStage(yBase + 9))
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
				"bias; phase-4 replaces mid±5bps with ask-on-BUY / bid-on-SELL. " +
				"Since phase-13 (v0.16.0) this covers every fill path " +
				"(immediate MARKET, resting and limit fills) and counts only " +
				"persisted new fills; before, it covered immediate MARKET " +
				"fills only and also counted orders refused after pricing, so " +
				"data from before the rollout is not comparable.").
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
				"make material. Since phase-13 (v0.16.0) this covers every " +
				"fill path and counts only persisted new fills; earlier " +
				"data covered immediate MARKET fills only.").
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

// fillVsMidQuantilesByBook shows the cost of a fill against the mid of the
// quote it was priced from (paper-trading-realism phase-13). Unlike the
// mid-fill-bias panels, which compare two sim pricing models, this is the
// number a live adapter computes the same way, so paper and live compare.
func fillVsMidQuantilesByBook(y int) *timeseries.PanelBuilder {
	return timeseries.NewPanelBuilder().
		Id(1004).
		Title("Fill vs mid P50 / P95 by book (bps, 15m)").
		Description(
			"paper-trading-realism phase-13: paper_fill_vs_mid_bps = signed " +
				"(fill price - mid) / mid * 10_000, mid taken from the quote the " +
				"fill was priced from. Positive = the book paid more (BUY) or " +
				"received less (SELL) than mid. Unlike `Fill-price advantage " +
				"lost`, which compares two sim pricing models, a live adapter " +
				"computes this the same way, so paper and live read on one number. " +
				"Observed on every fill path, once per persisted fill, and only " +
				"when the pricing quote is two-sided and uncrossed. Series appear " +
				"after the first fill on v0.16.0; the panel is empty before that.").
		GridPos(layout.Pos(0, y, 12, 8)).
		Datasource(datasources.Victoria()).
		WithTarget(prometheus.NewDataqueryBuilder().
			RefId("A").
			Expr(`histogram_quantile(0.50, sum by (le, book_id) (rate(paper_fill_vs_mid_bps_bucket{book_id=~"$book_id"}[15m])))`).
			LegendFormat("P50 {{book_id}}")).
		WithTarget(prometheus.NewDataqueryBuilder().
			RefId("B").
			Expr(`histogram_quantile(0.95, sum by (le, book_id) (rate(paper_fill_vs_mid_bps_bucket{book_id=~"$book_id"}[15m])))`).
			LegendFormat("P95 {{book_id}}")).
		Unit("short").
		ColorScheme(dashboard.NewFieldColorBuilder().Mode(dashboard.FieldColorModeIdPaletteClassic))
}

// orderToFillQuantilesByBookStage shows order-to-fill latency per (book,
// stage). The sim accepts an order at the PlaceOrder instant, so decision
// and submit match there; a live adapter separates network time from time
// resting.
func orderToFillQuantilesByBookStage(y int) *timeseries.PanelBuilder {
	return timeseries.NewPanelBuilder().
		Id(1005).
		Title("Order-to-fill P50 / P95 by book and stage (15m)").
		Description(
			"paper-trading-realism phase-13: paper_order_to_fill_duration_seconds " +
				"on the adapter's clock. stage=decision runs from the PlaceOrder " +
				"call to the fill; stage=submit from the broker accepting the " +
				"order to the fill. The sim accepts at the PlaceOrder instant, so " +
				"the two stages match; a live adapter separates network time " +
				"from time resting. fillModel.latencyMs picks an older pricing quote " +
				"and does not move the fill time, so it does not show here; a " +
				"resting limit order records how long it rested. An immediate " +
				"MARKET fill is not 0: the interval runs from the order entering " +
				"the sim to the fill being stamped, and the sim writes the " +
				"Submitted and Acknowledged order events through to the database " +
				"in between (the fill's own persist comes after the fill time is " +
				"read and is not in it). The first readings on v0.16.0 " +
				"(2026-11-16) put the MARKET books at about 0.06-0.5 s at P50 " +
				"and P95: the two event write-throughs plus the sim's own work " +
				"in between (quote lookup, validation, pricing, risk checks); " +
				"the split between them has not been measured. Quantiles are " +
				"interpolated within histogram buckets, so values below 1 ms " +
				"are not resolved.").
		GridPos(layout.Pos(12, y, 12, 8)).
		Datasource(datasources.Victoria()).
		WithTarget(prometheus.NewDataqueryBuilder().
			RefId("A").
			Expr(`histogram_quantile(0.50, sum by (le, book_id, stage) (rate(paper_order_to_fill_duration_seconds_bucket{book_id=~"$book_id"}[15m])))`).
			LegendFormat("P50 {{book_id}} / {{stage}}")).
		WithTarget(prometheus.NewDataqueryBuilder().
			RefId("B").
			Expr(`histogram_quantile(0.95, sum by (le, book_id, stage) (rate(paper_order_to_fill_duration_seconds_bucket{book_id=~"$book_id"}[15m])))`).
			LegendFormat("P95 {{book_id}} / {{stage}}")).
		Unit("s").
		ColorScheme(dashboard.NewFieldColorBuilder().Mode(dashboard.FieldColorModeIdPaletteClassic))
}
