// orders.go — order lifecycle state machine: pending counts, reject
// rate by reason, transition rate by (from, to). Phase-8.
// Row 850, panels 851-853.
package rows

import (
	"github.com/grafana/grafana-foundation-sdk/go/common"
	"github.com/grafana/grafana-foundation-sdk/go/dashboard"
	"github.com/grafana/grafana-foundation-sdk/go/prometheus"
	"github.com/grafana/grafana-foundation-sdk/go/stat"
	"github.com/grafana/grafana-foundation-sdk/go/timeseries"

	"github.com/Chalupa-Tech/chalupa-infra/scripts/dashboards/internal/common/datasources"
	"github.com/Chalupa-Tech/chalupa-infra/scripts/dashboards/internal/common/layout"
)

const (
	ordersRowID    = 850
	ordersRowTitle = "Orders"
	ordersHeight   = 9 // 1 (row header) + 8 (panel h)
)

// Orders renders the phase-8 order-lifecycle row: how many orders
// are pending, why orders are getting rejected, and how state
// transitions are flowing. The interesting steady state is "low
// pending count, low rejection rate"; spikes mean either market
// conditions stressed validation (stale_quote during a feed gap) or
// a strategy bug submitted impossible orders (insufficient_long).
func Orders(db *dashboard.DashboardBuilder, yBase int) int {
	db.WithRow(layout.Row(ordersRowID, yBase, ordersRowTitle))
	db.WithPanel(pendingOrders(yBase + 1))
	db.WithPanel(rejectionRateByReason(yBase + 1))
	db.WithPanel(stateTransitionRate(yBase + 1))
	return ordersHeight
}

func pendingOrders(y int) *stat.PanelBuilder {
	return stat.NewPanelBuilder().
		Id(851).
		Title("Pending orders").
		Description(
			"paper-trading-realism phase-8: count of orders currently in a " +
				"non-terminal lifecycle state (submitted | acknowledged | partial). " +
				"v1 sim collapses the full Submitted→Acknowledged→Filled sequence " +
				"inside one synchronous PlaceOrder call, so this panel reads as " +
				"\"orders the strategy submitted that haven't reached Filled yet\" " +
				"— typically zero. Phase-10 limit orders will linger in " +
				"acknowledged for as long as the limit doesn't cross the market.").
		GridPos(layout.Pos(0, y, 8, 8)).
		Datasource(datasources.Victoria()).
		WithTarget(prometheus.NewDataqueryBuilder().
			RefId("A").
			Expr(`sum by (book_id) (paper_order_state_gauge{state=~"submitted|acknowledged|partial", book_id=~"$book_id"})`).
			LegendFormat("{{book_id}}").
			Instant()).
		Unit("short").
		ColorMode(common.BigValueColorModeValue).
		GraphMode(common.BigValueGraphModeNone).
		ReduceOptions(common.NewReduceDataOptionsBuilder().
			Calcs([]string{"lastNotNull"}))
}

func rejectionRateByReason(y int) *timeseries.PanelBuilder {
	return timeseries.NewPanelBuilder().
		Id(852).
		Title("Rejection rate by reason (5m)").
		Description(
			"paper-trading-realism phase-8: rate(paper_order_rejects_total[5m]) " +
				"split by reject_reason. Bounded enum: stale_quote (no fresh " +
				"quote available), market_closed (book's session window doesn't " +
				"include now), daily_loss_halt (book breached its UTC-day limit), " +
				"insufficient_long (SELL > long, v1 has no shorting), " +
				"append_fill (DB persistence error), validation (unsupported " +
				"instruction or zero-priced quote). Steady state is flat-zero; " +
				"any bar means orders that the strategy fired could not land.").
		GridPos(layout.Pos(8, y, 8, 8)).
		Datasource(datasources.Victoria()).
		WithTarget(prometheus.NewDataqueryBuilder().
			RefId("A").
			Expr(`sum by (reject_reason) (rate(paper_order_rejects_total{book_id=~"$book_id"}[5m]))`).
			LegendFormat("{{reject_reason}}")).
		Unit("ops").
		ColorScheme(dashboard.NewFieldColorBuilder().Mode(dashboard.FieldColorModeIdPaletteClassic)).
		DrawStyle(common.GraphDrawStyleLine).
		LineWidth(2).
		FillOpacity(10).
		Legend(common.NewVizLegendOptionsBuilder().
			DisplayMode(common.LegendDisplayModeList).
			Placement(common.LegendPlacementBottom).
			ShowLegend(true))
}

func stateTransitionRate(y int) *timeseries.PanelBuilder {
	return timeseries.NewPanelBuilder().
		Id(853).
		Title("State transitions (5m)").
		Description(
			"paper-trading-realism phase-8: rate(paper_order_state_transitions_total[5m]) " +
				"split by (from_state → to_state). Healthy operation: " +
				"three matching curves at the same magnitude — →submitted, " +
				"submitted→acknowledged, acknowledged→filled. A " +
				"submitted→rejected curve climbing while acknowledged→filled " +
				"falls means validation is rejecting work the strategy still " +
				"thinks is succeeding.").
		GridPos(layout.Pos(16, y, 8, 8)).
		Datasource(datasources.Victoria()).
		WithTarget(prometheus.NewDataqueryBuilder().
			RefId("A").
			Expr(`sum by (from_state, to_state) (rate(paper_order_state_transitions_total{book_id=~"$book_id"}[5m]))`).
			LegendFormat("{{from_state}}→{{to_state}}")).
		Unit("ops").
		ColorScheme(dashboard.NewFieldColorBuilder().Mode(dashboard.FieldColorModeIdPaletteClassic)).
		DrawStyle(common.GraphDrawStyleLine).
		LineWidth(2).
		FillOpacity(10).
		Legend(common.NewVizLegendOptionsBuilder().
			DisplayMode(common.LegendDisplayModeList).
			Placement(common.LegendPlacementBottom).
			ShowLegend(true))
}
