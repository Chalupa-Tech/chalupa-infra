// risk.go — daily P&L per book, halts, halt-reject rate, daily-loss
// limit. Phase-9 added pre-trade-reject rate by reason and per-book
// risk-gate config.
// Renumbered: row 300, panels 301-306.
// (Phase-7b ids: row 30, panels 31-34. Phase-9 added 305/306.)
package rows

import (
	"github.com/grafana/grafana-foundation-sdk/go/cog"
	"github.com/grafana/grafana-foundation-sdk/go/common"
	"github.com/grafana/grafana-foundation-sdk/go/dashboard"
	"github.com/grafana/grafana-foundation-sdk/go/prometheus"
	"github.com/grafana/grafana-foundation-sdk/go/stat"
	"github.com/grafana/grafana-foundation-sdk/go/timeseries"

	"github.com/Chalupa-Tech/chalupa-infra/scripts/dashboards/internal/common/datasources"
	"github.com/Chalupa-Tech/chalupa-infra/scripts/dashboards/internal/common/layout"
	"github.com/Chalupa-Tech/chalupa-infra/scripts/dashboards/internal/common/links"
	"github.com/Chalupa-Tech/chalupa-infra/scripts/dashboards/internal/common/thresholds"
)

const (
	riskRowID    = 300
	riskRowTitle = "Risk"
	// 1 (row header) + 9 (panel 301 h) + 8 (phase-9 panels 305/306 h).
	riskHeight = 18
)

func Risk(db *dashboard.DashboardBuilder, yBase int) int {
	db.WithRow(layout.Row(riskRowID, yBase, riskRowTitle))
	db.WithPanel(dailyPnLPerBook(yBase + 1))
	db.WithPanel(booksHalted(yBase + 1))
	db.WithPanel(haltRejectRate(yBase + 1))
	db.WithPanel(dailyLossLimit(yBase + 5))
	// Phase-9: pre-trade-reject rate + per-book risk gate config.
	db.WithPanel(preTradeRejectsByReason(yBase + 10))
	db.WithPanel(riskGateConfig(yBase + 10))
	return riskHeight
}

func dailyPnLPerBook(y int) *timeseries.PanelBuilder {
	return timeseries.NewPanelBuilder().
		Id(301).
		Title("Daily P&L per book (USD, UTC day)").
		Description(
			"paper-trading-realism phase-6: paper_daily_pnl_usd resets at " +
				"UTC midnight. Red threshold line is -paper_daily_loss_limit_usd " +
				"— the point at which PlaceOrder refuses new orders until the " +
				"next UTC day. Useful as a pre-halt early warning when a book " +
				"approaches its floor.").
		GridPos(layout.Pos(0, y, 12, 9)).
		Datasource(datasources.Victoria()).
		WithTarget(prometheus.NewDataqueryBuilder().
			RefId("A").
			Expr(`paper_daily_pnl_usd{book_id=~"$book_id"}`).
			LegendFormat("{{book_id}}")).
		WithTarget(prometheus.NewDataqueryBuilder().
			RefId("B").
			Expr(`-paper_daily_loss_limit_usd{book_id=~"$book_id"}`).
			LegendFormat("limit {{book_id}}")).
		Unit("currencyUSD").
		ColorScheme(dashboard.NewFieldColorBuilder().Mode(dashboard.FieldColorModeIdPaletteClassic)).
		DrawStyle(common.GraphDrawStyleLine).
		LineWidth(2).
		FillOpacity(10).
		Legend(common.NewVizLegendOptionsBuilder().
			DisplayMode(common.LegendDisplayModeList).
			Placement(common.LegendPlacementBottom).
			ShowLegend(true)).
		// The override canary: every "limit X" series renders as a
		// dashed red line with no fill, so the threshold is visually
		// distinct from the actual P&L lines.
		OverrideByRegexp("^limit .*", []dashboard.DynamicConfigValue{
			{Id: "custom.lineStyle", Value: map[string]any{"fill": "dash", "dash": []int{8, 4}}},
			{Id: "custom.fillOpacity", Value: 0},
			{Id: "color", Value: map[string]any{"mode": "fixed", "fixedColor": "red"}},
		})
}

func booksHalted(y int) *stat.PanelBuilder {
	return stat.NewPanelBuilder().
		Id(302).
		Title("Books halted (today)").
		Description(
			"paper-trading-realism phase-6: number of books that have hit the " +
				"daily max-loss guard at least once in the current 24h window. " +
				"Red when any book is halted; green otherwise. A halted book " +
				"recovers at UTC midnight without intervention.").
		GridPos(layout.Pos(12, y, 6, 4)).
		Datasource(datasources.Victoria()).
		WithTarget(prometheus.NewDataqueryBuilder().
			RefId("A").
			Expr(`count(increase(paper_daily_loss_halt_total{book_id=~"$book_id"}[24h]) > 0) or vector(0)`).
			Instant()).
		Thresholds(thresholds.Build(thresholds.BooksHalted)).
		ColorMode(common.BigValueColorModeBackground).
		GraphMode(common.BigValueGraphModeNone).
		ReduceOptions(common.NewReduceDataOptionsBuilder().
			Calcs([]string{"lastNotNull"})).
		// Drilldown #5: a halt may be downstream of stale data that's
		// downstream of a failed token refresh — or of a cluster-latency
		// fill-decision lag. Two candidate root causes, two links;
		// Grafana renders both as a sub-menu off the panel.
		DataLinks([]cog.Builder[dashboard.DashboardLink]{
			links.To(
				"schwab-auth-lifecycle",
				"/d/schwab-auth-lifecycle?from=${__from}&to=${__to}",
			),
			links.To(
				"telemetry-mesh Cross-site P99",
				"/d/telemetry-mesh?from=${__from}&to=${__to}",
			),
		})
}

func haltRejectRate(y int) *timeseries.PanelBuilder {
	return timeseries.NewPanelBuilder().
		Id(303).
		Title("Halt reject rate per book (5m)").
		Description(
			"paper-trading-realism phase-6: rate(paper_daily_loss_halt_total[5m]) " +
				"per book. Non-zero = the strategy is still calling PlaceOrder " +
				"against a halted book. Expected briefly on first halt (strategy " +
				"catches up), sustained high rate means the strategy is " +
				"hot-looping without a back-off.").
		GridPos(layout.Pos(18, y, 6, 5)).
		Datasource(datasources.Victoria()).
		WithTarget(prometheus.NewDataqueryBuilder().
			RefId("A").
			Expr(`rate(paper_daily_loss_halt_total{book_id=~"$book_id"}[5m])`).
			LegendFormat("{{book_id}}")).
		Unit("ops").
		ColorScheme(dashboard.NewFieldColorBuilder().Mode(dashboard.FieldColorModeIdPaletteClassic)).
		LineWidth(2).
		FillOpacity(10).
		Legend(common.NewVizLegendOptionsBuilder().
			DisplayMode(common.LegendDisplayModeList).
			Placement(common.LegendPlacementBottom))
}

func dailyLossLimit(y int) *stat.PanelBuilder {
	return stat.NewPanelBuilder().
		Id(304).
		Title("Daily loss limit per book").
		Description(
			"paper-trading-realism phase-6: paper_daily_loss_limit_usd — " +
				"configured per-book floor (halting threshold is -limit). Zero " +
				"means the guard is disabled for that book.").
		GridPos(layout.Pos(12, y, 6, 5)).
		Datasource(datasources.Victoria()).
		WithTarget(prometheus.NewDataqueryBuilder().
			RefId("A").
			Expr(`paper_daily_loss_limit_usd{book_id=~"$book_id"}`).
			LegendFormat("{{book_id}}").
			Instant()).
		Unit("currencyUSD").
		ColorMode(common.BigValueColorModeValue).
		GraphMode(common.BigValueGraphModeNone).
		ReduceOptions(common.NewReduceDataOptionsBuilder().
			Calcs([]string{"lastNotNull"}))
}

// preTradeRejectsByReason renders the rate of orders that ended in
// state=rejected with a phase-9 risk reason — partitioned by reason
// so a sudden spike in (e.g.) risk_buying_power is visually distinct
// from risk_max_order_qty. Filters on reject_reason=~"risk_.*" so the
// panel stays focused on adapter-level pre-trade gates rather than
// bleeding in stale_quote / market_closed / daily_loss_halt rejections
// (those have their own visualizations).
func preTradeRejectsByReason(y int) *timeseries.PanelBuilder {
	return timeseries.NewPanelBuilder().
		Id(305).
		Title("Pre-trade rejects per reason (5m rate)").
		Description(
			"paper-trading-realism phase-9: rate of orders rejected by a " +
				"pre-trade risk gate, partitioned by reason. risk_max_order_qty " +
				"= fat-finger qty cap. risk_max_notional = single-order $ cap. " +
				"risk_buying_power = BUY notional > cash. risk_max_position = " +
				"BUY would push long qty over the per-symbol cap. A non-zero " +
				"sustained rate means a strategy is hot-looping against a cap " +
				"or the cap is misconfigured.").
		GridPos(layout.Pos(0, y, 12, 8)).
		Datasource(datasources.Victoria()).
		WithTarget(prometheus.NewDataqueryBuilder().
			RefId("A").
			Expr(`sum by (reject_reason) (rate(paper_order_rejects_total{book_id=~"$book_id", reject_reason=~"risk_.*"}[5m]))`).
			LegendFormat("{{reject_reason}}")).
		Unit("ops").
		ColorScheme(dashboard.NewFieldColorBuilder().Mode(dashboard.FieldColorModeIdPaletteClassic)).
		DrawStyle(common.GraphDrawStyleLine).
		LineWidth(2).
		FillOpacity(20).
		Legend(common.NewVizLegendOptionsBuilder().
			DisplayMode(common.LegendDisplayModeList).
			Placement(common.LegendPlacementBottom).
			ShowLegend(true))
}

// riskGateConfig renders the four phase-9 risk-gate gauges side-by-
// side per book so the operator can confirm the running config without
// reading YAML. Zero on a numeric gauge means the gate is disabled
// (matches the >0 enable predicate inside risk.Check). The
// buying-power gauge is 0/1; rendered as a separate stat panel would
// double the visual real estate, so all four metrics share one panel
// with a "limit" / "required" mixed-unit reading. Multi-series so the
// $book_id selector cleanly distinguishes books — palette-classic per
// reference_grafana_multi_series_coloring.
func riskGateConfig(y int) *stat.PanelBuilder {
	return stat.NewPanelBuilder().
		Id(306).
		Title("Risk gate config per book").
		Description(
			"paper-trading-realism phase-9: configured pre-trade risk caps " +
				"per book. max_order_qty / max_notional_usd / " +
				"max_position_qty_per_symbol — 0 means the gate is " +
				"disabled. buying_power_required is 0 (off) or 1 (on). Books " +
				"with all gates at 0 + buying_power_required=0 are running " +
				"unguarded — fine for back-compat tests, intentional " +
				"opt-out for some books.").
		GridPos(layout.Pos(12, y, 12, 8)).
		Datasource(datasources.Victoria()).
		WithTarget(prometheus.NewDataqueryBuilder().
			RefId("A").
			Expr(`paper_risk_max_order_qty{book_id=~"$book_id"}`).
			LegendFormat("max_order_qty / {{book_id}}").
			Instant()).
		WithTarget(prometheus.NewDataqueryBuilder().
			RefId("B").
			Expr(`paper_risk_max_notional_per_order_usd{book_id=~"$book_id"}`).
			LegendFormat("max_notional_usd / {{book_id}}").
			Instant()).
		WithTarget(prometheus.NewDataqueryBuilder().
			RefId("C").
			Expr(`paper_risk_max_position_qty_per_symbol{book_id=~"$book_id"}`).
			LegendFormat("max_position_qty / {{book_id}}").
			Instant()).
		WithTarget(prometheus.NewDataqueryBuilder().
			RefId("D").
			Expr(`paper_risk_buying_power_required{book_id=~"$book_id"}`).
			LegendFormat("buying_power_required / {{book_id}}").
			Instant()).
		ColorScheme(dashboard.NewFieldColorBuilder().Mode(dashboard.FieldColorModeIdPaletteClassic)).
		ColorMode(common.BigValueColorModeValue).
		GraphMode(common.BigValueGraphModeNone).
		ReduceOptions(common.NewReduceDataOptionsBuilder().
			Calcs([]string{"lastNotNull"}))
}
