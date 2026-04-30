// feed_health.go — feed plumbing & account header.
// Renumbered: row 100, panels 101-107 (5 stat tiles + 2 timeseries).
// Pre-port hand-written ids did not exist (panels were anonymous).
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
	feedHealthRowID    = 100
	feedHealthRowTitle = "Feed Health"
	feedHealthHeight   = 11 // 1 (row) + 4 (stat strip) + 6 (timeseries)
)

func FeedHealth(db *dashboard.DashboardBuilder, yBase int) int {
	db.WithRow(layout.Row(feedHealthRowID, yBase, feedHealthRowTitle))
	db.WithPanel(feedStatus(yBase + 1))
	db.WithPanel(watchlistSize(yBase + 1))
	db.WithPanel(natsPublishesPerMin(yBase + 1))
	db.WithPanel(pollErrors5m(yBase + 1))
	db.WithPanel(accountValue(yBase + 1))
	db.WithPanel(pollDuration(yBase + 5))
	db.WithPanel(natsPublishRate(yBase + 5))
	return feedHealthHeight
}

// pp returns a pointer to the given float64 — sugar for inline
// threshold steps.
func pp(v float64) *float64 { return &v }

func feedStatus(y int) *stat.PanelBuilder {
	steps := thresholds.Build([]thresholds.Step{
		{Color: "green", Value: nil},
		{Color: "yellow", Value: pp(600)},
		{Color: "red", Value: pp(1800)},
	})
	return stat.NewPanelBuilder().
		Id(101).
		Title("Feed Status").
		Description("Time since last successful poll by data type").
		GridPos(layout.Pos(0, y, 6, 4)).
		Datasource(datasources.Victoria()).
		WithTarget(prometheus.NewDataqueryBuilder().
			RefId("A").
			Expr(`time() - max by (type) (schwab_feed_last_poll_timestamp{registration=~"$registration"})`).
			LegendFormat("{{type}}")).
		Unit("s").
		Thresholds(steps).
		ColorMode(common.BigValueColorModeBackground).
		GraphMode(common.BigValueGraphModeNone).
		ReduceOptions(common.NewReduceDataOptionsBuilder().
			Calcs([]string{"lastNotNull"}))
}

func watchlistSize(y int) *stat.PanelBuilder {
	steps := thresholds.Build([]thresholds.Step{
		{Color: "blue", Value: nil},
	})
	return stat.NewPanelBuilder().
		Id(102).
		Title("Watchlist Size").
		GridPos(layout.Pos(6, y, 3, 4)).
		Datasource(datasources.Victoria()).
		WithTarget(prometheus.NewDataqueryBuilder().
			RefId("A").
			Expr(`max by (registration) (schwab_feed_watchlist_size{registration=~"$registration"})`).
			LegendFormat("symbols")).
		Thresholds(steps).
		ColorMode(common.BigValueColorModeBackground).
		GraphMode(common.BigValueGraphModeNone).
		ReduceOptions(common.NewReduceDataOptionsBuilder().
			Calcs([]string{"lastNotNull"}))
}

func natsPublishesPerMin(y int) *stat.PanelBuilder {
	steps := thresholds.Build([]thresholds.Step{
		{Color: "green", Value: nil},
		{Color: "yellow", Value: pp(0.1)},
	})
	return stat.NewPanelBuilder().
		Id(103).
		Title("NATS Publishes / min").
		GridPos(layout.Pos(9, y, 5, 4)).
		Datasource(datasources.Victoria()).
		WithTarget(prometheus.NewDataqueryBuilder().
			RefId("A").
			Expr(`sum(rate(schwab_feed_nats_publishes_total{registration=~"$registration"}[5m])) * 60`).
			LegendFormat("pub/min")).
		Decimals(1).
		Thresholds(steps).
		ColorMode(common.BigValueColorModeBackground).
		GraphMode(common.BigValueGraphModeNone).
		ReduceOptions(common.NewReduceDataOptionsBuilder().
			Calcs([]string{"lastNotNull"}))
}

func pollErrors5m(y int) *stat.PanelBuilder {
	steps := thresholds.Build([]thresholds.Step{
		{Color: "green", Value: nil},
		{Color: "yellow", Value: pp(1)},
		{Color: "red", Value: pp(5)},
	})
	return stat.NewPanelBuilder().
		Id(104).
		Title("Poll Errors / 5m").
		GridPos(layout.Pos(14, y, 5, 4)).
		Datasource(datasources.Victoria()).
		WithTarget(prometheus.NewDataqueryBuilder().
			RefId("A").
			Expr(`sum(increase(schwab_feed_errors_total{registration=~"$registration"}[5m]))`).
			LegendFormat("errors")).
		Decimals(0).
		Thresholds(steps).
		ColorMode(common.BigValueColorModeBackground).
		GraphMode(common.BigValueGraphModeNone).
		ReduceOptions(common.NewReduceDataOptionsBuilder().
			Calcs([]string{"lastNotNull"}))
}

func accountValue(y int) *stat.PanelBuilder {
	steps := thresholds.Build([]thresholds.Step{
		{Color: "blue", Value: nil},
	})
	return stat.NewPanelBuilder().
		Id(105).
		Title("Account Value").
		GridPos(layout.Pos(19, y, 5, 4)).
		Datasource(datasources.Victoria()).
		WithTarget(prometheus.NewDataqueryBuilder().
			RefId("A").
			Expr(`max by (registration) (schwab_account_value{registration=~"$registration"})`).
			LegendFormat("total")).
		Unit("currencyUSD").
		Decimals(2).
		Thresholds(steps).
		ColorMode(common.BigValueColorModeBackground).
		GraphMode(common.BigValueGraphModeNone).
		ReduceOptions(common.NewReduceDataOptionsBuilder().
			Calcs([]string{"lastNotNull"}))
}

func pollDuration(y int) *timeseries.PanelBuilder {
	return timeseries.NewPanelBuilder().
		Id(106).
		Title("Poll Duration").
		Description("Time to complete each poll cycle").
		GridPos(layout.Pos(0, y, 12, 6)).
		Datasource(datasources.Victoria()).
		WithTarget(prometheus.NewDataqueryBuilder().
			RefId("A").
			Expr(`sum by (type) (schwab_feed_poll_duration_seconds_sum{registration=~"$registration"}) / sum by (type) (schwab_feed_poll_duration_seconds_count{registration=~"$registration"})`).
			LegendFormat("{{type}} avg")).
		Unit("s").
		LineWidth(2).
		FillOpacity(10).
		ColorScheme(dashboard.NewFieldColorBuilder().Mode(dashboard.FieldColorModeIdPaletteClassic))
}

func natsPublishRate(y int) *timeseries.PanelBuilder {
	return timeseries.NewPanelBuilder().
		Id(107).
		Title("NATS Publish Rate").
		Description("Messages published to NATS per minute by type").
		GridPos(layout.Pos(12, y, 12, 6)).
		Datasource(datasources.Victoria()).
		WithTarget(prometheus.NewDataqueryBuilder().
			RefId("A").
			Expr(`sum by (type) (rate(schwab_feed_nats_publishes_total{registration=~"$registration"}[5m])) * 60`).
			LegendFormat("{{type}}")).
		Unit("msg/min").
		LineWidth(2).
		FillOpacity(20).
		Stacking(common.NewStackingConfigBuilder().Mode(common.StackingModeNormal)).
		ColorScheme(dashboard.NewFieldColorBuilder().Mode(dashboard.FieldColorModeIdPaletteClassic))
}
