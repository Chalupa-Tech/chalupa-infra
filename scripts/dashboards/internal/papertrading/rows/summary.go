// Package rows holds one file per dashboard row.
//
// Each row func appends its row header and child panels to the
// Dashboard builder and returns the y-units this row consumed (so the
// orchestrator can advance for the next row).
//
// summary.go — at-a-glance KPI strip.
// Phase-7b panel ids: row 100, KPI tiles 101-104.
// Phase-7e slice: ports id=101 ("Equity vs starting cash") only.
package rows

import (
	"github.com/grafana/grafana-foundation-sdk/go/common"
	"github.com/grafana/grafana-foundation-sdk/go/dashboard"
	"github.com/grafana/grafana-foundation-sdk/go/prometheus"
	"github.com/grafana/grafana-foundation-sdk/go/stat"

	"github.com/Chalupa-Tech/chalupa-infra/scripts/dashboards/internal/common/datasources"
	"github.com/Chalupa-Tech/chalupa-infra/scripts/dashboards/internal/common/layout"
	"github.com/Chalupa-Tech/chalupa-infra/scripts/dashboards/internal/common/thresholds"
)

const (
	summaryRowID    = 100
	summaryRowTitle = "Summary (at-a-glance)"
	summaryHeight   = 5 // 1 (row header) + 4 (KPI tile h)
)

// Summary appends the Summary row + its KPI tiles to db.
func Summary(db *dashboard.DashboardBuilder, yBase int) int {
	db.WithRow(layout.Row(summaryRowID, yBase, summaryRowTitle))
	db.WithPanel(equityVsStartingCash(yBase + 1))
	return summaryHeight
}

func equityVsStartingCash(y int) *stat.PanelBuilder {
	return stat.NewPanelBuilder().
		Id(101).
		Title("Equity vs starting cash").
		Description(
			"paper-trading-realism phase-7b: (paper_equity_usd / 10000) − 1. " +
				"Hardcodes $10k starting cash, which matches the per-book " +
				"values.yaml today; if startingCash ever varies per book this " +
				"becomes wrong silently and we'll need a paper_starting_cash_usd " +
				"metric. Green ≥ +1% (book is compounding); neutral ±1% (noise " +
				"floor); amber −1% to −5%; red < −5%.").
		GridPos(layout.Pos(0, y, 6, 4)).
		Datasource(datasources.Victoria()).
		WithTarget(prometheus.NewDataqueryBuilder().
			RefId("A").
			Expr(`paper_equity_usd{book_id=~"$book_id"} / 10000 - 1`).
			LegendFormat("{{book_id}}").
			Instant()).
		Unit("percentunit").
		Thresholds(thresholds.Build(thresholds.EquityReturn)).
		ColorMode(common.BigValueColorModeBackground).
		GraphMode(common.BigValueGraphModeNone).
		ReduceOptions(common.NewReduceDataOptionsBuilder().
			Calcs([]string{"lastNotNull"}).
			Fields("").
			Values(false))
}
