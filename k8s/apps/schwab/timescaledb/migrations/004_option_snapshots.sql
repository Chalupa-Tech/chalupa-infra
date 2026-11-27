-- Option chain snapshots recorded by go-schwab-feed and written by go-market-store (news-signals phase 1d).
-- bid and ask of the underlying are kept apart, not as a mid. real is the 4-byte float rule; strikes to the cent are exact below 100,000.
-- The unique key holds the partition column (snapshot_at) and the segmentby column (symbol), which TimescaleDB requires.
-- No retention policy: snapshots are research history that cannot be re-fetched.
CREATE TABLE IF NOT EXISTS option_snapshots (
  snapshot_at     timestamptz NOT NULL,
  received_at     timestamptz NOT NULL,
  symbol          text        NOT NULL,
  expiry          date        NOT NULL,
  strike          real        NOT NULL,
  type            "char"      NOT NULL,  -- 'C' or 'P'
  bid             real,
  ask             real,
  volume          integer,
  open_interest   integer,
  iv              real,
  delta           real,
  underlying_bid  real,
  underlying_ask  real,
  underlying_last real,
  trigger         text        NOT NULL,  -- interval | 1545 | close
  source          text        NOT NULL,
  UNIQUE (symbol, snapshot_at, expiry, strike, type)
);
SELECT create_hypertable('option_snapshots', 'snapshot_at', chunk_time_interval => INTERVAL '1 day', if_not_exists => true);
ALTER TABLE option_snapshots SET (
  timescaledb.compress,
  timescaledb.compress_segmentby = 'symbol',
  timescaledb.compress_orderby = 'snapshot_at DESC, expiry, strike, type'
);
SELECT add_compression_policy('option_snapshots', INTERVAL '7 days', if_not_exists => true);
