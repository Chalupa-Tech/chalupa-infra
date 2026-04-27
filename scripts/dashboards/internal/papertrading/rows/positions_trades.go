// positions_trades.go — open positions table, recent fills table.
// Phase-7b panel ids: row 7, panels 8-9.
// Phase-7e slice: ports id=8 ("Live positions") — the table-panel
// canary, exercising byName overrides for unit + threshold + cell color.
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
	positionsRowID    = 7
	positionsRowTitle = "Positions & trades"
	positionsHeight   = 8 // 1 (row header) + 7 (table h)
)

func PositionsTrades(db *dashboard.DashboardBuilder, yBase int) int {
	db.WithRow(layout.Row(positionsRowID, yBase, positionsRowTitle))
	db.WithPanel(livePositions(yBase + 1))
	return positionsHeight
}

func livePositions(y int) *table.PanelBuilder {
	rawSQL := "SELECT DISTINCT ON (book_id, symbol) " +
		"book_id, symbol, quantity, avg_price, mark_price, unrealized_pl " +
		"FROM paper_positions " +
		"WHERE symbol IN ($symbol) AND book_id IN ($book_id) AND quantity != 0 " +
		"ORDER BY book_id, symbol, time DESC"
	usd := []dashboard.DynamicConfigValue{{Id: "unit", Value: "currencyUSD"}}
	return table.NewPanelBuilder().
		Id(8).
		Title("Live positions").
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
