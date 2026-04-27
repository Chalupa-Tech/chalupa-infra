// simulator_health.go — refusal rates, scheduler tick health, queue depth.
// Phase-7b panel ids: row 16, panels 17-19.
// Phase-7e slice: row stub. Panels ship in 7e2.
package rows

import "github.com/grafana/grafana-foundation-sdk/go/dashboard"

func SimulatorHealth(_ *dashboard.DashboardBuilder, _ int) int { return 0 }
