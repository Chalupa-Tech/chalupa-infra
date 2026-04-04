-- TimescaleDB schema for Schwab market data
-- Spec 1138: go-schwab-feed → NATS → go-market-store → TimescaleDB

CREATE EXTENSION IF NOT EXISTS timescaledb;

-- Daily OHLCV candle data from Schwab price history API
CREATE TABLE IF NOT EXISTS candles (
    time    TIMESTAMPTZ      NOT NULL,
    symbol  TEXT             NOT NULL,
    open    DOUBLE PRECISION,
    high    DOUBLE PRECISION,
    low     DOUBLE PRECISION,
    close   DOUBLE PRECISION,
    volume  BIGINT,
    source  TEXT DEFAULT 'schwab'
);
SELECT create_hypertable('candles', 'time', if_not_exists => TRUE);
CREATE INDEX IF NOT EXISTS idx_candles_symbol_time ON candles (symbol, time DESC);

-- Intraday quote snapshots (append-only, 30-day retention)
CREATE TABLE IF NOT EXISTS quotes (
    time       TIMESTAMPTZ      NOT NULL,
    symbol     TEXT             NOT NULL,
    price      DOUBLE PRECISION,
    open_price DOUBLE PRECISION,
    high_price DOUBLE PRECISION,
    low_price  DOUBLE PRECISION,
    volume     BIGINT,
    change_pct DOUBLE PRECISION,
    bid        DOUBLE PRECISION,
    ask        DOUBLE PRECISION,
    source     TEXT DEFAULT 'schwab'
);
SELECT create_hypertable('quotes', 'time', if_not_exists => TRUE);
CREATE INDEX IF NOT EXISTS idx_quotes_symbol_time ON quotes (symbol, time DESC);
SELECT add_retention_policy('quotes', INTERVAL '30 days', if_not_exists => TRUE);

-- Grafana read-only user (password set via env var at runtime)
DO $$
BEGIN
    IF NOT EXISTS (SELECT FROM pg_catalog.pg_roles WHERE rolname = 'grafana') THEN
        CREATE ROLE grafana WITH LOGIN PASSWORD 'changeme';
    END IF;
END
$$;
GRANT CONNECT ON DATABASE market TO grafana;
GRANT USAGE ON SCHEMA public TO grafana;
GRANT SELECT ON ALL TABLES IN SCHEMA public TO grafana;
ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT SELECT ON TABLES TO grafana;
