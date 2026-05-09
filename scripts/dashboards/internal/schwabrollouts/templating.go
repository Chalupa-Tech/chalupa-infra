// Package schwabrollouts composes the Schwab Argo Rollouts Grafana dashboard.
package schwabrollouts

import (
	"github.com/grafana/grafana-foundation-sdk/go/dashboard"

	"github.com/Chalupa-Tech/chalupa-infra/scripts/dashboards/internal/common/datasources"
)

const (
	// namespaceVarQuery enumerates the namespaces currently emitting
	// rollout phase samples. The pre-port dashboard hard-coded
	// "schwab-ddowell" via a hidden constant variable; phase-4 promotes
	// this to a single-value picker so multi-tenant rollouts (the
	// paper-trading multi-user direction) become observable without a
	// dashboard edit.
	namespaceVarQuery = "label_values(rollout_phase, exported_namespace)"

	// serviceVarQuery is scoped to the selected namespace so the picker
	// shrinks when the operator narrows it.
	serviceVarQuery = `label_values(rollout_phase{exported_namespace="$namespace"}, name)`
)

// namespaceVar — single-value picker. Non-multi keeps the dependent
// `$service` query (`exported_namespace="$namespace"`) sensible
// without rewriting it as `=~`.
func namespaceVar() *dashboard.QueryVariableBuilder {
	return dashboard.NewQueryVariableBuilder("namespace").
		Datasource(datasources.Victoria()).
		Query(dashboard.StringOrMap{String: stringPtr(namespaceVarQuery)}).
		Refresh(dashboard.VariableRefreshOnDashboardLoad).
		Sort(dashboard.VariableSortAlphabeticalAsc)
}

// serviceVar — multi-select rollout picker. Label "Rollout" preserved
// from the pre-port dashboard.
func serviceVar() *dashboard.QueryVariableBuilder {
	return dashboard.NewQueryVariableBuilder("service").
		Label("Rollout").
		Datasource(datasources.Victoria()).
		Query(dashboard.StringOrMap{String: stringPtr(serviceVarQuery)}).
		Multi(true).
		IncludeAll(true).
		Refresh(dashboard.VariableRefreshOnDashboardLoad).
		Sort(dashboard.VariableSortAlphabeticalAsc)
}

func stringPtr(s string) *string { return &s }
