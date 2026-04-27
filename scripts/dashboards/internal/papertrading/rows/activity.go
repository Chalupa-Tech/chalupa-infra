// activity.go — order/fill counts, distinct symbols, by-strategy notional.
// Phase-7b panel ids: row 10, panels 11-15.
// Phase-7e slice: ports id=15 ("By-strategy notional") — the
// SQL-backed barchart canary.
package rows

import (
	"github.com/grafana/grafana-foundation-sdk/go/barchart"
	"github.com/grafana/grafana-foundation-sdk/go/dashboard"

	"github.com/Chalupa-Tech/chalupa-infra/scripts/dashboards/internal/common/datasources"
	"github.com/Chalupa-Tech/chalupa-infra/scripts/dashboards/internal/common/layout"
	"github.com/Chalupa-Tech/chalupa-infra/scripts/dashboards/internal/common/sql"
)

const (
	activityRowID    = 10
	activityRowTitle = "Activity"
	activityHeight   = 7 // 1 (row header) + 6 (barchart h)
)

func Activity(db *dashboard.DashboardBuilder, yBase int) int {
	db.WithRow(layout.Row(activityRowID, yBase, activityRowTitle))
	db.WithPanel(byStrategyNotional(yBase + 1))
	return activityHeight
}

func byStrategyNotional(y int) *barchart.PanelBuilder {
	rawSQL := "SELECT strategy, SUM(quantity * price) AS notional " +
		"FROM paper_fills " +
		"WHERE $__timeFilter(time) AND book_id IN ($book_id) " +
		"GROUP BY strategy ORDER BY notional DESC"
	return barchart.NewPanelBuilder().
		Id(15).
		Title("By-strategy notional").
		GridPos(layout.Pos(0, y, 24, 6)).
		Datasource(datasources.Timescale()).
		WithTarget(sql.Table("A", rawSQL)).
		Unit("currencyUSD").
		XField("strategy")
}
