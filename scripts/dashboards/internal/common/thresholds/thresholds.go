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

// EquityReturn — Summary tile id=201 ("Equity vs starting cash").
// Green ≥ +1%, neutral ±1%, amber −1% to −5%, red < −5%.
var EquityReturn = []Step{
	{Color: "red", Value: nil},
	{Color: "orange", Value: p(-0.05)},
	{Color: "transparent", Value: p(-0.01)},
	{Color: "green", Value: p(0.01)},
}

// TodayPnL — Summary tile id=202 ("Today P&L per book").
// Red < $0, neutral $0–$0.01, green ≥ $0.01 (just-positive splits day).
var TodayPnL = []Step{
	{Color: "red", Value: nil},
	{Color: "transparent", Value: p(0)},
	{Color: "green", Value: p(0.01)},
}

// MaxDrawdown — Summary tile id=203 ("Max drawdown (session)").
// Red < −3%, amber −1% to −3%, neutral −0.1% to −1%, green ≥ −0.1%.
var MaxDrawdown = []Step{
	{Color: "red", Value: nil},
	{Color: "orange", Value: p(-0.03)},
	{Color: "transparent", Value: p(-0.01)},
	{Color: "green", Value: p(-0.001)},
}

// WinRate — Summary tile id=204 ("Win rate per book").
// Red < 45%, amber 45–50%, neutral 50–55%, green ≥ 55%.
var WinRate = []Step{
	{Color: "red", Value: nil},
	{Color: "orange", Value: p(0.45)},
	{Color: "transparent", Value: p(0.50)},
	{Color: "green", Value: p(0.55)},
}

// ProfitFactor — Strategy-quality tile id=403 ("Profit factor").
// Red < 1.0, amber 1.0–1.2, neutral 1.2–2.0, green ≥ 2.0.
var ProfitFactor = []Step{
	{Color: "red", Value: nil},
	{Color: "orange", Value: p(1.0)},
	{Color: "transparent", Value: p(1.2)},
	{Color: "green", Value: p(2.0)},
}

// EquityByBook — Account tile id=602 ("Current equity by book").
// Red < $9000, amber $9000–$9500, neutral $9500–$10500, green ≥ $10500.
var EquityByBook = []Step{
	{Color: "red", Value: nil},
	{Color: "orange", Value: p(9000)},
	{Color: "transparent", Value: p(9500)},
	{Color: "green", Value: p(10500)},
}

// OrphanedPosition — Hygiene tile id=1201. Green when zero, red when any.
var OrphanedPosition = []Step{
	{Color: "green", Value: nil},
	{Color: "red", Value: p(0.01)},
}

// BooksHalted — Risk tile id=302. Green when zero, red when any halt.
var BooksHalted = []Step{
	{Color: "green", Value: nil},
	{Color: "red", Value: p(1)},
}

// ReconcileLiveness — Reconciliation tile id=921 ("Seconds since last
// reconcile sweep"). 60s default cadence ⇒ green < 60s, yellow at the
// next missed tick (60–120s), red beyond 2× cadence. Phase-11a.
var ReconcileLiveness = []Step{
	{Color: "green", Value: nil},
	{Color: "yellow", Value: p(60)},
	{Color: "red", Value: p(120)},
}

// ReconcileDivergence — Reconciliation tile id=922 ("Divergences in
// last 15m"). Green when zero, red when any. Sim's steady state is
// zero divergence; any non-zero needs investigation. Phase-11a.
var ReconcileDivergence = []Step{
	{Color: "green", Value: nil},
	{Color: "red", Value: p(1)},
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
