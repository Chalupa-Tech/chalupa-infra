// Package cte holds shared SQL CTEs reused across multiple panels.
//
// Conventions:
//   - Every CTE that joins paper_cash and paper_positions MUST use the
//     exact join key `pc.time = pos.time AND pc.book_id = pos.book_id` —
//     no time_bucket on the join. See the paper_cash_positions_join_key
//     memory.
//   - Never aggregate paper_positions without a time bound. The table
//     grows by tens of thousands of rows a day; an unbounded
//     `GROUP BY time, book_id` over it OOM-killed TimescaleDB on
//     2026-11-13. Use Equity, which samples one snapshot per bucket from
//     paper_cash and looks positions up for those snapshots only.
//   - Variable interpolation uses Grafana's $book_id, $strategy,
//     $__timeFilter(time) — preserved verbatim in the SQL strings.
package cte

// PanelBucket is a time_bucket width matched to the panel's resolution
// (Grafana's $__interval_ms), floored at 60s because snapshots are
// written about once a minute.
const PanelBucket = "make_interval(secs => GREATEST($__interval_ms, 60000) / 1000.0)"

// InRange restricts a scan to the dashboard's visible time range.
const InRange = "$__timeFilter(time)"

// lookback31d covers the visible range plus the 30 days of history the
// rolling daily-return windows need.
const lookback31d = "time >= $__timeFrom()::timestamptz - interval '31 days'" +
	" AND time <= $__timeTo()::timestamptz"

// Equity returns the CTE pair `sample`, `equity_ts` (no leading WITH).
// equity_ts columns: bucket, time, book_id, equity.
//
// It picks the last paper_cash snapshot in each bucket per book, then
// reads paper_positions for exactly those (book_id, time) pairs through
// the (book_id, time, symbol) index, so the cost scales with the number
// of chart points, not with table size. The join stays on the exact
// snapshot time.
//
// Snapshots where a held position has mark_price = 0 are dropped: the
// trader records a zero mark when it has no fresh quote (outside market
// hours), and counting those makes equity fall to the cash balance.
//
// rangePred is a predicate on paper_cash.time (InRange or a lookback);
// bucket is a time_bucket width expression; filterBooks adds the
// $book_id filter.
func Equity(rangePred, bucket string, filterBooks bool) string {
	books := ""
	if filterBooks {
		books = " AND book_id IN ($book_id)"
	}
	return "sample AS (" +
		"  SELECT book_id, time_bucket(" + bucket + ", time) AS bucket, MAX(time) AS time" +
		"  FROM paper_cash" +
		"  WHERE " + rangePred + books +
		"  GROUP BY book_id, bucket" +
		"), equity_ts AS (" +
		"  SELECT s.bucket, s.time, s.book_id, pc.cash + COALESCE(pos.equity, 0) AS equity" +
		"  FROM sample s" +
		"  JOIN paper_cash pc ON pc.book_id = s.book_id AND pc.time = s.time" +
		"  LEFT JOIN LATERAL (" +
		"    SELECT SUM(pp.quantity * pp.mark_price) AS equity," +
		"           BOOL_OR(pp.quantity != 0 AND pp.mark_price = 0) AS unmarked" +
		"    FROM paper_positions pp" +
		"    WHERE pp.book_id = s.book_id AND pp.time = s.time" +
		"  ) pos ON true" +
		"  WHERE NOT COALESCE(pos.unmarked, false)" +
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
