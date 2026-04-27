// risk.go — daily P&L per book vs daily-loss-limit threshold line.
// Phase-7b panel ids: row 30, panels 31-34.
// Phase-7e slice: ports id=31 ("Daily P&L per book") — the
// field-override canary (byRegexp + lineStyle.dash + fixedColor on
// the limit-line series).
package rows

import (
	"github.com/grafana/grafana-foundation-sdk/go/common"
	"github.com/grafana/grafana-foundation-sdk/go/dashboard"
	"github.com/grafana/grafana-foundation-sdk/go/prometheus"
	"github.com/grafana/grafana-foundation-sdk/go/timeseries"

	"github.com/Chalupa-Tech/chalupa-infra/scripts/dashboards/internal/common/datasources"
	"github.com/Chalupa-Tech/chalupa-infra/scripts/dashboards/internal/common/layout"
)

const (
	riskRowID    = 30
	riskRowTitle = "Risk"
	riskHeight   = 10 // 1 (row header) + 9 (timeseries h)
)

func Risk(db *dashboard.DashboardBuilder, yBase int) int {
	db.WithRow(layout.Row(riskRowID, yBase, riskRowTitle))
	db.WithPanel(dailyPnLPerBook(yBase + 1))
	return riskHeight
}

func dailyPnLPerBook(y int) *timeseries.PanelBuilder {
	return timeseries.NewPanelBuilder().
		Id(31).
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
