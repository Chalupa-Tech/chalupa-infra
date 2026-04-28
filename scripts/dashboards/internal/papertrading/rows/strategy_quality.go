// strategy_quality.go — Sharpe, Sortino, Profit factor, Avg-win/Avg-loss.
// Renumbered: row 400, panels 401-404.
// (Phase-7b ids: row 110, panels 111-114.)
package rows

import (
	"github.com/grafana/grafana-foundation-sdk/go/common"
	"github.com/grafana/grafana-foundation-sdk/go/dashboard"
	"github.com/grafana/grafana-foundation-sdk/go/stat"
	"github.com/grafana/grafana-foundation-sdk/go/timeseries"

	"github.com/Chalupa-Tech/chalupa-infra/scripts/dashboards/internal/common/cte"
	"github.com/Chalupa-Tech/chalupa-infra/scripts/dashboards/internal/common/datasources"
	"github.com/Chalupa-Tech/chalupa-infra/scripts/dashboards/internal/common/layout"
	"github.com/Chalupa-Tech/chalupa-infra/scripts/dashboards/internal/common/sql"
	"github.com/Chalupa-Tech/chalupa-infra/scripts/dashboards/internal/common/thresholds"
)

const (
	strategyQualityRowID    = 400
	strategyQualityRowTitle = "Strategy quality"
	strategyQualityHeight   = 13 // 1 + 8 (Sharpe/Sortino) + 4 (PF / Avg)
)

func StrategyQuality(db *dashboard.DashboardBuilder, yBase int) int {
	db.WithRow(layout.Row(strategyQualityRowID, yBase, strategyQualityRowTitle))
	db.WithPanel(sharpeRolling(yBase + 1))
	db.WithPanel(sortinoRolling(yBase + 1))
	db.WithPanel(profitFactor(yBase + 9))
	db.WithPanel(avgWinAvgLoss(yBase + 9))
	return strategyQualityHeight
}

func sharpeRolling(y int) *timeseries.PanelBuilder {
	rawSQL := cte.DailyReturnsCTE +
		"SELECT trade_date AS time, book_id AS metric," +
		"   AVG(ret) OVER w / NULLIF(STDDEV_SAMP(ret) OVER w, 0) * SQRT(252) AS sharpe" +
		" FROM returns" +
		" WINDOW w AS (PARTITION BY book_id ORDER BY trade_date" +
		"              ROWS BETWEEN 29 PRECEDING AND CURRENT ROW)" +
		" ORDER BY trade_date"
	return timeseries.NewPanelBuilder().
		Id(401).
		Title("Sharpe ratio (30d rolling, annualized, rf=0)").
		Description(
			"paper-trading-realism phase-7b: 30-day rolling Sharpe = " +
				"mean(daily_return) / stddev(daily_return) × √252. Risk-free " +
				"rate hardcoded to 0 (matches phase-6 UTC daily-P&L convention; " +
				"revisit when a strategy is a real promotion candidate). " +
				"Promotion threshold ≥ 1.0 per sandbox-live-separation brief. " +
				"Renders NULL for books with < 30 days of returns.").
		GridPos(layout.Pos(0, y, 12, 8)).
		Datasource(datasources.Timescale()).
		WithTarget(sql.TimeSeries("A", rawSQL)).
		Unit("short").
		LineWidth(2).
		FillOpacity(10).
		ColorScheme(dashboard.NewFieldColorBuilder().Mode(dashboard.FieldColorModeIdPaletteClassic)).
		Legend(common.NewVizLegendOptionsBuilder().
			DisplayMode(common.LegendDisplayModeList).
			Placement(common.LegendPlacementBottom))
}

func sortinoRolling(y int) *timeseries.PanelBuilder {
	rawSQL := cte.DailyReturnsCTE +
		"SELECT trade_date AS time, book_id AS metric," +
		"   AVG(ret) OVER w / NULLIF(STDDEV_SAMP(ret) FILTER (WHERE ret < 0) OVER w, 0) * SQRT(252) AS sortino" +
		" FROM returns" +
		" WINDOW w AS (PARTITION BY book_id ORDER BY trade_date" +
		"              ROWS BETWEEN 29 PRECEDING AND CURRENT ROW)" +
		" ORDER BY trade_date"
	return timeseries.NewPanelBuilder().
		Id(402).
		Title("Sortino ratio (30d rolling, annualized, rf=0)").
		Description(
			"paper-trading-realism phase-7b: 30-day rolling Sortino = " +
				"mean(daily_return) / stddev(daily_return WHERE ret<0) × √252. " +
				"Same numerator as Sharpe; denominator only counts losing days, " +
				"so Sortino > Sharpe means losses are concentrated (good). " +
				"Renders NULL for books with no losing days in the window or < " +
				"30 days of returns.").
		GridPos(layout.Pos(12, y, 12, 8)).
		Datasource(datasources.Timescale()).
		WithTarget(sql.TimeSeries("A", rawSQL)).
		Unit("short").
		LineWidth(2).
		FillOpacity(10).
		ColorScheme(dashboard.NewFieldColorBuilder().Mode(dashboard.FieldColorModeIdPaletteClassic)).
		Legend(common.NewVizLegendOptionsBuilder().
			DisplayMode(common.LegendDisplayModeList).
			Placement(common.LegendPlacementBottom))
}

func profitFactor(y int) *stat.PanelBuilder {
	rawSQL := "SELECT book_id," +
		"   SUM(realized_pl) FILTER (WHERE realized_pl > 0) /" +
		"   NULLIF(ABS(SUM(realized_pl) FILTER (WHERE realized_pl < 0)), 0) AS profit_factor" +
		" FROM paper_fills" +
		" WHERE realized_pl IS NOT NULL AND book_id IN ($book_id) AND $__timeFilter(time)" +
		" GROUP BY book_id"
	return stat.NewPanelBuilder().
		Id(403).
		Title("Profit factor").
		Description(
			"paper-trading-realism phase-7b: SUM(winning realized_pl) / " +
				"|SUM(losing realized_pl)|, per book, over the visible time " +
				"range. > 1.2 decent; > 2.0 strong (per " +
				"sandbox-live-separation brief — promotion threshold is 1.2). " +
				"Renders NULL for books with no losing trades in the window.").
		GridPos(layout.Pos(0, y, 12, 4)).
		Datasource(datasources.Timescale()).
		WithTarget(sql.Table("A", rawSQL)).
		Unit("short").
		Thresholds(thresholds.Build(thresholds.ProfitFactor)).
		ColorMode(common.BigValueColorModeBackground).
		GraphMode(common.BigValueGraphModeNone).
		ReduceOptions(common.NewReduceDataOptionsBuilder().
			Calcs([]string{"lastNotNull"}).
			Fields("").
			Values(false))
}

func avgWinAvgLoss(y int) *timeseries.PanelBuilder {
	rawSQL := "SELECT" +
		"   time_bucket('1 day', time) AS time," +
		"   book_id || '-win'  AS metric," +
		"   AVG(realized_pl) FILTER (WHERE realized_pl > 0) AS v" +
		" FROM paper_fills" +
		" WHERE realized_pl IS NOT NULL AND book_id IN ($book_id) AND $__timeFilter(time)" +
		" GROUP BY 1, book_id" +
		" UNION ALL" +
		" SELECT" +
		"   time_bucket('1 day', time) AS time," +
		"   book_id || '-loss' AS metric," +
		"   AVG(realized_pl) FILTER (WHERE realized_pl < 0) AS v" +
		" FROM paper_fills" +
		" WHERE realized_pl IS NOT NULL AND book_id IN ($book_id) AND $__timeFilter(time)" +
		" GROUP BY 1, book_id" +
		" ORDER BY time"
	return timeseries.NewPanelBuilder().
		Id(404).
		Title("Avg win / Avg loss (per day)").
		Description(
			"paper-trading-realism phase-7b: per-book daily mean realized_pl " +
				"on winning trades (positive series) vs losing trades (negative " +
				"series). Captures fat-tail shape — even a 55%-win-rate strategy " +
				"bleeds if avg loss > avg win in absolute terms. Series suffix " +
				"`-win` / `-loss` so multi-book comparisons stay unambiguous.").
		GridPos(layout.Pos(12, y, 12, 4)).
		Datasource(datasources.Timescale()).
		WithTarget(sql.TimeSeries("A", rawSQL)).
		Unit("currencyUSD").
		LineWidth(2).
		FillOpacity(10).
		ColorScheme(dashboard.NewFieldColorBuilder().Mode(dashboard.FieldColorModeIdPaletteClassic)).
		Legend(common.NewVizLegendOptionsBuilder().
			DisplayMode(common.LegendDisplayModeList).
			Placement(common.LegendPlacementBottom))
}
