// activity.go — fill counts (total/buys/sells/notional) + by-strategy notional.
// Renumbered: row 800, panels 801-805.
// (Phase-7b ids: row 10, panels 11-15.)
package rows

import (
	"github.com/grafana/grafana-foundation-sdk/go/barchart"
	"github.com/grafana/grafana-foundation-sdk/go/dashboard"
	"github.com/grafana/grafana-foundation-sdk/go/stat"

	"github.com/Chalupa-Tech/chalupa-infra/scripts/dashboards/internal/common/datasources"
	"github.com/Chalupa-Tech/chalupa-infra/scripts/dashboards/internal/common/layout"
	"github.com/Chalupa-Tech/chalupa-infra/scripts/dashboards/internal/common/sql"
)

const (
	activityRowID    = 800
	activityRowTitle = "Activity"
	activityHeight   = 11 // 1 + 4 (KPI tiles) + 6 (barchart h)
)

func Activity(db *dashboard.DashboardBuilder, yBase int) int {
	db.WithRow(layout.Row(activityRowID, yBase, activityRowTitle))
	db.WithPanel(totalFills(yBase + 1))
	db.WithPanel(buys(yBase + 1))
	db.WithPanel(sells(yBase + 1))
	db.WithPanel(notionalTraded(yBase + 1))
	db.WithPanel(byStrategyNotional(yBase + 5))
	return activityHeight
}

func totalFills(y int) *stat.PanelBuilder {
	rawSQL := "SELECT COUNT(*) AS total" +
		" FROM paper_fills" +
		" WHERE $__timeFilter(time) AND strategy IN ($strategy) AND book_id IN ($book_id)"
	return stat.NewPanelBuilder().
		Id(801).
		Title("Total fills").
		Description(
			"Count of paper_fills rows across all selected books / symbols / " +
				"strategies in the current time range. A \"fill\" is one " +
				"executed leg; a single OrderRequest produces one fill in v1 " +
				"(market-only, single-leg). Retries dedup via ClientOrderID so " +
				"a visible growth of this counter represents new work, not " +
				"retried work.").
		GridPos(layout.Pos(0, y, 6, 4)).
		Datasource(datasources.Timescale()).
		WithTarget(sql.Table("A", rawSQL)).
		Unit("short")
}

func buys(y int) *stat.PanelBuilder {
	rawSQL := "SELECT COUNT(*) AS buys" +
		" FROM paper_fills" +
		" WHERE side = 'BUY' AND $__timeFilter(time) AND strategy IN ($strategy) AND book_id IN ($book_id)"
	return stat.NewPanelBuilder().
		Id(802).
		Title("Buys").
		Description(
			"Subset of Total fills where side=BUY. Should roughly equal Sells " +
				"over a mature trading session; a sustained Buys > Sells gap " +
				"means strategies are accumulating inventory (expected for SMA " +
				"during an uptrend, suspicious for alternator).").
		GridPos(layout.Pos(6, y, 6, 4)).
		Datasource(datasources.Timescale()).
		WithTarget(sql.Table("A", rawSQL)).
		Unit("short").
		ColorScheme(dashboard.NewFieldColorBuilder().
			Mode(dashboard.FieldColorModeIdFixed).
			FixedColor("green"))
}

func sells(y int) *stat.PanelBuilder {
	rawSQL := "SELECT COUNT(*) AS sells" +
		" FROM paper_fills" +
		" WHERE side = 'SELL' AND $__timeFilter(time) AND strategy IN ($strategy) AND book_id IN ($book_id)"
	return stat.NewPanelBuilder().
		Id(803).
		Title("Sells").
		Description(
			"Subset of Total fills where side=SELL. SELLs carry non-NULL " +
				"realized_pl (unlike BUYs); the Cumulative realized P&L panel " +
				"aggregates them.").
		GridPos(layout.Pos(12, y, 6, 4)).
		Datasource(datasources.Timescale()).
		WithTarget(sql.Table("A", rawSQL)).
		Unit("short").
		ColorScheme(dashboard.NewFieldColorBuilder().
			Mode(dashboard.FieldColorModeIdFixed).
			FixedColor("orange"))
}

func notionalTraded(y int) *stat.PanelBuilder {
	rawSQL := "SELECT SUM(quantity * price) AS notional" +
		" FROM paper_fills" +
		" WHERE $__timeFilter(time) AND strategy IN ($strategy) AND book_id IN ($book_id)"
	return stat.NewPanelBuilder().
		Id(804).
		Title("Notional traded").
		Description(
			"sum(quantity × price) across all fills in the time range. A " +
				"rough measure of capital turnover — high notional on a low-cash " +
				"book means the book is churning (buying + selling rapidly), " +
				"which usually means slippage tax is the dominant P&L driver.").
		GridPos(layout.Pos(18, y, 6, 4)).
		Datasource(datasources.Timescale()).
		WithTarget(sql.Table("A", rawSQL)).
		Unit("currencyUSD")
}

func byStrategyNotional(y int) *barchart.PanelBuilder {
	rawSQL := "SELECT strategy, SUM(quantity * price) AS notional " +
		"FROM paper_fills " +
		"WHERE $__timeFilter(time) AND book_id IN ($book_id) " +
		"GROUP BY strategy ORDER BY notional DESC"
	return barchart.NewPanelBuilder().
		Id(805).
		Title("By-strategy notional").
		GridPos(layout.Pos(0, y, 24, 6)).
		Datasource(datasources.Timescale()).
		WithTarget(sql.Table("A", rawSQL)).
		Unit("currencyUSD").
		XField("strategy")
}
