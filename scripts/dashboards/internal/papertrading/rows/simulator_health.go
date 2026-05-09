// simulator_health.go — quote freshness, NATS dropped messages, fill persist latency.
// Renumbered: row 900, panels 901-903.
// (Phase-7b ids: row 16, panels 17-19.)
package rows

import (
	"github.com/grafana/grafana-foundation-sdk/go/cog"
	"github.com/grafana/grafana-foundation-sdk/go/dashboard"
	"github.com/grafana/grafana-foundation-sdk/go/prometheus"
	"github.com/grafana/grafana-foundation-sdk/go/timeseries"

	"github.com/Chalupa-Tech/chalupa-infra/scripts/dashboards/internal/common/datasources"
	"github.com/Chalupa-Tech/chalupa-infra/scripts/dashboards/internal/common/layout"
	"github.com/Chalupa-Tech/chalupa-infra/scripts/dashboards/internal/common/links"
)

const (
	simulatorHealthRowID    = 900
	simulatorHealthRowTitle = "Simulator health"
	simulatorHealthHeight   = 9 // 1 + 8 (timeseries h)
)

func SimulatorHealth(db *dashboard.DashboardBuilder, yBase int) int {
	db.WithRow(layout.Row(simulatorHealthRowID, yBase, simulatorHealthRowTitle))
	db.WithPanel(quoteAgePerSymbol(yBase + 1))
	db.WithPanel(natsDroppedMessages(yBase + 1))
	db.WithPanel(fillPersistP99(yBase + 1))
	return simulatorHealthHeight
}

func p64(v float64) *float64 { return &v }

func quoteAgePerSymbol(y int) *timeseries.PanelBuilder {
	return timeseries.NewPanelBuilder().
		Id(901).
		Title("Quote age per symbol").
		GridPos(layout.Pos(0, y, 8, 8)).
		Datasource(datasources.Victoria()).
		WithTarget(prometheus.NewDataqueryBuilder().
			RefId("A").
			Expr(`time() - paper_quote_last_update_timestamp_seconds`).
			LegendFormat("{{symbol}}")).
		Unit("s").
		LineWidth(1).
		ColorScheme(dashboard.NewFieldColorBuilder().Mode(dashboard.FieldColorModeIdPaletteClassic)).
		Thresholds(dashboard.NewThresholdsConfigBuilder().
			Mode(dashboard.ThresholdsModeAbsolute).
			Steps([]dashboard.Threshold{
				{Color: "green", Value: nil},
				{Color: "yellow", Value: p64(5)},
				{Color: "red", Value: p64(30)},
			})).
		// Drilldown #2: quote staleness is a feed problem — hop to the
		// schwab-feed dashboard with the affected symbol pre-selected.
		DataLinks([]cog.Builder[dashboard.DashboardLink]{
			links.To(
				"schwab-feed Poll Duration",
				"/d/schwab-feed?from=${__from}&to=${__to}&var-symbol=${__series.name}",
			),
		})
}

func natsDroppedMessages(y int) *timeseries.PanelBuilder {
	return timeseries.NewPanelBuilder().
		Id(902).
		Title("NATS dropped messages (rate/5m)").
		GridPos(layout.Pos(8, y, 8, 8)).
		Datasource(datasources.Victoria()).
		WithTarget(prometheus.NewDataqueryBuilder().
			RefId("A").
			// `or vector(0)` ensures the panel renders a flat green line at
			// 0 when the counter has never incremented (steady state).
			// Without this, an empty timeseries is ambiguous to operators —
			// "panel broken" vs "no drops". Aligns with panels 302/1201.
			Expr(`rate(paper_nats_dropped_messages_total[5m]) or vector(0)`).
			LegendFormat("drops/s")).
		Unit("short").
		Thresholds(dashboard.NewThresholdsConfigBuilder().
			Mode(dashboard.ThresholdsModeAbsolute).
			Steps([]dashboard.Threshold{
				{Color: "green", Value: nil},
				{Color: "red", Value: p64(0.001)},
			})).
		// Drilldown #3: cross-check this dropped-counter against the
		// publisher-side rate. Delta = subscriber-side issue.
		DataLinks([]cog.Builder[dashboard.DashboardLink]{
			links.To(
				"schwab-feed NATS Publish Rate",
				"/d/schwab-feed?from=${__from}&to=${__to}",
			),
		})
}

func fillPersistP99(y int) *timeseries.PanelBuilder {
	return timeseries.NewPanelBuilder().
		Id(903).
		Title("Fill persist p99 latency").
		GridPos(layout.Pos(16, y, 8, 8)).
		Datasource(datasources.Victoria()).
		WithTarget(prometheus.NewDataqueryBuilder().
			RefId("A").
			Expr(`histogram_quantile(0.99, sum(rate(paper_fill_persist_duration_seconds_bucket[5m])) by (le))`).
			LegendFormat("p99")).
		Unit("s").
		Thresholds(dashboard.NewThresholdsConfigBuilder().
			Mode(dashboard.ThresholdsModeAbsolute).
			Steps([]dashboard.Threshold{
				{Color: "green", Value: nil},
				{Color: "yellow", Value: p64(0.1)},
				{Color: "red", Value: p64(1)},
			}))
}
