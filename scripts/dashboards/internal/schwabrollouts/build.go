package schwabrollouts

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/grafana/grafana-foundation-sdk/go/dashboard"

	"github.com/Chalupa-Tech/chalupa-infra/scripts/dashboards/internal/common/normalize"
)

const (
	dashboardTitle = "Schwab Argo Rollouts"
	dashboardUID   = "schwab-rollouts"
)

// dashboardTags reflects the phase-4 tag taxonomy: drop "canary"
// (covered all rollout phases, not just canary), pick up the
// "chalupa" prefix and a domain tag.
func dashboardTags() []string {
	return []string{"chalupa", "platform", "argo-rollouts"}
}

func compose() (dashboard.Dashboard, error) {
	db := dashboard.NewDashboardBuilder(dashboardTitle).
		Uid(dashboardUID).
		Editable().
		Tooltip(dashboard.DashboardCursorSyncCrosshair).
		Tags(dashboardTags()).
		Refresh("30s").
		Time("now-6h", "now").
		WithVariable(namespaceVar()).
		WithVariable(serviceVar()).
		WithPanel(rolloutPhase()).
		WithPanel(analysisRunResults()).
		WithPanel(podUpStatus()).
		WithPanel(containerRestarts()).
		WithPanel(canaryVsStableRestarts()).
		WithPanel(rolloutReplicas())
	return db.Build()
}

// Render returns the canonical, committable JSON for the schwab-rollouts
// dashboard. Builds twice and fails loud if the two builds differ —
// catches non-determinism before the CI gate.
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
