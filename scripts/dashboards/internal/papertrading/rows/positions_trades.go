// positions_trades.go — open positions table, recent fills table.
// Renumbered: row 700, panels 701-702.
// (Phase-7b ids: row 7, panels 8-9.)
package rows

import (
	"github.com/grafana/grafana-foundation-sdk/go/common"
	"github.com/grafana/grafana-foundation-sdk/go/dashboard"
	"github.com/grafana/grafana-foundation-sdk/go/table"

	"github.com/Chalupa-Tech/chalupa-infra/scripts/dashboards/internal/common/datasources"
	"github.com/Chalupa-Tech/chalupa-infra/scripts/dashboards/internal/common/layout"
	"github.com/Chalupa-Tech/chalupa-infra/scripts/dashboards/internal/common/sql"
)

const (
	positionsRowID    = 700
	positionsRowTitle = "Positions & trades"
	positionsHeight   = 8 // 1 (row header) + 7 (table h)
)

func PositionsTrades(db *dashboard.DashboardBuilder, yBase int) int {
	db.WithRow(layout.Row(positionsRowID, yBase, positionsRowTitle))
	db.WithPanel(livePositions(yBase + 1))
	db.WithPanel(tradeLog(yBase + 1))
	return positionsHeight
}

func livePositions(y int) *table.PanelBuilder {
	rawSQL := "SELECT pp.book_id, pp.symbol, pp.quantity, pp.avg_price, pp.mark_price, pp.unrealized_pl " +
		"FROM paper_positions pp " +
		"JOIN (" +
		"  SELECT DISTINCT ON (book_id) book_id, time FROM paper_cash " +
		"  WHERE book_id IN ($book_id) AND time > NOW() - INTERVAL '7 days' " +
		"  ORDER BY book_id, time DESC" +
		") latest ON pp.book_id = latest.book_id AND pp.time = latest.time " +
		"WHERE pp.symbol IN ($symbol) AND pp.quantity != 0 " +
		"ORDER BY pp.book_id, pp.symbol"
	usd := []dashboard.DynamicConfigValue{{Id: "unit", Value: "currencyUSD"}}
	return table.NewPanelBuilder().
		Id(701).
		Title("Live positions").
		Description(
			"Current open positions for the selected books and symbols, "+
				"with avg cost, mark, and unrealized P&L. Reads each book's "+
				"latest snapshot (last 7 days) from paper_positions where "+
				"quantity != 0 — closed positions disappear deterministically "+
				"once SimAdapter prunes the gauge series.").
		GridPos(layout.Pos(0, y, 12, 7)).
		Datasource(datasources.Timescale()).
		WithTarget(sql.Table("A", rawSQL)).
		Align(common.FieldTextAlignmentRight).
		OverrideByName("unrealized_pl", []dashboard.DynamicConfigValue{
			{Id: "unit", Value: "currencyUSD"},
			{Id: "custom.cellOptions", Value: map[string]any{"type": "color-text"}},
			{Id: "thresholds", Value: map[string]any{
				"mode": "absolute",
				"steps": []map[string]any{
					{"color": "red", "value": nil},
					{"color": "green", "value": 0},
				},
			}},
		}).
		OverrideByName("avg_price", usd).
		OverrideByName("mark_price", usd)
}

func tradeLog(y int) *table.PanelBuilder {
	rawSQL := "SELECT time, book_id, symbol, side, quantity, price, strategy" +
		" FROM paper_fills" +
		" WHERE symbol IN ($symbol) AND strategy IN ($strategy) AND book_id IN ($book_id)" +
		" ORDER BY time DESC LIMIT 50"
	return table.NewPanelBuilder().
		Id(702).
		Title("Trade log (last 50 fills)").
		Description(
			"Most recent 50 fills across the selected books / symbols / "+
				"strategies, oldest at bottom. realized_pl is non-NULL on "+
				"SELLs only — BUYs render blank in that column; that's the "+
				"phase-4b invariant, not missing data.").
		GridPos(layout.Pos(12, y, 12, 7)).
		Datasource(datasources.Timescale()).
		WithTarget(sql.Table("A", rawSQL)).
		Align(common.FieldTextAlignmentRight).
		OverrideByName("price", []dashboard.DynamicConfigValue{
			{Id: "unit", Value: "currencyUSD"},
		})
}
