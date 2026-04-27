// Package sql is a thin wrapper over the SDK's UnknownDataquery for
// Postgres / TimescaleDB dataqueries.
//
// The grafana-foundation-sdk ships first-class Dataquery types for
// Prometheus, Loki, Tempo, etc., but no Postgres plugin type. The cog
// runtime exposes UnknownDataquery (a typed map[string]any) for exactly
// this case — the wire JSON shape is identical to what Grafana writes
// when you edit a Postgres panel in the UI.
//
// Keep these helpers minimal. Only fields that show up in our committed
// dashboard JSON are exposed.
package sql

import "github.com/grafana/grafana-foundation-sdk/go/cog/variants"

// TimeSeries returns a Postgres dataquery configured for time-series
// output (Grafana's "Time series" format). Use for panels that plot
// time-bucketed values over the dashboard's time range.
func TimeSeries(refID, rawSQL string) *variants.UnknownDataqueryBuilder {
	return query(refID, "time_series", rawSQL)
}

// Table returns a Postgres dataquery configured for table output. Use
// for table-panel data and for any query whose result is a flat
// rowset (notional aggregations, position rosters, etc.).
func Table(refID, rawSQL string) *variants.UnknownDataqueryBuilder {
	return query(refID, "table", rawSQL)
}

func query(refID, format, rawSQL string) *variants.UnknownDataqueryBuilder {
	return variants.NewUnknownDataqueryBuilderFromObject(variants.UnknownDataquery{
		"refId":  refID,
		"format": format,
		"rawSql": rawSQL,
	})
}
