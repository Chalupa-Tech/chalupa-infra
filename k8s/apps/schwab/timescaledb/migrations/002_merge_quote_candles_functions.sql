-- Baseline: the three merge_quote_candles* job functions and their add_job schedules.
-- The baseline check needs the same definitions and all three scheduled jobs; otherwise the file runs.
-- Function definitions are the live ones (pg_get_functiondef, 2026-11-24).
-- The add_job blocks run only on a fresh cluster; the live one is baselined without running them.
-- baseline-if: SELECT (SELECT md5(string_agg(pg_get_functiondef(p.oid), E'\n' ORDER BY p.proname)) FROM pg_proc p JOIN pg_namespace n ON n.oid = p.pronamespace WHERE n.nspname = 'public' AND p.proname IN ('merge_quote_candles', 'merge_quote_candles_1h', 'merge_quote_candles_1d')) = '228df82454f39fc28b34308d2ed5ff17' AND (SELECT count(*) FROM timescaledb_information.jobs WHERE proc_name IN ('merge_quote_candles', 'merge_quote_candles_1h', 'merge_quote_candles_1d')) = 3 AND (SELECT count(*) FROM timescaledb_information.jobs j JOIN (VALUES ('merge_quote_candles', INTERVAL '5 minutes'), ('merge_quote_candles_1h', INTERVAL '1 hour'), ('merge_quote_candles_1d', INTERVAL '1 day')) v(proc, every) ON j.proc_name = v.proc AND j.schedule_interval = v.every AND j.scheduled) = 3

CREATE OR REPLACE FUNCTION public.merge_quote_candles(job_id integer, config jsonb)
 RETURNS void
 LANGUAGE plpgsql
AS $function$
BEGIN
  INSERT INTO candles (time, symbol, frequency, open, high, low, close, volume, source)
  SELECT time, symbol, frequency, open, high, low, close, volume, source
  FROM candles_from_quotes
  WHERE time > now() - INTERVAL '1 day'
  ON CONFLICT (time, symbol, frequency, source) DO UPDATE SET
    open = EXCLUDED.open, high = EXCLUDED.high, low = EXCLUDED.low,
    close = EXCLUDED.close, volume = EXCLUDED.volume;
END;
$function$
;

CREATE OR REPLACE FUNCTION public.merge_quote_candles_1d(job_id integer, config jsonb)
 RETURNS void
 LANGUAGE plpgsql
AS $function$
BEGIN
  INSERT INTO candles (time, symbol, frequency, open, high, low, close, volume, source)
  SELECT time, symbol, frequency, open, high, low, close, volume, source
  FROM candles_1d
  WHERE time > now() - INTERVAL '7 days'
  ON CONFLICT (time, symbol, frequency, source) DO UPDATE SET
    open = EXCLUDED.open, high = EXCLUDED.high, low = EXCLUDED.low,
    close = EXCLUDED.close, volume = EXCLUDED.volume;
END;
$function$
;

CREATE OR REPLACE FUNCTION public.merge_quote_candles_1h(job_id integer, config jsonb)
 RETURNS void
 LANGUAGE plpgsql
AS $function$
BEGIN
  INSERT INTO candles (time, symbol, frequency, open, high, low, close, volume, source)
  SELECT time, symbol, frequency, open, high, low, close, volume, source
  FROM candles_1h
  WHERE time > now() - INTERVAL '3 days'
  ON CONFLICT (time, symbol, frequency, source) DO UPDATE SET
    open = EXCLUDED.open, high = EXCLUDED.high, low = EXCLUDED.low,
    close = EXCLUDED.close, volume = EXCLUDED.volume;
END;
$function$
;


-- add_job does not raise on a duplicate, so look at the job list first. An
-- existing job is put back to the expected interval and enabled; a second job
-- for one procedure is drift this file will not guess about, so it fails.
DO $$
DECLARE
  j record;
  n int;
BEGIN
  FOR j IN
    SELECT * FROM (VALUES
      ('merge_quote_candles',    INTERVAL '5 minutes'),
      ('merge_quote_candles_1h', INTERVAL '1 hour'),
      ('merge_quote_candles_1d', INTERVAL '1 day')
    ) AS t(proc, every)
  LOOP
    SELECT count(*) INTO n FROM timescaledb_information.jobs WHERE proc_name = j.proc;
    IF n = 0 THEN
      PERFORM add_job(j.proc::regproc, j.every);
    ELSIF n = 1 THEN
      PERFORM alter_job(job_id, schedule_interval => j.every, scheduled => true)
        FROM timescaledb_information.jobs WHERE proc_name = j.proc;
    ELSE
      RAISE EXCEPTION '% has % jobs; expected exactly one', j.proc, n;
    END IF;
  END LOOP;
END $$;
