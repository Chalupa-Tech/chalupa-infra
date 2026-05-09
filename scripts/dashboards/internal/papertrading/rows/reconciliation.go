// reconciliation.go — sim-only event-vs-projection reconciliation
// signals. Liveness stat (seconds since last sweep), healthy-sweep
// volume stat, and divergence rate timeseries by kind. Sim's steady
// state is zero divergence; any non-zero on the divergence panel
// indicates a write-through ordering bug. The expected drilldown for a
// divergent sweep is the structured log line emitted by the reconciler
// goroutine — operators query VictoriaLogs for `_msg:"reconciliation
// discrepancy"` filtered by book_id (deeplink lives on the Liveness
// panel description). paper-trading-realism phase-11a.
//
// Row 920, panels 921-923.
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
	reconciliationRowID    = 920
	reconciliationRowTitle = "Reconciliation"
	reconciliationHeight   = 9 // 1 + 8 (stat / timeseries h)
)

func Reconciliation(db *dashboard.DashboardBuilder, yBase int) int {
	db.WithRow(layout.Row(reconciliationRowID, yBase, reconciliationRowTitle))
	db.WithPanel(reconcileLiveness(yBase + 1))
	db.WithPanel(reconcileHealthySweeps(yBase + 1))
	db.WithPanel(reconcileDivergenceRate(yBase + 1))
	return reconciliationHeight
}

func reconcileLiveness(y int) *stat.PanelBuilder {
	return stat.NewPanelBuilder().
		Id(921).
		Title("Seconds since last reconcile sweep").
		Description(
			"paper-trading-realism phase-11a: time elapsed since the per-book " +
				"reconciler last completed a sweep. Default cadence is 60s; " +
				"yellow at the next missed tick, red past 2× cadence — a stalled " +
				"reconciler goroutine surfaces here before the divergence " +
				"counter goes silent. " +
				"For divergent sweeps, drill into VictoriaLogs explorer with " +
				"the query `_msg:\"reconciliation discrepancy\" AND " +
				"app:\"go-paper-trader\"`.").
		GridPos(layout.Pos(0, y, 8, 8)).
		Datasource(datasources.Victoria()).
		WithTarget(prometheus.NewDataqueryBuilder().
			RefId("A").
			Expr(`time() - paper_reconcile_last_run_timestamp_seconds{book_id=~"$book_id"}`).
			LegendFormat("{{book_id}}").
			Instant()).
		Unit("s").
		Thresholds(thresholds.Build(thresholds.ReconcileLiveness)).
		ColorMode(common.BigValueColorModeBackground).
		GraphMode(common.BigValueGraphModeNone).
		ReduceOptions(common.NewReduceDataOptionsBuilder().
			Calcs([]string{"lastNotNull"}))
}

func reconcileHealthySweeps(y int) *stat.PanelBuilder {
	return stat.NewPanelBuilder().
		Id(922).
		Title("Healthy sweeps (last 15m)").
		Description(
			"paper-trading-realism phase-11a: count of reconciler sweeps in " +
				"the last 15m that found zero divergence between the adapter " +
				"broker-view and the projection-from-events view. With the " +
				"default 60s cadence and a healthy sim, expect ≈15. Zero is " +
				"red — either the reconciler is not running or every sweep is " +
				"divergent (cross-check the Divergence rate panel and the " +
				"Liveness stat).").
		GridPos(layout.Pos(8, y, 8, 8)).
		Datasource(datasources.Victoria()).
		WithTarget(prometheus.NewDataqueryBuilder().
			RefId("A").
			Expr(`sum by (book_id) (increase(paper_reconcile_healthy_sweeps_total{book_id=~"$book_id"}[15m])) or vector(0)`).
			LegendFormat("{{book_id}}").
			Instant()).
		Unit("short").
		Thresholds(thresholds.Build([]thresholds.Step{
			{Color: "red", Value: nil},
			{Color: "green", Value: p64(1)},
		})).
		ColorMode(common.BigValueColorModeBackground).
		GraphMode(common.BigValueGraphModeNone).
		ReduceOptions(common.NewReduceDataOptionsBuilder().
			Calcs([]string{"lastNotNull"}))
}

func reconcileDivergenceRate(y int) *timeseries.PanelBuilder {
	return timeseries.NewPanelBuilder().
		Id(923).
		Title("Divergence rate by kind").
		Description(
			"paper-trading-realism phase-11a: rate(paper_reconciliation_divergence_total[15m]) " +
				"split by kind ∈ {broker_only_order, local_only_order, " +
				"position_qty_mismatch, cash_mismatch}. Sim's steady state is " +
				"a flat zero line — any non-zero rate is a write-through " +
				"ordering bug. Stack drill: open the matching log line in " +
				"VictoriaLogs with `_msg:\"reconciliation discrepancy\"` and " +
				"the divergent book_id.").
		GridPos(layout.Pos(16, y, 8, 8)).
		Datasource(datasources.Victoria()).
		WithTarget(prometheus.NewDataqueryBuilder().
			RefId("A").
			Expr(`sum by (book_id, kind) (rate(paper_reconciliation_divergence_total{book_id=~"$book_id"}[15m])) or vector(0)`).
			LegendFormat("{{book_id}} / {{kind}}")).
		Unit("short").
		LineWidth(1).
		ColorScheme(dashboard.NewFieldColorBuilder().Mode(dashboard.FieldColorModeIdPaletteClassic)).
		Thresholds(dashboard.NewThresholdsConfigBuilder().
			Mode(dashboard.ThresholdsModeAbsolute).
			Steps([]dashboard.Threshold{
				{Color: "green", Value: nil},
				{Color: "red", Value: p64(0.001)},
			}))
}
