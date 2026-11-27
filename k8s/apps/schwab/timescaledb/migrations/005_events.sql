-- Events recorded by go-event-feed and written by go-market-store (news-signals phase 1d).
-- Rows are never updated: a revision is a new row whose supersedes holds the event_id it revises.
-- events_current lists only rows nothing supersedes.
-- No retention policy: events are research history that cannot be re-fetched.
CREATE TABLE IF NOT EXISTS events (
  event_id        bigint GENERATED ALWAYS AS IDENTITY,
  received_at     timestamptz NOT NULL,   -- our first sight; the partition column
  symbol          text,                   -- null for market-wide events
  event_type      text        NOT NULL,   -- headline, earnings_scheduled, earnings_reported, rating_change, insider_trade, filing, fda_meeting, market_price, recommendation_trend
  source          text        NOT NULL,   -- finnhub, kalshi, ...
  source_event_id text        NOT NULL,   -- the source's own id, or a stable key we derive
  event_time      timestamptz,            -- when it happens or is scheduled for
  published_at    timestamptz,            -- the source's own time
  known_at        timestamptz NOT NULL,   -- greatest(published_at, received_at): earliest a strategy may use it
  payload         jsonb       NOT NULL,
  content_hash    bytea       NOT NULL,   -- sha256 of the canonical payload; the writer compares it to skip unchanged polls
  supersedes      bigint,                 -- event_id of the row this one revises
  PRIMARY KEY (event_id, received_at)
);
SELECT create_hypertable('events', 'received_at', chunk_time_interval => INTERVAL '7 days', if_not_exists => true);
CREATE INDEX IF NOT EXISTS events_source_key ON events (source, source_event_id, received_at DESC);
CREATE INDEX IF NOT EXISTS events_symbol_time ON events (symbol, event_type, received_at DESC);
ALTER TABLE events SET (
  timescaledb.compress,
  timescaledb.compress_segmentby = 'source, event_type',
  timescaledb.compress_orderby = 'received_at DESC, event_id'
);
SELECT add_compression_policy('events', INTERVAL '7 days', if_not_exists => true);
CREATE OR REPLACE VIEW events_current AS
  SELECT e.* FROM events e
  WHERE NOT EXISTS (SELECT 1 FROM events n WHERE n.supersedes = e.event_id);
