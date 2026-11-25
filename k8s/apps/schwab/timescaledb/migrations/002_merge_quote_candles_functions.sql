-- Baseline: the three merge_quote_candles* job functions and their add_job schedules.
-- The baseline check needs the same definitions and all three scheduled jobs; otherwise the file runs.
-- Function definitions are the live ones (pg_get_functiondef, 2026-11-24).
-- The add_job blocks run only on a fresh cluster; the live one is baselined without running them.
-- baseline-if: SELECT (SELECT md5(string_agg(pg_get_functiondef(p.oid), E'\n' ORDER BY p.proname)) FROM pg_proc p JOIN pg_namespace n ON n.oid = p.pronamespace WHERE n.nspname = 'public' AND p.proname IN ('merge_quote_candles', 'merge_quote_candles_1h', 'merge_quote_candles_1d')) = '228df82454f39fc28b34308d2ed5ff17' AND (SELECT count(*) FROM (VALUES ('merge_quote_candles', INTERVAL '5 minutes'), ('merge_quote_candles_1h', INTERVAL '1 hour'), ('merge_quote_candles_1d', INTERVAL '1 day')) v(proc, every) WHERE (SELECT count(*) FROM timescaledb_information.jobs j WHERE j.proc_name = v.proc) = 1 AND (SELECT count(*) FROM timescaledb_information.jobs j WHERE j.proc_name = v.proc AND j.proc_schema = 'public' AND j.schedule_interval = v.every AND j.scheduled) = 1) = 3

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
-- existing job is put back to the expected interval and enabled; a job for the
-- same procedure in another schema, or a second one, is drift this file will
-- not guess about, so it fails. Calls are schema-qualified with every argument
-- typed, so an overload in a schema the applications can write cannot match.
DO $$
DECLARE
  j record;
  n_public int;
  n_other int;
  id int;
BEGIN
  FOR j IN
    SELECT * FROM (VALUES
      ('merge_quote_candles',    INTERVAL '5 minutes'),
      ('merge_quote_candles_1h', INTERVAL '1 hour'),
      ('merge_quote_candles_1d', INTERVAL '1 day')
    ) AS t(proc, every)
  LOOP
    SELECT count(*) FILTER (WHERE proc_schema = 'public'),
           count(*) FILTER (WHERE proc_schema <> 'public')
      INTO n_public, n_other
      FROM timescaledb_information.jobs WHERE proc_name = j.proc;
    IF n_other > 0 THEN
      RAISE EXCEPTION '% has a job outside schema public', j.proc;
    ELSIF n_public = 0 THEN
      PERFORM public.add_job(('public.' || j.proc)::regproc, j.every::interval, NULL::jsonb,
                             NULL::timestamptz, true, NULL::regproc, true, NULL::text);
    ELSIF n_public = 1 THEN
      SELECT job_id INTO id FROM timescaledb_information.jobs
        WHERE proc_name = j.proc AND proc_schema = 'public';
      PERFORM public.alter_job(id, j.every::interval, NULL::interval, NULL::integer, NULL::interval,
                               true, NULL::jsonb, NULL::timestamptz, false, NULL::regproc,
                               NULL::boolean, NULL::timestamptz, NULL::text);
    ELSE
      RAISE EXCEPTION '% has % jobs; expected exactly one', j.proc, n_public;
    END IF;
  END LOOP;
  IF (SELECT count(*)
        FROM (VALUES ('merge_quote_candles', INTERVAL '5 minutes'),
                     ('merge_quote_candles_1h', INTERVAL '1 hour'),
                     ('merge_quote_candles_1d', INTERVAL '1 day')) v(proc, every)
        JOIN timescaledb_information.jobs t
          ON t.proc_name = v.proc AND t.proc_schema = 'public'
         AND t.schedule_interval = v.every AND t.scheduled) <> 3 THEN
    RAISE EXCEPTION 'merge_quote_candles jobs are not all scheduled at their intervals';
  END IF;
END $$;
