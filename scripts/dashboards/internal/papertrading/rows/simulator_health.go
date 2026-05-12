// simulator_health.go — quote freshness, NATS dropped messages, fill persist latency,
// adapter event drop rate.
// Renumbered: row 900, panels 901-904.
// (Phase-7b ids: row 16, panels 17-19; phase-11f added panel 904.)
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
	simulatorHealthHeight   = 17 // 1 + 8 + 8 (row header + two visual rows of timeseries)
)

// SimulatorHealth wires its three operator-debug panels under a
// default-collapsed row (paper-trading-realism phase-11c). The panels
// remain accessible (one click to expand) but don't dominate the
// landing view during a 30-day evaluation window — they belong on
// screen during build, not measurement. To preserve the collapsed
// behavior in committed JSON, panels are nested into the row builder
// rather than at the dashboard top level: when collapsed, only the
// nested panels hide; sibling rows remain at top-level gridPos.
func SimulatorHealth(db *dashboard.DashboardBuilder, yBase int) int {
	row := layout.Row(simulatorHealthRowID, yBase, simulatorHealthRowTitle).
		Collapsed(true).
		WithPanel(quoteAgePerSymbol(yBase + 1)).
		WithPanel(natsDroppedMessages(yBase + 1)).
		WithPanel(fillPersistP99(yBase + 1)).
		WithPanel(adapterEventsDropped(yBase + 9))
	db.WithRow(row)
	return simulatorHealthHeight
}

func p64(v float64) *float64 { return &v }

func quoteAgePerSymbol(y int) *timeseries.PanelBuilder {
	return timeseries.NewPanelBuilder().
		Id(901).
		Title("Quote age per symbol").
		Description(
			"Seconds since the last cached quote per symbol — computed as " +
				"`time() - paper_quote_last_update_timestamp_seconds`. >30s " +
				"during market hours means the SimAdapter's stale-quote " +
				"refusal is firing (orders for that symbol bounce with " +
				"ErrStaleQuote); check NATS health and feed-side metrics " +
				"first, then the symbol watchlist.").
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
		Description(
			"5-minute rate of NATS slow-consumer drops per book. Healthy " +
				"steady state = 0 (the `or vector(0)` keeps the line green " +
				"when the counter has never incremented). Sustained drops " +
				"mean the trader is falling behind quote ingest — typical " +
				"causes are TimescaleDB write-stalls on AppendFill or the " +
				"strategy goroutine blocking on a slow PlaceOrder.").
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

// adapterEventsDropped — phase-11f panel 904. Rate of AdapterEvent
// drops on the per-book SimAdapter.Events() channel. Drop policy is
// "drop-with-measurement" per
// workstreams/paper-trading-realism/briefs/2026-05-12-adapter-channel-backpressure-policy.md
// — events the strategy never saw. Amber 0.1/s (current steady state
// at typical quote cadence; non-zero but harmless), red 1.0/s
// (catastrophic; alert fires at this threshold sustained 10m).
// Durable lifecycle history is in paper_order_events; this panel
// surfaces an in-process visibility gap, not data loss.
func adapterEventsDropped(y int) *timeseries.PanelBuilder {
	return timeseries.NewPanelBuilder().
		Id(904).
		Title("Adapter events dropped (rate/5m)").
		Description(
			"5-minute rate of AdapterEvent drops on the SimAdapter.Events() " +
				"channel per book. The channel is bounded (cap 256) and the " +
				"producer does a non-blocking send: on overflow it increments " +
				"`paper_adapter_events_dropped_total` rather than blocking " +
				"PlaceOrder. Events here are order-lifecycle transitions " +
				"(Submitted/Acknowledged/Filled/Rejected/Canceled/Replaced) " +
				"— the strategy or dashboard subscriber never saw them. The " +
				"durable record is in `paper_order_events`; this panel " +
				"measures in-process subscriber drift, not data loss. " +
				"Policy rationale: see " +
				"workstreams/paper-trading-realism/briefs/2026-05-12-adapter-channel-backpressure-policy.md").
		GridPos(layout.Pos(0, y, 8, 8)).
		Datasource(datasources.Victoria()).
		WithTarget(prometheus.NewDataqueryBuilder().
			RefId("A").
			// `or vector(0)` keeps the panel rendering a flat-green line at 0
			// when no book has dropped anything (steady state for a deployed
			// trader; the counter is created lazily per book on first drop).
			// Aligns with the convention in panels 902 / 1201.
			Expr(`sum by (book_id) (rate(paper_adapter_events_dropped_total[5m])) or vector(0)`).
			LegendFormat("{{book_id}}")).
		Unit("short").
		LineWidth(1).
		ColorScheme(dashboard.NewFieldColorBuilder().Mode(dashboard.FieldColorModeIdPaletteClassic)).
		Thresholds(dashboard.NewThresholdsConfigBuilder().
			Mode(dashboard.ThresholdsModeAbsolute).
			Steps([]dashboard.Threshold{
				{Color: "green", Value: nil},
				{Color: "orange", Value: p64(0.1)},
				{Color: "red", Value: p64(1.0)},
			}))
}

func fillPersistP99(y int) *timeseries.PanelBuilder {
	return timeseries.NewPanelBuilder().
		Id(903).
		Title("Fill persist p99 latency").
		Description(
			"P99 of the AppendFill write-through to TimescaleDB " +
				"(paper_fill_persist_duration_seconds). Internal SLO: <50ms " +
				"steady state, <100ms tolerable. Sustained spikes correlate " +
				"with TSDB pressure (chunk pruning, vacuum, or a misbehaving " +
				"sibling tenant on the cluster) and will eventually surface " +
				"as NATS drops as the trader falls behind.").
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
