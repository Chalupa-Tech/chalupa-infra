// hygiene.go — orphan-position summary stat + per-(book,symbol) table.
// Renumbered: row 1200, panels 1201-1202.
// (Phase-7b ids: row 24, panels 25-26.)
package rows

import (
	"github.com/grafana/grafana-foundation-sdk/go/common"
	"github.com/grafana/grafana-foundation-sdk/go/dashboard"
	"github.com/grafana/grafana-foundation-sdk/go/prometheus"
	"github.com/grafana/grafana-foundation-sdk/go/stat"
	"github.com/grafana/grafana-foundation-sdk/go/table"

	"github.com/Chalupa-Tech/chalupa-infra/scripts/dashboards/internal/common/datasources"
	"github.com/Chalupa-Tech/chalupa-infra/scripts/dashboards/internal/common/layout"
	"github.com/Chalupa-Tech/chalupa-infra/scripts/dashboards/internal/common/thresholds"
)

const (
	hygieneRowID    = 1200
	hygieneRowTitle = "Hygiene"
	hygieneHeight   = 8 // 1 + 7 (stat / table h)
)

func Hygiene(db *dashboard.DashboardBuilder, yBase int) int {
	db.WithRow(layout.Row(hygieneRowID, yBase, hygieneRowTitle))
	db.WithPanel(orphanedPositionTotal(yBase + 1))
	db.WithPanel(orphanedPositionsTable(yBase + 1))
	return hygieneHeight
}

func orphanedPositionTotal(y int) *stat.PanelBuilder {
	return stat.NewPanelBuilder().
		Id(1201).
		Title("Orphaned position $ (all books)").
		Description(
			"paper-trading-realism phase-3: sum of paper_orphaned_position_usd " +
				"across every (book, symbol). Non-zero = a book holds inventory " +
				"the strategy will never close; operator must reconcile manually. " +
				"Green = healthy, red = intervention required.").
		GridPos(layout.Pos(0, y, 8, 7)).
		Datasource(datasources.Victoria()).
		WithTarget(prometheus.NewDataqueryBuilder().
			RefId("A").
			Expr(`sum(paper_orphaned_position_usd{book_id=~"$book_id"}) or vector(0)`).
			Instant()).
		Unit("currencyUSD").
		Thresholds(thresholds.Build(thresholds.OrphanedPosition)).
		ColorMode(common.BigValueColorModeBackground).
		GraphMode(common.BigValueGraphModeNone).
		ReduceOptions(common.NewReduceDataOptionsBuilder().
			Calcs([]string{"lastNotNull"}))
}

func orphanedPositionsTable(y int) *table.PanelBuilder {
	return table.NewPanelBuilder().
		Id(1202).
		Title("Orphaned positions by (book, symbol)").
		Description(
			"paper-trading-realism phase-3: one row per (book, symbol) where " +
				"the strategy no longer trades the symbol but the position map " +
				"still holds inventory. MarkValue = qty × current_mid from the " +
				"adapter's cached quote. Empty table = healthy.").
		GridPos(layout.Pos(8, y, 16, 7)).
		Datasource(datasources.Victoria()).
		WithTarget(prometheus.NewDataqueryBuilder().
			RefId("A").
			Expr(`paper_orphaned_position_usd{book_id=~"$book_id"}`).
			Instant().
			Format(prometheus.PromQueryFormatTable)).
		WithTransformation(dashboard.DataTransformerConfig{
			Id: "organize",
			Options: map[string]any{
				"excludeByName": map[string]any{
					"Time":       true,
					"__name__":   true,
					"container":  true,
					"endpoint":   true,
					"instance":   true,
					"job":        true,
					"namespace":  true,
					"pod":        true,
					"prometheus": true,
					"service":    true,
				},
				"renameByName": map[string]any{
					"Value": "MarkValue (USD)",
				},
			},
		}).
		Align(common.FieldTextAlignmentLeft).
		OverrideByName("MarkValue (USD)", []dashboard.DynamicConfigValue{
			{Id: "unit", Value: "currencyUSD"},
			{Id: "custom.align", Value: "right"},
		})
}
