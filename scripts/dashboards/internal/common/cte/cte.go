// Package cte holds shared SQL CTEs reused across multiple panels.
//
// Conventions:
//   - Every CTE that joins paper_cash and paper_positions MUST use the
//     exact join key `pc.time = pos.time AND pc.book_id = pos.book_id` —
//     no time_bucket on the join. See the paper_cash_positions_join_key
//     memory.
//   - Variable interpolation uses Grafana's $book_id, $strategy,
//     $__timeFilter(time) — preserved verbatim in the SQL strings.
package cte

// DailyReturnsCTE is the 30-day rolling daily-returns CTE for Sharpe /
// Sortino / rolling return panels. Output columns: book_id, trade_date, ret.
const DailyReturnsCTE = "WITH equity_ts AS (" +
	"  SELECT pc.time, pc.book_id, pc.cash + COALESCE(pos.equity, 0) AS equity" +
	"  FROM paper_cash pc" +
	"  LEFT JOIN (" +
	"    SELECT time, book_id, SUM(quantity * mark_price) AS equity" +
	"    FROM paper_positions GROUP BY time, book_id" +
	"  ) pos ON pc.time = pos.time AND pc.book_id = pos.book_id" +
	"  WHERE pc.book_id IN ($book_id)" +
	"), daily AS (" +
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
