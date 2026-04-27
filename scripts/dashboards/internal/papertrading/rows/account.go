// account.go — cash, equity, exposure (per book).
// Phase-7b panel ids: row 2, panels 3-6, 20, 130.
// Phase-7e slice: row stub. Panels ship in 7e2.
package rows

import "github.com/grafana/grafana-foundation-sdk/go/dashboard"

func Account(_ *dashboard.DashboardBuilder, _ int) int { return 0 }
