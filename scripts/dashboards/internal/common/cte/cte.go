// Package cte holds shared SQL CTEs reused across multiple panels.
//
// Conventions:
//   - Performance panels read paper_equity, the per-minute equity summary
//     the trader writes (one row per book per minute, never retained
//     away). Do not rebuild equity from paper_cash and paper_positions:
//     paper_positions grows by tens of thousands of rows a day, and an
//     unbounded `GROUP BY time, book_id` over it OOM-killed TimescaleDB on
//     2026-11-13. Use Equity.
//   - A panel that does need a paper_cash / paper_positions join (Live
//     positions) MUST use the exact join key
//     `pc.time = pos.time AND pc.book_id = pos.book_id`, no time_bucket on
//     the join, and bound it in time. See the paper_cash_positions_join_key
//     memory.
//   - Variable interpolation uses Grafana's $book_id, $strategy,
//     $__timeFilter(time) — preserved verbatim in the SQL strings.
package cte

// PanelBucket is a time_bucket width matched to the panel's resolution
// (Grafana's $__interval_ms), floored at 60s because paper_equity has
// one row per book per minute.
const PanelBucket = "make_interval(secs => GREATEST($__interval_ms, 60000) / 1000.0)"

// InRange restricts a scan to the dashboard's visible time range.
const InRange = "$__timeFilter(time)"

// lookback31d covers the visible range plus the 30 days of history the
// rolling daily-return windows need.
const lookback31d = "time >= $__timeFrom()::timestamptz - interval '31 days'" +
	" AND time <= $__timeTo()::timestamptz"

// Equity returns the CTE `equity_ts` (no leading WITH).
// equity_ts columns: bucket, time, book_id, equity.
//
// It reads paper_equity and keeps the last row in each bucket per book,
// so the cost scales with the number of chart points, not with the size
// of paper_cash or paper_positions. The trader values every position at
// its last known mark, so a minute with no fresh quote (off-hours) holds
// its last equity instead of falling to cash, and no zero-mark filter is
// needed here.
//
// rangePred is a predicate on paper_equity.time (InRange or a lookback);
// bucket is a time_bucket width expression; filterBooks adds the
// $book_id filter.
func Equity(rangePred, bucket string, filterBooks bool) string {
	books := ""
	if filterBooks {
		books = " AND book_id IN ($book_id)"
	}
	return "equity_ts AS (" +
		"  SELECT time_bucket(" + bucket + ", time) AS bucket," +
		"         MAX(time) AS time, book_id, LAST(equity, time) AS equity" +
		"  FROM paper_equity" +
		"  WHERE " + rangePred + books +
		"  GROUP BY book_id, bucket" +
		")"
}

// DailyReturnsCTE is the 30-day rolling daily-returns CTE for Sharpe /
// Sortino / rolling return panels. Output columns: book_id, trade_date, ret.
// Equity is sampled hourly over the visible range plus 31 days.
var DailyReturnsCTE = "WITH " + Equity(lookback31d, "interval '1 hour'", true) +
	", daily AS (" +
	"  SELECT" +
	"    book_id," +
	"    time_bucket('1 day', time) AS trade_date," +
	"    FIRST(equity, time) AS sod," +
	"    LAST(equity, time)  AS eod" +
	"  FROM equity_ts" +
	"  GROUP BY book_id, trade_date" +
	"), returns AS (" +
	"  SELECT book_id, trade_date, (eod - sod) / NULLIF(sod, 0) AS ret" +
	"  FROM daily" +
	") "
