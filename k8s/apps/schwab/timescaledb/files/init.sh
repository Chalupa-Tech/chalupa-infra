#!/bin/bash
set -e

psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname "$POSTGRES_DB" <<-EOSQL
    -- TimescaleDB schema for Schwab market data
    -- Spec 1138: go-schwab-feed → NATS → go-market-store → TimescaleDB

    CREATE EXTENSION IF NOT EXISTS timescaledb;

    -- Multi-frequency OHLCV candle data from Schwab price history API
    CREATE TABLE IF NOT EXISTS candles (
        time      TIMESTAMPTZ      NOT NULL,
        symbol    TEXT             NOT NULL,
        frequency TEXT             NOT NULL DEFAULT '1d',
        open      DOUBLE PRECISION,
        high      DOUBLE PRECISION,
        low       DOUBLE PRECISION,
        close     DOUBLE PRECISION,
        volume    BIGINT,
        source    TEXT DEFAULT 'schwab'
    );
    SELECT create_hypertable('candles', 'time', if_not_exists => TRUE);
    CREATE UNIQUE INDEX IF NOT EXISTS idx_candles_unique ON candles (time, symbol, frequency);
    CREATE INDEX IF NOT EXISTS idx_candles_symbol_freq_time ON candles (symbol, frequency, time DESC);

    -- Compression on candles (segmentby symbol+frequency, orderby time)
    ALTER TABLE candles SET (
        timescaledb.compress,
        timescaledb.compress_segmentby = 'symbol, frequency',
        timescaledb.compress_orderby = 'time DESC'
    );
    SELECT add_compression_policy('candles', INTERVAL '30 days', if_not_exists => true);

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

    -- Compression on quotes
    ALTER TABLE quotes SET (
        timescaledb.compress,
        timescaledb.compress_segmentby = 'symbol',
        timescaledb.compress_orderby = 'time DESC'
    );
    SELECT add_compression_policy('quotes', INTERVAL '7 days', if_not_exists => true);

    -- Frequency-based retention (daily candles kept forever)
    CREATE OR REPLACE FUNCTION cleanup_old_candles() RETURNS void AS \$func\$
    BEGIN
        DELETE FROM candles WHERE frequency = '5m' AND time < now() - INTERVAL '90 days';
        DELETE FROM candles WHERE frequency = '1h' AND time < now() - INTERVAL '1 year';
    END;
    \$func\$ LANGUAGE plpgsql;
    SELECT add_job('cleanup_old_candles', '1 day', if_not_exists => true);

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
