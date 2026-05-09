package papertrading

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/grafana/grafana-foundation-sdk/go/dashboard"

	"github.com/Chalupa-Tech/chalupa-infra/scripts/dashboards/internal/common/banner"
	"github.com/Chalupa-Tech/chalupa-infra/scripts/dashboards/internal/common/normalize"
	"github.com/Chalupa-Tech/chalupa-infra/scripts/dashboards/internal/papertrading/rows"
)

const (
	dashboardTitle = "Paper Trading"
	dashboardUID   = "paper-trading"
	bannerPanelID  = 1
	bannerHeight   = 3 // matches banner.Panel's gridPos.h
)

// rowFunc appends a row's panels to the Dashboard builder and returns
// the y-units this row consumed (so the orchestrator can advance).
type rowFunc func(db *dashboard.DashboardBuilder, yBase int) int

// orderedRows defines the top-to-bottom row order. Order matches the
// phase-7b committed JSON, with phase-8's Orders row sitting after
// Activity (lifecycle of orders is conceptually adjacent to the
// activity counts; both look at "what is/was happening" rather than
// long-term quality).
var orderedRows = []rowFunc{
	rows.Summary,
	rows.Risk,
	rows.StrategyQuality,
	rows.StrategyComparison,
	rows.Account,
	rows.PositionsTrades,
	rows.Activity,
	rows.Orders,
	rows.WorkingOrders,
	rows.SimulatorHealth,
	rows.Reconciliation,
	rows.ExecutionQuality,
	rows.FillRealism,
	rows.Hygiene,
}

func compose() (dashboard.Dashboard, error) {
	db := dashboard.NewDashboardBuilder(dashboardTitle).
		Uid(dashboardUID).
		Editable().
		Tags([]string{"paper-trading"}).
		Refresh("30s").
		Time("now-6h", "now").
		WithVariable(bookIDVar()).
		WithVariable(symbolVar()).
		WithVariable(strategyVar()).
		WithPanel(banner.Panel(bannerPanelID, 0))

	yBase := bannerHeight
	for _, build := range orderedRows {
		yBase += build(db, yBase)
	}
	return db.Build()
}

// Render returns the canonical, committable JSON for the paper-trading
// dashboard. It builds twice and fails loud if the two builds differ —
// catches non-determinism before the CI gate has to.
func Render() ([]byte, error) {
	first, err := encode()
	if err != nil {
		return nil, err
	}
	second, err := encode()
	if err != nil {
		return nil, err
	}
	if !bytes.Equal(first, second) {
		return nil, fmt.Errorf("non-deterministic build: two consecutive renders produced different JSON")
	}
	return first, nil
}

func encode() ([]byte, error) {
	model, err := compose()
	if err != nil {
		return nil, fmt.Errorf("compose: %w", err)
	}
	raw, err := json.Marshal(model)
	if err != nil {
		return nil, fmt.Errorf("marshal SDK output: %w", err)
	}
	return normalize.Render(raw)
}
