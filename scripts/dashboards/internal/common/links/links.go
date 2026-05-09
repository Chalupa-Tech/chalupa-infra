// Package links wraps dashboard.DashboardLinkBuilder with chalupa-tech
// defaults for cross-dashboard panel data links (drilldowns).
//
// Use the builders returned here with PanelBuilder.DataLinks(), not
// PanelBuilder.Links(). The two SDK methods write to different JSON
// locations:
//
//   - DataLinks → fieldConfig.defaults.links (per-series data links).
//     ${__field.labels.<name>}, ${__series.name}, and
//     ${__data.fields.<name>} are only resolved here.
//   - Links → panel-level links (header chevron menu). Only resolves
//     ${__from}, ${__to}, and ${var-<dashboardvar>}.
//
// Every drilldown defined for the cross-dashboard navigation plan uses
// at least one series-context token, so DataLinks is the correct method
// for the chalupa-tech navigation contract.
//
// Defaults applied by To():
//
//   - Title prefixed with "→ " — visual cue this is a drilldown.
//   - TargetBlank = true — operator keeps the source dashboard open.
//   - Type = link — external URL, not Grafana's internal `dashboards`
//     filter.
//
// The url MUST embed `from=${__from}&to=${__to}` explicitly. KeepTime
// is intentionally NOT set: we want the source dashboard's currently
// visible window forwarded as URL params, not Grafana's built-in
// time-range carry-over (which uses the URL fragment and races with
// the target's default-time on initial load).
//
// Variable seeding (`var-symbol=...`, `var-book_id=...`) is the caller's
// responsibility — pick the interpolation token that matches the source
// panel's series labels.
package links

import (
	"github.com/grafana/grafana-foundation-sdk/go/dashboard"
)

// To returns a panel data-link builder for cross-dashboard drilldowns.
// targetTitle is rendered after the "→ " prefix.
func To(targetTitle, url string) *dashboard.DashboardLinkBuilder {
	return dashboard.NewDashboardLinkBuilder("→ " + targetTitle).
		Url(url).
		Type(dashboard.DashboardLinkTypeLink).
		TargetBlank(true)
}
