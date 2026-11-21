// panels.go — the eight panels of the Storage & Log Pipeline dashboard.
// Row 100 (Longhorn): panels 101-104. Row 200 (Log pipeline): 201-204.
package storagelogs

import (
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

func pp(v float64) *float64 { return &v }

func stringPtr(s string) *string { return &s }

func query(refID, expr, legend string) *prometheus.DataqueryBuilder {
	return prometheus.NewDataqueryBuilder().RefId(refID).Expr(expr).LegendFormat(legend)
}

// series is the common timeseries shape: palette-classic (multi-series),
// table legend with last and max, shared tooltip. alertAt > 0 draws the
// matching alert's threshold as a dashed line; 0 draws none.
func series(id uint32, title, description string, pos dashboard.GridPos, unit string, alertAt float64) *timeseries.PanelBuilder {
	steps := []thresholds.Step{{Color: "transparent", Value: nil}}
	style := common.GraphThresholdsStyleModeOff
	if alertAt > 0 {
		steps = append(steps, thresholds.Step{Color: "red", Value: pp(alertAt)})
		style = common.GraphThresholdsStyleModeDashed
	}
	return timeseries.NewPanelBuilder().
		Id(id).
		Title(title).
		Description(description).
		GridPos(pos).
		Datasource(datasources.Victoria()).
		Min(0).
		Unit(unit).
		LineWidth(2).
		FillOpacity(10).
		ShowPoints(common.VisibilityModeNever).
		Thresholds(thresholds.Build(steps)).
		ThresholdsStyle(common.NewGraphThresholdsStyleConfigBuilder().Mode(style)).
		ColorScheme(dashboard.NewFieldColorBuilder().Mode(dashboard.FieldColorModeIdPaletteClassic)).
		Legend(common.NewVizLegendOptionsBuilder().
			ShowLegend(true).
			DisplayMode(common.LegendDisplayModeTable).
			Placement(common.LegendPlacementBottom).
			Calcs([]string{"lastNotNull", "max"})).
		Tooltip(common.NewVizTooltipOptionsBuilder().
			Mode(common.TooltipDisplayModeMulti))
}

// volumeRobustness — id 101. One lane per volume. Each longhorn-manager
// reports the volumes attached to its node; `max by` keeps one series
// when a volume moves between nodes.
func volumeRobustness(y int) *statetimeline.PanelBuilder {
	mapping := func(value, text, color string) dashboard.ValueMapping {
		vm := dashboard.ValueMap{
			Type: dashboard.MappingTypeValueToText,
			Options: map[string]dashboard.ValueMappingResult{
				value: {Text: stringPtr(text), Color: stringPtr(color)},
			},
		}
		return dashboard.ValueMapping{ValueMap: &vm}
	}
	return statetimeline.NewPanelBuilder().
		Id(101).
		Title("Volume robustness").
		Description("Per Longhorn volume: healthy, degraded (a replica is not healthy; LonghornVolumeDegraded after 30m) or faulted (no healthy replica; LonghornVolumeFaulted pages after 2m). Unknown is a detached volume.").
		GridPos(layout.Pos(0, y, 24, 6)).
		Datasource(datasources.Victoria()).
		WithTarget(query("A",
			`max by (pvc_namespace, pvc) (longhorn_volume_robustness)`,
			"{{pvc_namespace}}/{{pvc}}")).
		Mappings([]dashboard.ValueMapping{
			mapping("0", "Unknown", "text"),
			mapping("1", "Healthy", "green"),
			mapping("2", "Degraded", "orange"),
			mapping("3", "Faulted", "red"),
		}).
		Thresholds(thresholds.Build([]thresholds.Step{
			{Color: "transparent", Value: nil},
		})).
		ShowValue(common.VisibilityModeNever).
		MergeValues(true).
		RowHeight(0.8)
}

// managerLogRate — id 102. The signal that would have caught 2026-11-17.
func managerLogRate(y int) *timeseries.PanelBuilder {
	return series(102, "longhorn-manager log lines/s",
		"Lines per second from longhorn-manager, per node, counted by Vector. LonghornManagerLogFlood fires above 100/s for 10m; on 2026-11-17 the replay loop reached about 2,200/s.",
		layout.Pos(0, y, 12, 7), "short", 100).
		WithTarget(query("A",
			`sum by (node) (rate(vector_pod_log_lines_total{log_namespace="longhorn-system", log_container="longhorn-manager"}[5m]))`,
			"{{node}}"))
}

// csiFormatFailures — id 103. No series exists until the first failure,
// so "none seen" is the normal state, not missing data.
func csiFormatFailures(y int) *stat.PanelBuilder {
	return stat.NewPanelBuilder().
		Id(103).
		Title("CSI format failures").
		Description("\"format of disk\" lines from longhorn-csi-plugin in the selected range: mkfs failed, or the phase-97 guard refused a volume that holds data. LonghornFormatRefusedOrFailed pages on any.").
		GridPos(layout.Pos(12, y, 6, 7)).
		Datasource(datasources.Victoria()).
		WithTarget(query("A",
			`sum by (volume) (increase(vector_longhorn_csi_format_failures_total[$__range]))`,
			"{{volume}}")).
		Unit("short").
		Decimals(0).
		NoValue("none seen").
		Thresholds(thresholds.Build([]thresholds.Step{
			{Color: "green", Value: nil},
			{Color: "red", Value: pp(1)},
		})).
		ColorMode(common.BigValueColorModeBackground).
		GraphMode(common.BigValueGraphModeNone).
		ReduceOptions(common.NewReduceDataOptionsBuilder().
			Calcs([]string{"lastNotNull"}))
}

// mkfsGuardAge — id 104. LonghornMkfsGuardNotApplied fires past 30m.
func mkfsGuardAge(y int) *stat.PanelBuilder {
	return stat.NewPanelBuilder().
		Id(104).
		Title("mkfs guard: since last success").
		Description("Time since the longhorn-mkfs-guard CronJob (every 5m) last succeeded. It re-applies the guard after a Longhorn upgrade; LonghornMkfsGuardNotApplied fires past 30m.").
		GridPos(layout.Pos(18, y, 6, 7)).
		Datasource(datasources.Victoria()).
		WithTarget(query("A",
			`time() - max(kube_cronjob_status_last_successful_time{namespace="longhorn-system", cronjob="longhorn-mkfs-guard"})`,
			"")).
		Unit("s").
		Thresholds(thresholds.Build([]thresholds.Step{
			{Color: "green", Value: nil},
			{Color: "orange", Value: pp(600)},
			{Color: "red", Value: pp(1800)},
		})).
		ColorMode(common.BigValueColorModeBackground).
		GraphMode(common.BigValueGraphModeNone).
		ReduceOptions(common.NewReduceDataOptionsBuilder().
			Calcs([]string{"lastNotNull"}))
}

// logIngestRate — id 201. Ingest against LogIngestSpike's moving
// threshold (10x the 1-day median, floor 100 rows/s).
func logIngestRate(y int) *timeseries.PanelBuilder {
	return series(201, "VictoriaLogs ingest (rows/s)",
		"Rows per second VictoriaLogs ingested, and LogIngestSpike's threshold: 10x the 1-day median, never below 100. The alert needs 15m above it.",
		layout.Pos(0, y, 12, 8), "short", 100).
		WithTarget(query("A", `sum(rate(vl_rows_ingested_total[5m]))`, "ingested")).
		WithTarget(query("B",
			`clamp_min(10 * quantile_over_time(0.5, sum(rate(vl_rows_ingested_total[5m]))[1d:5m]), 100)`,
			"alert threshold"))
}

// topLogStreams — id 202. Names the stream behind a spike.
func topLogStreams(y int) *timeseries.PanelBuilder {
	return series(202, "Top log streams (lines/s)",
		"The ten busiest namespace/container pairs by lines per second, as counted by Vector on each node.",
		layout.Pos(12, y, 12, 8), "short", 0).
		WithTarget(query("A",
			`topk(10, sum by (node, log_namespace, log_container) (rate(vector_pod_log_lines_total[5m])))`,
			"{{log_namespace}}/{{log_container}} ({{node}})"))
}

// pipelineCPU — id 203. What a log flood costs. `max by` removes the
// duplicate cadvisor series kubelet exposes on two endpoints.
func pipelineCPU(y int) *timeseries.PanelBuilder {
	return series(203, "Log pipeline CPU (cores)",
		"CPU of Vector, VictoriaLogs and longhorn-manager per pod. On 2026-11-17 these went from about 0.05 to 2.9 cores together on a 4-core node.",
		layout.Pos(0, y, 12, 8), "short", 0).
		WithTarget(query("A",
			`max by (pod, container) (rate(container_cpu_usage_seconds_total{namespace=~"observability|longhorn-system", container=~"vector|vlogs|longhorn-manager"}[5m]))`,
			"{{pod}}"))
}

// vmagentQueue — id 204. MetricsPipelineStale fires when this stays
// above 64 MiB for 10m.
func vmagentQueue(y int) *timeseries.PanelBuilder {
	return series(204, "vmagent queue pending",
		"Bytes vmagent holds for vmsingle. After a vmsingle outage it replays oldest-first, so recent data is missing until this drains; MetricsPipelineStale fires above 64 MiB for 10m.",
		layout.Pos(12, y, 12, 8), "bytes", 64*1024*1024).
		WithTarget(query("A", `max(vm_persistentqueue_bytes_pending)`, "pending"))
}
