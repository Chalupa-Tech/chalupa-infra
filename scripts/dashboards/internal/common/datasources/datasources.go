// Package datasources holds the datasource UID + type constants used
// across all chalupa-infra dashboards.
package datasources

import (
	cog "github.com/grafana/grafana-foundation-sdk/go/cog"
	"github.com/grafana/grafana-foundation-sdk/go/dashboard"
)

const (
	VictoriaUID  = "VictoriaMetrics"
	VictoriaType = "prometheus"

	TimescaleUID  = "timescaledb"
	TimescaleType = "grafana-postgresql-datasource"
)

func Victoria() dashboard.DataSourceRef {
	return dashboard.DataSourceRef{
		Type: cog.ToPtr(VictoriaType),
		Uid:  cog.ToPtr(VictoriaUID),
	}
}

func Timescale() dashboard.DataSourceRef {
	return dashboard.DataSourceRef{
		Type: cog.ToPtr(TimescaleType),
		Uid:  cog.ToPtr(TimescaleUID),
	}
}
