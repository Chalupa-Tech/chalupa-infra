// Package storagelogs composes the Storage & Log Pipeline dashboard
// (platform phase-95): Longhorn volume health and the log pipeline
// (Vector → VictoriaLogs) plus vmagent's queue, the signals that were
// missing during the 2026-11-17 tpi1 incident. Each panel matches an
// alert in k8s/platform/observability/templates/alert-rules.yaml
// (group storage-and-log-pipeline) and draws that alert's threshold.
package storagelogs

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/grafana/grafana-foundation-sdk/go/dashboard"

	"github.com/Chalupa-Tech/chalupa-infra/scripts/dashboards/internal/common/layout"
	"github.com/Chalupa-Tech/chalupa-infra/scripts/dashboards/internal/common/normalize"
)

const (
	dashboardTitle = "Storage & Log Pipeline"
	dashboardUID   = "storage-logs"

	longhornRowID = 100
	pipelineRowID = 200
	longhornRowY  = 0
	pipelineRowY  = 15
)

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
	}
}

func compose() (dashboard.Dashboard, error) {
	db := dashboard.NewDashboardBuilder(dashboardTitle).
		Uid(dashboardUID).
		Editable().
		Tooltip(dashboard.DashboardCursorSyncCrosshair).
		Tags([]string{"chalupa", "platform", "longhorn", "logs"}).
		Refresh("1m").
		Time("now-24h", "now")
	for _, l := range crossDashLinks() {
		db.Link(l)
	}

	db.WithRow(layout.Row(longhornRowID, longhornRowY, "Longhorn"))
	db.WithPanel(volumeRobustness(longhornRowY + 1))
	db.WithPanel(managerLogRate(longhornRowY + 7))
	db.WithPanel(csiFormatFailures(longhornRowY + 7))
	db.WithPanel(mkfsGuardAge(longhornRowY + 7))

	db.WithRow(layout.Row(pipelineRowID, pipelineRowY, "Log pipeline"))
	db.WithPanel(logIngestRate(pipelineRowY + 1))
	db.WithPanel(topLogStreams(pipelineRowY + 1))
	db.WithPanel(pipelineCPU(pipelineRowY + 9))
	db.WithPanel(vmagentQueue(pipelineRowY + 9))
	return db.Build()
}

// Render returns the canonical, committable JSON. Builds twice and fails
// if the two builds differ.
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
