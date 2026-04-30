// Package schwabfeed composes the Schwab market feed Grafana dashboard.
package schwabfeed

import (
	"github.com/grafana/grafana-foundation-sdk/go/dashboard"

	"github.com/Chalupa-Tech/chalupa-infra/scripts/dashboards/internal/common/datasources"
)

const (
	// registrationVarQuery resolves the set of Schwab broker
	// registrations currently emitting account-value samples.
	registrationVarQuery = "label_values(schwab_account_value, registration)"

	// symbolVarQuery is scoped to the selected registration so the
	// picker shrinks when the operator narrows registration.
	symbolVarQuery = `label_values(schwab_quote_price{registration=~"$registration"}, symbol)`
)

// vmQueryVariable constructs a multi-select VictoriaMetrics-backed
// query variable. Both feed variables follow the same shape — only
// the name + query differ.
func vmQueryVariable(name, query string) *dashboard.QueryVariableBuilder {
	return dashboard.NewQueryVariableBuilder(name).
		Datasource(datasources.Victoria()).
		Query(dashboard.StringOrMap{String: stringPtr(query)}).
		Multi(true).
		IncludeAll(true).
		Refresh(dashboard.VariableRefreshOnDashboardLoad)
}

func registrationVar() *dashboard.QueryVariableBuilder {
	return vmQueryVariable("registration", registrationVarQuery)
}

func symbolVar() *dashboard.QueryVariableBuilder {
	return vmQueryVariable("symbol", symbolVarQuery)
}

func stringPtr(s string) *string { return &s }
