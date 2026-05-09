// panels.go — the six panels of the Schwab Argo Rollouts dashboard.
//
// Pre-port hand-written ids did not exist (panels were anonymous). The
// dashboard has no row separators; panels are top-level. IDs assigned
// here are sequential 1–6 in source-document order.
//
// Phase-4 substantive deltas vs. pre-port (per 7e3 audit):
//   - Panels 3–6 (multi-series timeseries / barchart) gain
//     palette-classic — without it, every per-pod series collides on
//     the same hash color.
//   - Panel 1 (stat) and panel 2 (state-timeline) keep their
//     value-mapped colors; palette-classic does not apply.
package schwabrollouts

import (
	"github.com/grafana/grafana-foundation-sdk/go/barchart"
	"github.com/grafana/grafana-foundation-sdk/go/common"
	"github.com/grafana/grafana-foundation-sdk/go/dashboard"
	"github.com/grafana/grafana-foundation-sdk/go/prometheus"
	"github.com/grafana/grafana-foundation-sdk/go/stat"
	"github.com/grafana/grafana-foundation-sdk/go/statetimeline"
	"github.com/grafana/grafana-foundation-sdk/go/timeseries"

	"github.com/Chalupa-Tech/chalupa-infra/scripts/dashboards/internal/common/datasources"
	"github.com/Chalupa-Tech/chalupa-infra/scripts/dashboards/internal/common/layout"
	"github.com/Chalupa-Tech/chalupa-infra/scripts/dashboards/internal/common/thresholds"
)

// pp returns a pointer to the given float64 — sugar for inline
// threshold steps.
func pp(v float64) *float64 { return &v }

// rolloutPhase — id 1. Stat tile mapping rollout_phase∈{1..6} to a
// labeled color: Completed/Progressing/Paused/Abort/Error/Timeout. The
// PromQL OR-chain encodes the phase enum since the metric exposes it
// as a label, not a numeric.
func rolloutPhase() *stat.PanelBuilder {
	expr := `(rollout_phase{exported_namespace="$namespace", phase="Completed"} == 1) * 1
or
(rollout_phase{exported_namespace="$namespace", phase="Progressing"} == 1) * 2
or
(rollout_phase{exported_namespace="$namespace", phase="Paused"} == 1) * 3
or
(rollout_phase{exported_namespace="$namespace", phase="Abort"} == 1) * 4
or
(rollout_phase{exported_namespace="$namespace", phase="Error"} == 1) * 5
or
(rollout_phase{exported_namespace="$namespace", phase="Timeout"} == 1) * 6`

	mapping := func(value, text, color string) dashboard.ValueMapping {
		vm := dashboard.ValueMap{
			Type: dashboard.MappingTypeValueToText,
			Options: map[string]dashboard.ValueMappingResult{
				value: {Text: stringPtr(text), Color: stringPtr(color)},
			},
		}
		return dashboard.ValueMapping{ValueMap: &vm}
	}

	steps := thresholds.Build([]thresholds.Step{
		{Color: "green", Value: nil},
	})

	return stat.NewPanelBuilder().
		Id(1).
		Title("Rollout Phase").
		GridPos(layout.Pos(0, 0, 24, 4)).
		Datasource(datasources.Victoria()).
		WithTarget(prometheus.NewDataqueryBuilder().
			RefId("A").
			Expr(expr).
			LegendFormat("{{name}}")).
		Mappings([]dashboard.ValueMapping{
			mapping("1", "Completed", "green"),
			mapping("2", "Progressing", "yellow"),
			mapping("3", "Paused", "blue"),
			mapping("4", "Abort", "red"),
			mapping("5", "Error", "dark-red"),
			mapping("6", "Timeout", "orange"),
		}).
		Thresholds(steps).
		ColorMode(common.BigValueColorModeBackground).
		GraphMode(common.BigValueGraphModeNone).
		ReduceOptions(common.NewReduceDataOptionsBuilder().
			Calcs([]string{"lastNotNull"}))
}

// analysisRunResults — id 2. First state-timeline in the SDK ports.
// Maps analysis_run_phase∈{0..4} to {Running, Successful, Failed,
// Inconclusive, Error}; transparent threshold so cells inherit only
// the value-mapped color.
func analysisRunResults() *statetimeline.PanelBuilder {
	expr := `(analysis_run_phase{exported_namespace="$namespace", phase="Running"} == 1) * 0
or
(analysis_run_phase{exported_namespace="$namespace", phase="Successful"} == 1) * 1
or
(analysis_run_phase{exported_namespace="$namespace", phase="Failed"} == 1) * 2
or
(analysis_run_phase{exported_namespace="$namespace", phase="Inconclusive"} == 1) * 3
or
(analysis_run_phase{exported_namespace="$namespace", phase="Error"} == 1) * 4`

	mapping := func(value, text, color string) dashboard.ValueMapping {
		vm := dashboard.ValueMap{
			Type: dashboard.MappingTypeValueToText,
			Options: map[string]dashboard.ValueMappingResult{
				value: {Text: stringPtr(text), Color: stringPtr(color)},
			},
		}
		return dashboard.ValueMapping{ValueMap: &vm}
	}

	steps := thresholds.Build([]thresholds.Step{
		{Color: "transparent", Value: nil},
	})

	return statetimeline.NewPanelBuilder().
		Id(2).
		Title("AnalysisRun Results").
		GridPos(layout.Pos(0, 4, 24, 6)).
		Datasource(datasources.Victoria()).
		WithTarget(prometheus.NewDataqueryBuilder().
			RefId("A").
			Expr(expr).
			LegendFormat("{{name}}")).
		Mappings([]dashboard.ValueMapping{
			mapping("0", "Running", "yellow"),
			mapping("1", "Successful", "green"),
			mapping("2", "Failed", "red"),
			mapping("3", "Inconclusive", "orange"),
			mapping("4", "Error", "dark-red"),
		}).
		Thresholds(steps).
		ShowValue(common.VisibilityModeAuto).
		MergeValues(true).
		AlignValue(common.TimelineValueAlignmentLeft)
}

// podUpStatus — id 3. Per-pod up/down indicator. Multi-series via
// `pod` label → palette-classic per phase-4 substantive fix.
func podUpStatus() *timeseries.PanelBuilder {
	steps := thresholds.Build([]thresholds.Step{
		{Color: "red", Value: nil},
		{Color: "green", Value: pp(1)},
	})
	return timeseries.NewPanelBuilder().
		Id(3).
		Title("Pod Up Status (per pod)").
		GridPos(layout.Pos(0, 10, 12, 8)).
		Datasource(datasources.Victoria()).
		WithTarget(prometheus.NewDataqueryBuilder().
			RefId("A").
			Expr(`up{namespace="$namespace", container=~"$service"}`).
			LegendFormat("{{pod}}")).
		Min(0).
		Max(1).
		Decimals(0).
		Unit("short").
		Thresholds(steps).
		LineWidth(2).
		FillOpacity(10).
		PointSize(5).
		ShowPoints(common.VisibilityModeAuto).
		ColorScheme(dashboard.NewFieldColorBuilder().Mode(dashboard.FieldColorModeIdPaletteClassic)).
		Legend(common.NewVizLegendOptionsBuilder().
			ShowLegend(true).
			DisplayMode(common.LegendDisplayModeTable).
			Placement(common.LegendPlacementBottom).
			Calcs([]string{"lastNotNull"})).
		Tooltip(common.NewVizTooltipOptionsBuilder().
			Mode(common.TooltipDisplayModeMulti))
}

// containerRestarts — id 4. Per-pod / per-container restart counter.
// Multi-series → palette-classic.
func containerRestarts() *timeseries.PanelBuilder {
	steps := thresholds.Build([]thresholds.Step{
		{Color: "green", Value: nil},
		{Color: "red", Value: pp(1)},
	})
	return timeseries.NewPanelBuilder().
		Id(4).
		Title("Container Restarts (per pod)").
		GridPos(layout.Pos(12, 10, 12, 8)).
		Datasource(datasources.Victoria()).
		WithTarget(prometheus.NewDataqueryBuilder().
			RefId("A").
			Expr(`kube_pod_container_status_restarts_total{namespace="$namespace", container=~"$service"}`).
			LegendFormat("{{pod}} / {{container}}")).
		Min(0).
		Decimals(0).
		Unit("short").
		Thresholds(steps).
		LineWidth(2).
		FillOpacity(10).
		PointSize(5).
		ShowPoints(common.VisibilityModeAuto).
		ColorScheme(dashboard.NewFieldColorBuilder().Mode(dashboard.FieldColorModeIdPaletteClassic)).
		Legend(common.NewVizLegendOptionsBuilder().
			ShowLegend(true).
			DisplayMode(common.LegendDisplayModeTable).
			Placement(common.LegendPlacementBottom).
			Calcs([]string{"lastNotNull", "max"})).
		Tooltip(common.NewVizTooltipOptionsBuilder().
			Mode(common.TooltipDisplayModeMulti))
}

// canaryVsStableRestarts — id 5. Horizontal barchart of 5m-rate
// restarts grouped by pod. Multi-series → palette-classic.
func canaryVsStableRestarts() *barchart.PanelBuilder {
	steps := thresholds.Build([]thresholds.Step{
		{Color: "green", Value: nil},
		{Color: "red", Value: pp(1)},
	})
	return barchart.NewPanelBuilder().
		Id(5).
		Title("Canary vs Stable Restarts (rate/5m)").
		GridPos(layout.Pos(0, 18, 12, 8)).
		Datasource(datasources.Victoria()).
		WithTarget(prometheus.NewDataqueryBuilder().
			RefId("A").
			Expr(`sum by (pod) (increase(kube_pod_container_status_restarts_total{namespace="$namespace", container=~"$service"}[5m]))`).
			LegendFormat("{{pod}}")).
		Unit("short").
		Decimals(0).
		Thresholds(steps).
		FillOpacity(80).
		Orientation(common.VizOrientationHorizontal).
		ShowValue(common.VisibilityModeAuto).
		Stacking(common.StackingModeNone).
		ColorScheme(dashboard.NewFieldColorBuilder().Mode(dashboard.FieldColorModeIdPaletteClassic)).
		Legend(common.NewVizLegendOptionsBuilder().
			ShowLegend(true).
			DisplayMode(common.LegendDisplayModeList).
			Placement(common.LegendPlacementBottom))
}

// rolloutReplicas — id 6. Two-query timeseries (ready / running per
// pod). Multi-series → palette-classic.
func rolloutReplicas() *timeseries.PanelBuilder {
	return timeseries.NewPanelBuilder().
		Id(6).
		Title("Rollout Replicas").
		GridPos(layout.Pos(12, 18, 12, 8)).
		Datasource(datasources.Victoria()).
		WithTarget(prometheus.NewDataqueryBuilder().
			RefId("A").
			Expr(`kube_pod_status_ready{namespace="$namespace", pod=~"($service)-.+", condition="true"}`).
			LegendFormat("{{pod}} ready")).
		WithTarget(prometheus.NewDataqueryBuilder().
			RefId("B").
			Expr(`kube_pod_status_phase{namespace="$namespace", pod=~"($service)-.+", phase="Running"} == 1`).
			LegendFormat("{{pod}} running")).
		Min(0).
		Decimals(0).
		Unit("short").
		LineWidth(2).
		FillOpacity(10).
		Stacking(common.NewStackingConfigBuilder().Mode(common.StackingModeNone)).
		ColorScheme(dashboard.NewFieldColorBuilder().Mode(dashboard.FieldColorModeIdPaletteClassic)).
		Legend(common.NewVizLegendOptionsBuilder().
			ShowLegend(true).
			DisplayMode(common.LegendDisplayModeTable).
			Placement(common.LegendPlacementBottom).
			Calcs([]string{"lastNotNull"})).
		Tooltip(common.NewVizTooltipOptionsBuilder().
			Mode(common.TooltipDisplayModeMulti))
}
