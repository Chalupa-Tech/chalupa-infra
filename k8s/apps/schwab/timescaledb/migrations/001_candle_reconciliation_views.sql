-- Baseline: the six candle_* reconciliation views added by market-data phases 11 and 12.
-- Definitions are the live ones (pg_get_viewdef, 2026-11-24), so a fresh cluster and the live one agree.
-- baseline-if: SELECT count(*) = 6 FROM pg_views WHERE schemaname = 'public' AND viewname IN ('candle_source_variance', 'candle_backfill_gaps', 'candle_source_variance_1d', 'candle_backfill_gaps_1d', 'candle_source_variance_1h', 'candle_backfill_gaps_1h')

CREATE OR REPLACE VIEW candle_backfill_gaps AS
 SELECT q."time",
    q.symbol,
    q.frequency,
    q.close AS quote_close,
    now() - q."time" AS age
   FROM candles q
     LEFT JOIN candles h ON q."time" = h."time" AND q.symbol = h.symbol AND q.frequency = h.frequency AND h.source = 'ddowell-individual'::text
  WHERE q.source = 'quote'::text AND q.frequency = '5m'::text AND h."time" IS NULL AND q."time" < (now() - '04:00:00'::interval) AND COALESCE(q.volume, 0::bigint) > 0 AND (EXISTS ( SELECT 1
           FROM candles hd
          WHERE hd.frequency = '5m'::text AND hd.source = 'ddowell-individual'::text AND hd."time" >= (date_trunc('day'::text, (q."time" AT TIME ZONE 'America/Chicago'::text)) AT TIME ZONE 'America/Chicago'::text) AND hd."time" < ((date_trunc('day'::text, (q."time" AT TIME ZONE 'America/Chicago'::text)) + '1 day'::interval) AT TIME ZONE 'America/Chicago'::text)));

CREATE OR REPLACE VIEW candle_backfill_gaps_1d AS
 SELECT "time",
    symbol,
    frequency,
    close AS quote_close,
    now() - "time" AS age
   FROM candles q
  WHERE source = 'quote'::text AND frequency = '1d'::text AND "time" < (now() - '2 days'::interval) AND (EXISTS ( SELECT 1
           FROM candles hd
          WHERE hd.frequency = '1d'::text AND hd.source = 'ddowell-individual'::text AND (hd."time" AT TIME ZONE 'America/Chicago'::text)::date = (q."time" AT TIME ZONE 'UTC'::text)::date)) AND NOT (EXISTS ( SELECT 1
           FROM candles h
          WHERE h.symbol = q.symbol AND h.frequency = '1d'::text AND h.source = 'ddowell-individual'::text AND (h."time" AT TIME ZONE 'America/Chicago'::text)::date = (q."time" AT TIME ZONE 'UTC'::text)::date));

CREATE OR REPLACE VIEW candle_backfill_gaps_1h AS
 SELECT "time",
    symbol,
    frequency,
    close AS quote_close,
    now() - "time" AS age
   FROM candles q
  WHERE source = 'quote'::text AND frequency = '1h'::text AND COALESCE(volume, 0::bigint) > 0 AND "time" < (now() - '04:00:00'::interval) AND (EXISTS ( SELECT 1
           FROM candles hd
          WHERE hd.frequency = '30m'::text AND hd.source = 'ddowell-individual'::text AND hd."time" >= q."time" AND hd."time" < (q."time" + '01:00:00'::interval))) AND NOT (EXISTS ( SELECT 1
           FROM candles h
          WHERE h.symbol = q.symbol AND h.frequency = '30m'::text AND h.source = 'ddowell-individual'::text AND h."time" >= q."time" AND h."time" < (q."time" + '01:00:00'::interval)));

CREATE OR REPLACE VIEW candle_source_variance AS
 SELECT h."time",
    h.symbol,
    h.frequency,
    h.close AS hist_close,
    q.close AS quote_close,
    h.high AS hist_high,
    q.high AS quote_high,
    h.low AS hist_low,
    q.low AS quote_low,
        CASE
            WHEN h.close <> 0::double precision THEN abs(h.close - q.close) / abs(h.close)
            ELSE NULL::double precision
        END AS close_variance,
        CASE
            WHEN h.high <> 0::double precision THEN abs(h.high - q.high) / abs(h.high)
            ELSE NULL::double precision
        END AS high_variance,
        CASE
            WHEN h.low <> 0::double precision THEN abs(h.low - q.low) / abs(h.low)
            ELSE NULL::double precision
        END AS low_variance
   FROM candles h
     JOIN candles q ON h."time" = q."time" AND h.symbol = q.symbol AND h.frequency = q.frequency
  WHERE h.source = 'ddowell-individual'::text AND (q.source = ANY (ARRAY['quote'::text, 'quote-derived'::text]));

CREATE OR REPLACE VIEW candle_source_variance_1d AS
 SELECT h."time",
    h.symbol,
    h.frequency,
    h.close AS hist_close,
    q.close AS quote_close,
    h.high AS hist_high,
    q.high AS quote_high,
    h.low AS hist_low,
    q.low AS quote_low,
        CASE
            WHEN h.close <> 0::double precision THEN abs(h.close - q.close) / abs(h.close)
            ELSE NULL::double precision
        END AS close_variance,
        CASE
            WHEN h.high <> 0::double precision THEN abs(h.high - q.high) / abs(h.high)
            ELSE NULL::double precision
        END AS high_variance,
        CASE
            WHEN h.low <> 0::double precision THEN abs(h.low - q.low) / abs(h.low)
            ELSE NULL::double precision
        END AS low_variance
   FROM candles h
     JOIN candles q ON h.symbol = q.symbol AND h.frequency = q.frequency AND (h."time" AT TIME ZONE 'America/Chicago'::text)::date = (q."time" AT TIME ZONE 'UTC'::text)::date
  WHERE h.frequency = '1d'::text AND h.source = 'ddowell-individual'::text AND q.source = 'quote'::text;

CREATE OR REPLACE VIEW candle_source_variance_1h AS
 WITH h AS (
         SELECT candles.symbol,
            time_bucket('01:00:00'::interval, candles."time") AS hr,
            max(candles.high) AS high,
            min(candles.low) AS low,
            (array_agg(candles.close ORDER BY candles."time" DESC))[1] AS close
           FROM candles
          WHERE candles.frequency = '30m'::text AND candles.source = 'ddowell-individual'::text
          GROUP BY candles.symbol, (time_bucket('01:00:00'::interval, candles."time"))
         HAVING count(*) = 2
        )
 SELECT q."time",
    q.symbol,
    q.frequency,
    h.close AS hist_close,
    q.close AS quote_close,
    h.high AS hist_high,
    q.high AS quote_high,
    h.low AS hist_low,
    q.low AS quote_low,
        CASE
            WHEN h.close <> 0::double precision THEN abs(h.close - q.close) / abs(h.close)
            ELSE NULL::double precision
        END AS close_variance,
        CASE
            WHEN h.high <> 0::double precision THEN abs(h.high - q.high) / abs(h.high)
            ELSE NULL::double precision
        END AS high_variance,
        CASE
            WHEN h.low <> 0::double precision THEN abs(h.low - q.low) / abs(h.low)
            ELSE NULL::double precision
        END AS low_variance
   FROM h
     JOIN candles q ON q.symbol = h.symbol AND q."time" = h.hr
  WHERE q.frequency = '1h'::text AND q.source = 'quote'::text;

