#!/bin/bash
set -e

psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname "$POSTGRES_DB" <<-EOSQL
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
    CREATE UNIQUE INDEX IF NOT EXISTS idx_candles_unique ON candles (time, symbol);
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

    -- 5-minute candles from quote snapshots (24h retention)
    CREATE MATERIALIZED VIEW IF NOT EXISTS candles_5m
    WITH (timescaledb.continuous) AS
    SELECT
        time_bucket('5 minutes', time) AS time,
        symbol,
        first(price, time) AS open,
        max(high_price) AS high,
        min(low_price) AS low,
        last(price, time) AS close,
        max(volume) AS volume,
        'quote_agg' AS source
    FROM quotes
    GROUP BY time_bucket('5 minutes', time), symbol
    WITH NO DATA;
    SELECT add_continuous_aggregate_policy('candles_5m',
        start_offset => INTERVAL '1 day',
        end_offset => INTERVAL '5 minutes',
        schedule_interval => INTERVAL '5 minutes',
        if_not_exists => TRUE);
    SELECT add_retention_policy('candles_5m', INTERVAL '24 hours', if_not_exists => TRUE);

    -- Hourly candles from quote snapshots (kept indefinitely)
    CREATE MATERIALIZED VIEW IF NOT EXISTS candles_1h
    WITH (timescaledb.continuous) AS
    SELECT
        time_bucket('1 hour', time) AS time,
        symbol,
        first(price, time) AS open,
        max(high_price) AS high,
        min(low_price) AS low,
        last(price, time) AS close,
        max(volume) AS volume,
        'quote_agg_1h' AS source
    FROM quotes
    GROUP BY time_bucket('1 hour', time), symbol
    WITH NO DATA;
    SELECT add_continuous_aggregate_policy('candles_1h',
        start_offset => INTERVAL '7 days',
        end_offset => INTERVAL '1 hour',
        schedule_interval => INTERVAL '1 hour',
        if_not_exists => TRUE);

    -- App user for go-market-store
    DO \$\$
    BEGIN
        IF NOT EXISTS (SELECT FROM pg_catalog.pg_roles WHERE rolname = 'marketstore') THEN
            CREATE ROLE marketstore WITH LOGIN PASSWORD '${APP_PASSWORD}';
        END IF;
    END
    \$\$;
    GRANT CONNECT ON DATABASE market TO marketstore;
    GRANT USAGE ON SCHEMA public TO marketstore;
    GRANT SELECT, INSERT, UPDATE ON ALL TABLES IN SCHEMA public TO marketstore;
    ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT SELECT, INSERT, UPDATE ON TABLES TO marketstore;
    GRANT CREATE ON SCHEMA public TO marketstore;

    -- Grafana read-only user
    DO \$\$
    BEGIN
        IF NOT EXISTS (SELECT FROM pg_catalog.pg_roles WHERE rolname = 'grafana') THEN
            CREATE ROLE grafana WITH LOGIN PASSWORD '${GRAFANA_PASSWORD}';
        END IF;
    END
    \$\$;
    GRANT CONNECT ON DATABASE market TO grafana;
    GRANT USAGE ON SCHEMA public TO grafana;
    GRANT SELECT ON ALL TABLES IN SCHEMA public TO grafana;
    GRANT SELECT ON ALL TABLES IN SCHEMA _timescaledb_internal TO grafana;
    ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT SELECT ON TABLES TO grafana;
EOSQL
