// Package layout provides grid-layout helpers for dashboards.
//
// Each row module is told its absolute y-base by the build orchestrator
// and positions its child panels with absolute-y gridPos coordinates.
// Grafana's panel grid is 24 columns wide. Common cell widths:
//
//	6  : KPI tile (4-wide row of KPIs)
//	8  : 1/3 width
//	12 : 1/2 width
//	24 : full width
package layout

import (
	"github.com/grafana/grafana-foundation-sdk/go/dashboard"
)

const GridWidth = 24

// Row returns a collapsible row header at absolute-y `y`. Always
// full-width, height 1 (Grafana's row-panel convention).
func Row(rowID, y int, title string) *dashboard.RowBuilder {
	return dashboard.NewRowBuilder(title).
		Id(uint32(rowID)).
		GridPos(dashboard.GridPos{X: 0, Y: uint32(y), W: GridWidth, H: 1})
}

// Pos is sugar for the SDK's GridPos struct with int args.
func Pos(x, y, w, h int) dashboard.GridPos {
	return dashboard.GridPos{X: uint32(x), Y: uint32(y), W: uint32(w), H: uint32(h)}
}
