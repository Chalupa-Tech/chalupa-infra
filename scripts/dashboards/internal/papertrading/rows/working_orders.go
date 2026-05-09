// working_orders.go — phase-10 expansion of the order surface:
// stacked timeseries of Acknowledged orders by type, distribution
// of limit-fill price improvement, and a table of working orders
// with age. Row 870, panels 871-873.
//
// The Orders row above (850) covers lifecycle health: pending count,
// rejection rate, transition rate. This row covers the *catalogue*:
// what's resting right now and what kind of fills the limits are
// getting. Designed to render as the operator's first answer to
// "did my LIMIT actually go in?" without leaving the Paper Trading
// dashboard.
package rows

import (
	"github.com/grafana/grafana-foundation-sdk/go/common"
	"github.com/grafana/grafana-foundation-sdk/go/dashboard"
	"github.com/grafana/grafana-foundation-sdk/go/heatmap"
	"github.com/grafana/grafana-foundation-sdk/go/prometheus"
	"github.com/grafana/grafana-foundation-sdk/go/table"
	"github.com/grafana/grafana-foundation-sdk/go/timeseries"

	"github.com/Chalupa-Tech/chalupa-infra/scripts/dashboards/internal/common/datasources"
	"github.com/Chalupa-Tech/chalupa-infra/scripts/dashboards/internal/common/layout"
	"github.com/Chalupa-Tech/chalupa-infra/scripts/dashboards/internal/common/sql"
)

const (
	workingOrdersRowID    = 870
	workingOrdersRowTitle = "Working orders (phase-10)"
	workingOrdersHeight   = 9 // 1 (row header) + 8 (panel h)
)

// WorkingOrders renders the phase-10 order-types row: open orders by
// type, limit-fill improvement distribution, and a per-order working
// table. Steady state is "low + flat" — a strategy that uses LIMIT
// orders at all will keep some non-zero acknowledged count during the
// interval between submission and fill, but a sustained climb means
// the strategy is submitting limits the market is never crossing
// (badly priced) or the EOD cancel gate is failing.
func WorkingOrders(db *dashboard.DashboardBuilder, yBase int) int {
	db.WithRow(layout.Row(workingOrdersRowID, yBase, workingOrdersRowTitle))
	db.WithPanel(openOrdersByType(yBase + 1))
	db.WithPanel(limitFillImprovement(yBase + 1))
	db.WithPanel(workingOrdersTable(yBase + 1))
	return workingOrdersHeight
}

// openOrdersByType is the stacked-timeseries view of paper_orders_by_type
// filtered to non-terminal states. The interesting buckets are
// LIMIT/acknowledged (resting limits), STOP/acknowledged (resting
// stops), and STOP_LIMIT/acknowledged (resting stop-limits, both
// pre- and post-trigger — the gauge does not split on the
// stop_triggered flag because the acknowledged count is what an
// operator wants to see at a glance; drill into the table panel for
// per-order detail).
func openOrdersByType(y int) *timeseries.PanelBuilder {
	return timeseries.NewPanelBuilder().
		Id(871).
		Title("Open orders by type").
		Description(
			"paper-trading-realism phase-10: paper_orders_by_type filtered to " +
				"non-terminal states (acknowledged | partial). Stacks by " +
				"order_type so a sudden climb in LIMIT acknowledges shows " +
				"up as a growing area without the operator hunting through " +
				"individual queries. Steady state for a market-only " +
				"strategy (alternator/SMA) is flat zero; a limit-using " +
				"strategy oscillates as orders rest and fill.").
		GridPos(layout.Pos(0, y, 8, 8)).
		Datasource(datasources.Victoria()).
		WithTarget(prometheus.NewDataqueryBuilder().
			RefId("A").
			Expr(`sum by (order_type) (paper_orders_by_type{state=~"acknowledged|partial", book_id=~"$book_id"})`).
			LegendFormat("{{order_type}}")).
		Unit("short").
		ColorScheme(dashboard.NewFieldColorBuilder().Mode(dashboard.FieldColorModeIdPaletteClassic)).
		DrawStyle(common.GraphDrawStyleLine).
		LineWidth(2).
		FillOpacity(40). // stacked area
		Stacking(common.NewStackingConfigBuilder().Mode(common.StackingModeNormal)).
		Legend(common.NewVizLegendOptionsBuilder().
			DisplayMode(common.LegendDisplayModeList).
			Placement(common.LegendPlacementBottom).
			ShowLegend(true))
}

// limitFillImprovement renders paper_limit_fill_improvement_bps as a
// heatmap. v1 sim fills LIMITs at the limit price exactly so the
// distribution collapses to a single bucket at 0bps; the panel
// exists to surface non-zero distribution as soon as a future phase
// (phase-14 SchwabAdapter live, or a phase-10β price-improvement
// model) starts emitting it. Sub-zero buckets are a regression
// signal — sim should never fill worse than the limit demanded.
func limitFillImprovement(y int) *heatmap.PanelBuilder {
	return heatmap.NewPanelBuilder().
		Id(872).
		Title("Limit-fill price improvement (bps)").
		Description(
			"paper-trading-realism phase-10: paper_limit_fill_improvement_bps " +
				"distribution per (book, symbol). Positive = the book got a " +
				"fill BETTER than the limit demanded (real exchanges sometimes " +
				"give price improvement). v1 sim fills at limit exactly, so the " +
				"current steady state is 100% mass at the 0-bps bucket — " +
				"phase-14 (SchwabAdapter) or a future improvement model would " +
				"populate the right tail. Negative buckets are a regression " +
				"signal (sim must never fill worse than the limit).").
		GridPos(layout.Pos(8, y, 8, 8)).
		Datasource(datasources.Victoria()).
		WithTarget(prometheus.NewDataqueryBuilder().
			RefId("A").
			Expr(`sum by (le) (rate(paper_limit_fill_improvement_bps_bucket{book_id=~"$book_id"}[15m]))`).
			Format("heatmap").
			LegendFormat("{{le}}"))
}

// workingOrdersTable lists every order in paper_orders that is in
// state='acknowledged' (or 'partial' once partial fills exist), with
// the time it has been working. Drives the operator's "what's
// outstanding right now?" question. Sorted by created_at DESC so the
// freshest orders are at the top.
//
// $now() - created_at uses Postgres' interval arithmetic so the panel
// renders without depending on the dashboard's time range — working
// orders are about right-now, not historical replay. Filtering by
// $book_id keeps the table scoped to the books the operator selected;
// the strategy variable is omitted intentionally because an operator
// debugging a hung order may not know which strategy owns it yet.
func workingOrdersTable(y int) *table.PanelBuilder {
	rawSQL := "SELECT client_order_id, book_id, symbol, side, order_type, " +
		"time_in_force, COALESCE(limit_price, NULL) AS limit_price, " +
		"COALESCE(stop_price, NULL) AS stop_price, stop_triggered, " +
		"quantity, filled_quantity, created_at, " +
		"NOW() - created_at AS age " +
		"FROM paper_orders " +
		"WHERE state = 'acknowledged' AND book_id IN ($book_id) " +
		"ORDER BY created_at DESC LIMIT 50"
	return table.NewPanelBuilder().
		Id(873).
		Title("Working orders").
		Description(
			"paper-trading-realism phase-10: every paper_orders row in " +
				"state='acknowledged' for the selected books. Age is " +
				"NOW() - created_at — a working order whose age exceeds the " +
				"strategy's expected fill window means either (a) the limit is " +
				"badly priced or (b) the EOD-cancel pass missed a DAY order. " +
				"The view excludes filled/canceled/rejected so it doesn't " +
				"flood with terminal rows; for full lifecycle history, query " +
				"paper_order_events directly.").
		GridPos(layout.Pos(16, y, 8, 8)).
		Datasource(datasources.Timescale()).
		WithTarget(sql.Table("A", rawSQL)).
		Align(common.FieldTextAlignmentRight).
		OverrideByName("limit_price", []dashboard.DynamicConfigValue{{Id: "unit", Value: "currencyUSD"}}).
		OverrideByName("stop_price", []dashboard.DynamicConfigValue{{Id: "unit", Value: "currencyUSD"}})
}
