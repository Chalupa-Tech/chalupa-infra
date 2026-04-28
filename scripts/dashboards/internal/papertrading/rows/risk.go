// risk.go — daily P&L per book, halts, halt-reject rate, daily-loss limit.
// Renumbered: row 300, panels 301-304.
// (Phase-7b ids: row 30, panels 31-34.)
package rows

import (
	"github.com/grafana/grafana-foundation-sdk/go/common"
	"github.com/grafana/grafana-foundation-sdk/go/dashboard"
	"github.com/grafana/grafana-foundation-sdk/go/prometheus"
	"github.com/grafana/grafana-foundation-sdk/go/stat"
	"github.com/grafana/grafana-foundation-sdk/go/timeseries"

	"github.com/Chalupa-Tech/chalupa-infra/scripts/dashboards/internal/common/datasources"
	"github.com/Chalupa-Tech/chalupa-infra/scripts/dashboards/internal/common/layout"
	"github.com/Chalupa-Tech/chalupa-infra/scripts/dashboards/internal/common/thresholds"
)

const (
	riskRowID    = 300
	riskRowTitle = "Risk"
	riskHeight   = 10 // 1 (row header) + 9 (id=301 timeseries h)
)

func Risk(db *dashboard.DashboardBuilder, yBase int) int {
	db.WithRow(layout.Row(riskRowID, yBase, riskRowTitle))
	db.WithPanel(dailyPnLPerBook(yBase + 1))
	db.WithPanel(booksHalted(yBase + 1))
	db.WithPanel(haltRejectRate(yBase + 1))
	db.WithPanel(dailyLossLimit(yBase + 5))
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
			Calcs([]string{"lastNotNull"}))
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
