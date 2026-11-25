#!/usr/bin/env bash
# Runs the migration Job's script against a real TimescaleDB, the way a fresh
# cluster sees it: the chart's bootstrap SQL first, then the migrations.
# Needs psql, helm, python3 with PyYAML, and a superuser connection to an empty
# server in PGHOST/PGUSER/PGPASSWORD (CI supplies a service container).
set -euo pipefail

chart="$(cd "$(dirname "$0")/../.." && pwd)/k8s/apps/schwab/timescaledb"
work="$(mktemp -d "${TMPDIR:-/tmp}/tsdb-db-test.XXXXXX")"
fail() { echo "FAIL: $*" >&2; exit 1; }
# A uniquely named scratch database: this script never touches `market` or any
# other existing database, even when pointed at a real server by mistake.
db="migtest_$$_$(date +%s)"
sql() { psql --no-psqlrc -v ON_ERROR_STOP=1 -Atq -d "$db" -c "$1"; }
migrate() { MIGRATIONS_DIR="${1:-$chart/migrations}" PGDATABASE="$db" CONNECT_WAIT=30 bash "$chart/files/migrate.sh"; }
psql --no-psqlrc -v ON_ERROR_STOP=1 -q -d postgres -c "CREATE DATABASE $db TEMPLATE template0"   # template1 may already carry the extension
roles_made=0
cleanup() {
  psql --no-psqlrc -q -d postgres -c "DROP DATABASE IF EXISTS $db WITH (FORCE)" >/dev/null 2>&1 || true
  if [ "$roles_made" = 1 ]; then
    psql --no-psqlrc -q -d postgres -c "DROP ROLE IF EXISTS marketstore, grafana, papertrader" >/dev/null 2>&1 || true
  fi
  rm -rf "$work"
}
trap cleanup EXIT

# The extension version the chart bootstraps (postInitTemplateSQL), installed and asserted.
ext="$(helm template t "$chart" | sed -n "s/.*CREATE EXTENSION IF NOT EXISTS timescaledb VERSION '\([0-9.]*\)'.*/\1/p" | head -n 1)"
[ -n "$ext" ] || fail "no bootstrap extension version found in the chart"
sql "CREATE EXTENSION timescaledb VERSION '$ext'"
[ "$(sql "SELECT extversion FROM pg_extension WHERE extname = 'timescaledb'")" = "$ext" ] || fail "extension is not $ext"
# Roles are cluster-wide: refuse a server that already has them, and drop only the ones this run made.
[ "$(psql --no-psqlrc -Atq -d postgres -c "SELECT count(*) FROM pg_roles WHERE rolname IN ('marketstore', 'grafana', 'papertrader')")" = 0 ] \
  || fail "this server already has marketstore/grafana/papertrader roles; use a disposable instance"
roles_made=1
sql "CREATE ROLE marketstore LOGIN; CREATE ROLE grafana LOGIN; CREATE ROLE papertrader LOGIN;
     GRANT pg_read_all_data, pg_write_all_data TO marketstore; GRANT pg_read_all_data TO grafana;
     GRANT CREATE, USAGE ON SCHEMA public TO papertrader"

# Bootstrap SQL exactly as CloudNativePG would run it at initdb.
helm template t "$chart" >"$work/render.yaml"
RENDER="$work/render.yaml" OUT="$work/schema.sql" python3 -I - <<'PY'
import os, yaml
for d in yaml.safe_load_all(open(os.environ["RENDER"])):
    if d and d["kind"] == "ConfigMap" and d["metadata"]["name"] == "timescaledb-init-sql":
        open(os.environ["OUT"], "w").write(d["data"]["schema.sql"])
PY
psql --no-psqlrc -v ON_ERROR_STOP=1 -q -d "$db" -f "$work/schema.sql" >/dev/null

echo "== fresh cluster: both migrations run"
out="$(migrate)" || fail "first run: $out"
[ "$(grep -c '^applied$' <<<"$out")" -eq 2 ] || fail "expected two applied: $out"
[ "$(sql "SELECT count(*) FROM pg_views WHERE schemaname='public' AND viewname LIKE 'candle\_%'")" = 6 ] || fail "six views"
[ "$(sql "SELECT count(*) FROM timescaledb_information.jobs WHERE proc_name LIKE 'merge\_quote\_candles%' AND scheduled")" = 3 ] || fail "three scheduled jobs"
[ "$(sql "SELECT count(*) FROM migrations.schema_migrations WHERE NOT baseline")" = 2 ] || fail "ledger rows"

echo "== second run skips"
out="$(migrate)" || fail "second run: $out"
[ "$(grep -c 'already applied' <<<"$out")" -eq 2 ] || fail "expected two skips: $out"
[ "$(sql "SELECT count(*) FROM timescaledb_information.jobs WHERE proc_name LIKE 'merge\_quote\_candles%'")" = 3 ] || fail "jobs duplicated"

echo "== an edited applied file fails the run"
cp -R "$chart/migrations" "$work/edited"
echo "-- edited" >>"$work/edited/001_candle_reconciliation_views.sql"
if out="$(migrate "$work/edited" 2>&1)"; then fail "edited migration was accepted"; fi
grep -q "edited after it was applied" <<<"$out" || fail "wrong failure: $out"

echo "== a ledger lost with the objects still live is baselined, not re-run"
sql "DROP SCHEMA migrations CASCADE"
out="$(migrate)" || fail "baseline run: $out"
[ "$(grep -c 'baselined, not run' <<<"$out")" -eq 2 ] || fail "expected two baselines: $out"
[ "$(sql "SELECT count(*) FROM migrations.schema_migrations WHERE baseline")" = 2 ] || fail "baseline rows"

echo "== a drifted view is repaired by running the file instead of baselining it; a missing job is added"
sql "DROP SCHEMA migrations CASCADE"
sql "CREATE OR REPLACE VIEW candle_backfill_gaps AS SELECT time, symbol, frequency, close AS quote_close, now() - time AS age FROM candles WHERE false"
sql "SELECT delete_job(job_id) FROM timescaledb_information.jobs WHERE proc_name = 'merge_quote_candles_1d'" >/dev/null
out="$(migrate)" || fail "repair run: $out"
[ "$(grep -c '^applied$' <<<"$out")" -eq 2 ] || fail "drift should have run both files: $out"
[ "$(sql "SELECT count(*) FROM timescaledb_information.jobs WHERE proc_name LIKE 'merge\_quote\_candles%' AND scheduled")" = 3 ] || fail "job not restored"

echo "== a changed job is never altered: the migration fails and leaves it as found"
sql "DROP SCHEMA migrations CASCADE"
sql "SELECT alter_job(job_id, scheduled => false) FROM timescaledb_information.jobs WHERE proc_name = 'merge_quote_candles_1h'" >/dev/null
if out="$(migrate 2>&1)"; then fail "a disabled job was accepted: $out"; fi
grep -q "not exactly as expected" <<<"$out" || fail "wrong failure: $out"
[ "$(sql "SELECT scheduled FROM timescaledb_information.jobs WHERE proc_name = 'merge_quote_candles_1h'")" = f ] || fail "job was altered"
[ "$(sql "SELECT count(*) FROM migrations.schema_migrations")" = 1 ] || fail "failed 002 must not be recorded (001 may be)"
sql "SELECT alter_job(job_id, scheduled => true) FROM timescaledb_information.jobs WHERE proc_name = 'merge_quote_candles_1h'" >/dev/null
out="$(migrate)" || fail "run after fixing the job: $out"

echo "== renaming an applied migration is refused, not re-run"
cp -R "$chart/migrations" "$work/renamed"
mv "$work/renamed/001_candle_reconciliation_views.sql" "$work/renamed/001_views_renamed.sql"
if out="$(migrate "$work/renamed" 2>&1)"; then fail "a renamed migration was accepted: $out"; fi
grep -q "cannot be renamed" <<<"$out" || fail "wrong failure for rename: $out"

echo "== a job paused with next_start = infinity is not accepted either"
sql "DROP SCHEMA migrations CASCADE"
sql "SELECT alter_job(job_id, next_start => 'infinity') FROM timescaledb_information.jobs WHERE proc_name = 'merge_quote_candles'" >/dev/null
if out="$(migrate 2>&1)"; then fail "a paused job was accepted: $out"; fi
grep -q "not exactly as expected" <<<"$out" || fail "wrong failure for paused job: $out"
sql "SELECT alter_job(job_id, next_start => now()) FROM timescaledb_information.jobs WHERE proc_name = 'merge_quote_candles'" >/dev/null
out="$(migrate)" || fail "run after unpausing: $out"

echo "== application roles cannot touch the ledger"
for stmt in "INSERT INTO migrations.schema_migrations (name, checksum) VALUES ('x', 'y')" \
            "UPDATE migrations.schema_migrations SET checksum = 'z'" \
            "DELETE FROM migrations.schema_migrations"; do
  for role in marketstore papertrader; do
    n="$(psql --no-psqlrc -Atq -d "$db" -c "SET ROLE $role" -c "$stmt RETURNING 1" 2>&1 | grep -c '^1$' || true)"
    [ "$n" = 0 ] || fail "$role could run: $stmt"
  done
done
[ "$(sql "SELECT count(*) FROM migrations.schema_migrations")" = 2 ] || fail "ledger changed"

echo "ok"
