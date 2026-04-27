// Package thresholds holds named threshold tables shared across stat
// panels.
//
// Each table is a slice of (color, threshold-value) pairs in absolute
// mode. Convert to an SDK ThresholdsConfigBuilder via Build().
//
// Color names match Grafana's palette: red, orange, transparent, green,
// super-light-green, super-light-orange, etc. (see
// https://grafana.com/docs/grafana/latest/visualizations/panels-visualizations/configure-thresholds/).
package thresholds

import (
	"github.com/grafana/grafana-foundation-sdk/go/dashboard"
)

// Step is a single (color, value) threshold step. Value=nil means
// "from -infinity".
type Step struct {
	Color string
	Value *float64
}

// p is a small helper for building step-value pointers inline.
func p(v float64) *float64 { return &v }

// EquityReturn — "equity vs starting cash" KPI tile (id=101 in
// phase-7b). Green ≥ +1%, neutral ±1%, amber −1% to −5%, red < −5%.
var EquityReturn = []Step{
	{Color: "red", Value: nil},
	{Color: "orange", Value: p(-0.05)},
	{Color: "transparent", Value: p(-0.01)},
	{Color: "green", Value: p(0.01)},
}

// WinRate — promotion threshold ≥ 0.55 per sandbox-live-separation brief.
var WinRate = []Step{
	{Color: "red", Value: nil},
	{Color: "orange", Value: p(0.40)},
	{Color: "transparent", Value: p(0.50)},
	{Color: "green", Value: p(0.55)},
}

// ProfitFactor — ≥ 1.5 = healthy.
var ProfitFactor = []Step{
	{Color: "red", Value: nil},
	{Color: "orange", Value: p(1.0)},
	{Color: "transparent", Value: p(1.2)},
	{Color: "green", Value: p(1.5)},
}

// SharpeSortino — ≥ 1.0 = promotion candidate per the
// sandbox-live-separation brief.
var SharpeSortino = []Step{
	{Color: "red", Value: nil},
	{Color: "orange", Value: p(0.0)},
	{Color: "transparent", Value: p(0.5)},
	{Color: "green", Value: p(1.0)},
}

// DailyMaxLoss — −2% = soft warn, −5% = hard stop per phase-6 risk module.
var DailyMaxLoss = []Step{
	{Color: "red", Value: nil},
	{Color: "orange", Value: p(-0.05)},
	{Color: "transparent", Value: p(-0.02)},
	{Color: "green", Value: p(0.0)},
}

// Build returns an SDK ThresholdsConfigBuilder in absolute mode for
// the given step list. The returned *ThresholdsConfigBuilder satisfies
// cog.Builder[ThresholdsConfig] structurally, so it can be passed
// directly to panel builders' Thresholds() method.
func Build(steps []Step) *dashboard.ThresholdsConfigBuilder {
	out := make([]dashboard.Threshold, 0, len(steps))
	for _, s := range steps {
		out = append(out, dashboard.Threshold{Color: s.Color, Value: s.Value})
	}
	return dashboard.NewThresholdsConfigBuilder().
		Mode(dashboard.ThresholdsModeAbsolute).
		Steps(out)
}
