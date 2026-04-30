package schwabfeed

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/grafana/grafana-foundation-sdk/go/dashboard"

	"github.com/Chalupa-Tech/chalupa-infra/scripts/dashboards/internal/common/normalize"
	"github.com/Chalupa-Tech/chalupa-infra/scripts/dashboards/internal/schwabfeed/rows"
)

const (
	dashboardTitle = "Schwab Market Feed"
	dashboardUID   = "schwab-feed"
)

// rowFunc appends a row's panels to the Dashboard builder and returns
// the y-units this row consumed (so the orchestrator can advance).
type rowFunc func(db *dashboard.DashboardBuilder, yBase int) int

// orderedRows defines the top-to-bottom row order. Order matches the
// pre-port hand-written JSON.
var orderedRows = []rowFunc{
	rows.FeedHealth,
	rows.MarketData,
}

// crossDashLinks reproduces the pre-port navigation strip. These are
// the same five links the hand-written JSON shipped; preserve byte
// shape so the cutover diff is benign. Cross-dashboard drill-down
// links (variable-passing data links) are out of scope for phase-2 —
// see briefs/dashboard-navigation-plan.md / phase-3.
func crossDashLinks() []*dashboard.DashboardLinkBuilder {
	link := func(title, url, icon string) *dashboard.DashboardLinkBuilder {
		return dashboard.NewDashboardLinkBuilder(title).
			Type(dashboard.DashboardLinkTypeLink).
			Icon(icon).
			Url(url).
			TargetBlank(false)
	}
	return []*dashboard.DashboardLinkBuilder{
		link("Cluster Overview", "/d/cluster-overview", "dashboard"),
		link("Logs Explorer", "/d/victorialogs-explorer", "doc"),
		link("Traces Explorer", "/d/traces-explorer", "bolt"),
		link("Service RED", "/d/service-red", "dashboard"),
		link("Trading Dashboard", "/d/schwab-trading", "graph-bar"),
	}
}

func compose() (dashboard.Dashboard, error) {
	db := dashboard.NewDashboardBuilder(dashboardTitle).
		Uid(dashboardUID).
		Editable().
		Tooltip(dashboard.DashboardCursorSyncCrosshair).
		Tags([]string{"chalupa", "schwab-feed", "trading"}).
		Refresh("30s").
		Time("now-6h", "now").
		WithVariable(registrationVar()).
		WithVariable(symbolVar())
	for _, l := range crossDashLinks() {
		db.Link(l)
	}
	yBase := 0
	for _, build := range orderedRows {
		yBase += build(db, yBase)
	}
	return db.Build()
}

// Render returns the canonical, committable JSON for the schwab-feed
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
