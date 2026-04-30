// market_data.go — quotes, change %, and live position glance.
// Renumbered: row 200, panels 201-205 (1 stat, 1 table, 1 timeseries,
// 1 bargauge, 1 piechart). The bargauge and piechart panel types are
// not present in paper-trading and are the schwab-feed port's first
// SDK-builder usage of either type.
// Pre-port hand-written ids did not exist (panels were anonymous).
package rows

import (
	"github.com/grafana/grafana-foundation-sdk/go/bargauge"
	"github.com/grafana/grafana-foundation-sdk/go/common"
	"github.com/grafana/grafana-foundation-sdk/go/dashboard"
	"github.com/grafana/grafana-foundation-sdk/go/piechart"
	"github.com/grafana/grafana-foundation-sdk/go/prometheus"
	"github.com/grafana/grafana-foundation-sdk/go/stat"
	"github.com/grafana/grafana-foundation-sdk/go/table"
	"github.com/grafana/grafana-foundation-sdk/go/timeseries"

	"github.com/Chalupa-Tech/chalupa-infra/scripts/dashboards/internal/common/datasources"
	"github.com/Chalupa-Tech/chalupa-infra/scripts/dashboards/internal/common/layout"
	"github.com/Chalupa-Tech/chalupa-infra/scripts/dashboards/internal/common/thresholds"
)

const (
	marketDataRowID    = 200
	marketDataRowTitle = "Market Data"
	marketDataHeight   = 27 // 1 (row) + 10 (stat/table) + 8 (ts) + 8 (bargauge/piechart)
)

func MarketData(db *dashboard.DashboardBuilder, yBase int) int {
	db.WithRow(layout.Row(marketDataRowID, yBase, marketDataRowTitle))
	db.WithPanel(dayPnL(yBase + 1))
	db.WithPanel(quotePrices(yBase + 1))
	db.WithPanel(priceChangeRolling(yBase + 11))
	db.WithPanel(positionDayPnL(yBase + 19))
	db.WithPanel(positionValues(yBase + 19))
	return marketDataHeight
}

// dayPnL — id 201. Account-wide day P&L stat (the only schwab_account_*
// metric on the market-data row).
func dayPnL(y int) *stat.PanelBuilder {
	steps := thresholds.Build([]thresholds.Step{
		{Color: "red", Value: nil},
		{Color: "green", Value: pp(0)},
	})
	return stat.NewPanelBuilder().
		Id(201).
		Title("Day P&L").
		GridPos(layout.Pos(0, y, 6, 4)).
		Datasource(datasources.Victoria()).
		WithTarget(prometheus.NewDataqueryBuilder().
			RefId("A").
			Expr(`max by (registration) (schwab_account_day_gain_loss{registration=~"$registration"})`).
			LegendFormat("day P&L")).
		Unit("currencyUSD").
		Decimals(2).
		Thresholds(steps).
		ColorMode(common.BigValueColorModeBackground).
		GraphMode(common.BigValueGraphModeNone).
		ReduceOptions(common.NewReduceDataOptionsBuilder().
			Calcs([]string{"lastNotNull"}))
}

// quotePrices — id 202. The widest panel on the dashboard: a 3-query
// instant-table join (price / change% / volume) keyed on `symbol`,
// with field-level overrides for unit + cell coloring on Change %.
//
// The seriesToColumns transformation merges three Prometheus instant
// frames (one per query) into a single row-per-symbol view; organize
// drops the per-query Time columns and renames the Value #A/B/C
// outputs to human-readable column titles. Overrides give Price a $
// unit, Volume a short unit, and Change % a percent unit + red→green
// color-background threshold.
func quotePrices(y int) *table.PanelBuilder {
	pricePromQL := `max by (symbol) (schwab_quote_price{registration=~"$registration", symbol=~"$symbol"})`
	changePromQL := `max by (symbol) (schwab_quote_change_pct{registration=~"$registration", symbol=~"$symbol"})`
	volumePromQL := `max by (symbol) (schwab_quote_volume{registration=~"$registration", symbol=~"$symbol"})`

	seriesToColumns := dashboard.DataTransformerConfig{
		Id:      "seriesToColumns",
		Options: map[string]any{"byField": "symbol"},
	}
	organize := dashboard.DataTransformerConfig{
		Id: "organize",
		Options: map[string]any{
			"renameByName": map[string]any{
				"Value #A": "Price",
				"Value #B": "Change %",
				"Value #C": "Volume",
			},
			"excludeByName": map[string]any{
				"Time":   true,
				"Time 1": true,
				"Time 2": true,
				"Time 3": true,
			},
		},
	}

	return table.NewPanelBuilder().
		Id(202).
		Title("Quote Prices").
		Description("Latest price for each symbol").
		GridPos(layout.Pos(6, y, 18, 10)).
		Datasource(datasources.Victoria()).
		WithTarget(prometheus.NewDataqueryBuilder().
			RefId("A").
			Expr(pricePromQL).
			LegendFormat("{{symbol}}").
			Instant().
			Format(prometheus.PromQueryFormatTable)).
		WithTarget(prometheus.NewDataqueryBuilder().
			RefId("B").
			Expr(changePromQL).
			LegendFormat("{{symbol}}").
			Instant().
			Format(prometheus.PromQueryFormatTable)).
		WithTarget(prometheus.NewDataqueryBuilder().
			RefId("C").
			Expr(volumePromQL).
			LegendFormat("{{symbol}}").
			Instant().
			Format(prometheus.PromQueryFormatTable)).
		WithTransformation(seriesToColumns).
		WithTransformation(organize).
		OverrideByName("Price", []dashboard.DynamicConfigValue{
			{Id: "unit", Value: "currencyUSD"},
			{Id: "decimals", Value: 2},
		}).
		OverrideByName("Change %", []dashboard.DynamicConfigValue{
			{Id: "unit", Value: "percent"},
			{Id: "decimals", Value: 2},
			{Id: "thresholds", Value: map[string]any{
				"mode": "absolute",
				"steps": []map[string]any{
					{"color": "red", "value": nil},
					{"color": "green", "value": 0},
				},
			}},
			{Id: "custom.cellOptions", Value: map[string]any{"type": "color-background"}},
		}).
		OverrideByName("Volume", []dashboard.DynamicConfigValue{
			{Id: "unit", Value: "short"},
			{Id: "decimals", Value: 0},
		})
}

// priceChangeRolling — id 203. 15-minute price drift, multi-symbol.
// palette-classic mandatory by feedback memory
// `reference_grafana_multi_series_coloring`: panels iterating over
// `$symbol` must use it, otherwise every series collides on the same
// hash color.
func priceChangeRolling(y int) *timeseries.PanelBuilder {
	steps := thresholds.Build([]thresholds.Step{
		{Color: "red", Value: nil},
		{Color: "green", Value: pp(0)},
	})
	return timeseries.NewPanelBuilder().
		Id(203).
		Title("Price Change % (15m rolling)").
		Description("Percentage price change over a rolling 15-minute window").
		GridPos(layout.Pos(0, y, 24, 8)).
		Datasource(datasources.Victoria()).
		WithTarget(prometheus.NewDataqueryBuilder().
			RefId("A").
			Expr(`100 * (max by (symbol) (schwab_quote_price{registration=~"$registration", symbol=~"$symbol"}) / max by (symbol) (schwab_quote_price{registration=~"$registration", symbol=~"$symbol"} offset 15m) - 1)`).
			LegendFormat("{{symbol}}")).
		Unit("percent").
		Decimals(2).
		LineWidth(2).
		FillOpacity(0).
		PointSize(4).
		ShowPoints(common.VisibilityModeAlways).
		Thresholds(steps).
		ColorScheme(dashboard.NewFieldColorBuilder().Mode(dashboard.FieldColorModeIdPaletteClassic)).
		Tooltip(common.NewVizTooltipOptionsBuilder().
			Mode(common.TooltipDisplayModeMulti).
			Sort(common.SortOrderDescending))
}

// positionDayPnL — id 204. First bargauge in the SDK ports. Horizontal
// gradient across each symbol's day gain/loss.
func positionDayPnL(y int) *bargauge.PanelBuilder {
	steps := thresholds.Build([]thresholds.Step{
		{Color: "red", Value: nil},
		{Color: "green", Value: pp(0)},
	})
	return bargauge.NewPanelBuilder().
		Id(204).
		Title("Position Day P&L").
		Description("Day gain/loss by position").
		GridPos(layout.Pos(0, y, 12, 8)).
		Datasource(datasources.Victoria()).
		WithTarget(prometheus.NewDataqueryBuilder().
			RefId("A").
			Expr(`max by (symbol) (schwab_position_day_gain_loss{registration=~"$registration", symbol=~"$symbol"})`).
			LegendFormat("{{symbol}}").
			Instant()).
		Unit("currencyUSD").
		Decimals(2).
		Thresholds(steps).
		Orientation(common.VizOrientationHorizontal).
		DisplayMode(common.BarGaugeDisplayModeGradient).
		ReduceOptions(common.NewReduceDataOptionsBuilder().
			Calcs([]string{"lastNotNull"}))
}

// positionValues — id 205. First piechart in the SDK ports. Allocates
// account equity across symbols by current position notional.
// `> 0` filter drops zero-quantity carryover series.
func positionValues(y int) *piechart.PanelBuilder {
	return piechart.NewPanelBuilder().
		Id(205).
		Title("Position Values").
		Description("Market value of each position").
		GridPos(layout.Pos(12, y, 12, 8)).
		Datasource(datasources.Victoria()).
		WithTarget(prometheus.NewDataqueryBuilder().
			RefId("A").
			Expr(`max by (symbol) (schwab_position_value{registration=~"$registration", symbol=~"$symbol"}) > 0`).
			LegendFormat("{{symbol}}").
			Instant()).
		Unit("currencyUSD").
		Decimals(0).
		ColorScheme(dashboard.NewFieldColorBuilder().Mode(dashboard.FieldColorModeIdPaletteClassic)).
		Legend(piechart.NewPieChartLegendOptionsBuilder().
			ShowLegend(true).
			DisplayMode(common.LegendDisplayModeTable).
			Placement(common.LegendPlacementRight)).
		ReduceOptions(common.NewReduceDataOptionsBuilder().
			Calcs([]string{"lastNotNull"}))
}
