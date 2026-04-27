// Package papertrading composes the paper-trading Grafana dashboard.
package papertrading

import (
	"github.com/grafana/grafana-foundation-sdk/go/dashboard"

	"github.com/Chalupa-Tech/chalupa-infra/scripts/dashboards/internal/common/datasources"
)

const bookIDQuery = "SELECT DISTINCT book_id FROM (" +
	"SELECT book_id FROM paper_fills WHERE time > NOW() - INTERVAL '7 days' " +
	"UNION " +
	"SELECT book_id FROM paper_cash WHERE time > NOW() - INTERVAL '7 days'" +
	") sub ORDER BY book_id"

const symbolQuery = "SELECT DISTINCT symbol FROM paper_positions ORDER BY symbol"

const strategyQuery = "SELECT DISTINCT strategy FROM paper_fills ORDER BY strategy"

// queryVariable is a small constructor for our 3 multi-select query
// variables, all backed by TimescaleDB.
func queryVariable(name, query string) *dashboard.QueryVariableBuilder {
	return dashboard.NewQueryVariableBuilder(name).
		Datasource(datasources.Timescale()).
		Query(dashboard.StringOrMap{String: stringPtr(query)}).
		Multi(true).
		IncludeAll(true).
		Refresh(dashboard.VariableRefreshOnDashboardLoad)
}

func bookIDVar() *dashboard.QueryVariableBuilder {
	return queryVariable("book_id", bookIDQuery)
}

func symbolVar() *dashboard.QueryVariableBuilder {
	return queryVariable("symbol", symbolQuery)
}

func strategyVar() *dashboard.QueryVariableBuilder {
	return queryVariable("strategy", strategyQuery)
}

func stringPtr(s string) *string { return &s }
