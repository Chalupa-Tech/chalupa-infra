#!/usr/bin/env bash
# Offline checks for the timescaledb migration Job: the chart renders, the
# rendered ConfigMaps carry the migration files unchanged, and the script's
# generated psql script has the expected shape and rejects bad input.
# Needs helm and python3 with PyYAML; no cluster or Postgres.
set -euo pipefail

chart="$(cd "$(dirname "$0")/../.." && pwd)/k8s/apps/schwab/timescaledb"
work="$(mktemp -d "${TMPDIR:-/tmp}/tsdb-migrations.XXXXXX")"
trap 'rm -rf "$work"' EXIT
fail() { echo "FAIL: $*" >&2; exit 1; }

helm lint "$chart" >/dev/null || fail "helm lint"
helm template t "$chart" >"$work/render.yaml"

CHART="$chart" RENDER="$work/render.yaml" python3 -I - <<'PY' || fail "rendered chart"
import glob, os, yaml
chart, render = os.environ["CHART"], os.environ["RENDER"]
docs = [d for d in yaml.safe_load_all(open(render)) if d]
cms = {d["metadata"]["name"]: d for d in docs if d["kind"] == "ConfigMap"}
files = sorted(glob.glob(os.path.join(chart, "migrations", "*.sql")))
assert files, "no migrations"
mig = cms["timescaledb-migrations"]["data"]
assert sorted(mig) == [os.path.basename(f) for f in files], "migration set differs"
for f in files:
    assert mig[os.path.basename(f)] == open(f).read(), f"{f} altered by Helm"
script = cms["timescaledb-migrate-script"]["data"]["migrate.sh"]
assert script == open(os.path.join(chart, "files", "migrate.sh")).read(), "script altered"
job = next(d for d in docs if d["kind"] == "Job")
ann = job["metadata"]["annotations"]
assert ann["argocd.argoproj.io/hook"] == "PostSync"
assert "ttlSecondsAfterFinished" not in job["spec"], "failed Jobs must be kept"
assert "HookSucceeded" in ann["argocd.argoproj.io/hook-delete-policy"]
c = job["spec"]["template"]["spec"]["containers"][0]
assert c["resources"]["limits"]["memory"], "no memory limit"
cluster = next(d for d in docs if d["kind"] == "Cluster")
assert cluster["spec"]["enableSuperuserAccess"] is True
assert all(r["name"] != "migrator" for r in cluster["spec"]["managed"]["roles"])
assert not any(d["kind"] == "ExternalSecret" and "migrator" in d["metadata"]["name"] for d in docs), "superuser cred must not go through OpenBao"
env = {e["name"]: e for e in c["env"]}
assert env["PGUSER"]["value"] == "postgres"
assert env["PGPASSWORD"]["valueFrom"]["secretKeyRef"]["name"] == "timescaledb-superuser"
init = cms["timescaledb-init-sql"]["data"]["schema.sql"]
assert "CREATE OR REPLACE VIEW candle_" not in init, "views still in bootstrap SQL"
assert "FUNCTION merge_quote_candles" not in init, "functions still in bootstrap SQL"
PY

gen() { DRY_RUN=1 MIGRATIONS_DIR="$1" bash "$chart/files/migrate.sh"; }

out="$(gen "$chart/migrations")"
for want in 'pg_advisory_lock' 'CREATE TABLE IF NOT EXISTS migrations.schema_migrations' '\if :drifted' \
  'ENABLE ROW LEVEL SECURITY' 'SET search_path = pg_catalog, pg_temp' 'SET LOCAL search_path = public, pg_temp' "'001_candle_reconciliation_views.sql'" "'002_merge_quote_candles_functions.sql'" '\i '; do
  grep -qF -- "$want" <<<"$out" || fail "generated script lacks: $want"
done
[ "$(grep -c '^\\if :pending' <<<"$out")" -eq "$(ls "$chart"/migrations/*.sql | wc -l)" ] || fail "pending guard per file"
# A baseline-if header is ONE line of complete SQL: quotes and parentheses
# balance (a stray newline in an E'\\n' literal cuts the query short).
CHART="$chart" python3 -I - <<'PY' || fail "baseline-if header"
import glob, os
for f in sorted(glob.glob(os.path.join(os.environ["CHART"], "migrations", "*.sql"))):
    for line in open(f):
        if line.startswith("-- baseline-if: "):
            q = line[len("-- baseline-if: "):].rstrip("\n")
            assert q.count("'") % 2 == 0, f"{f}: unbalanced quotes"
            assert q.count("(") == q.count(")"), f"{f}: unbalanced parentheses"
            assert q.upper().startswith("SELECT"), f"{f}: not a SELECT"
PY
# The lock must be taken before any migration statement.
[ "$(grep -n 'pg_advisory_lock' <<<"$out" | head -n1 | cut -d: -f1)" -lt "$(grep -n '\\i ' <<<"$out" | head -n1 | cut -d: -f1)" ] || fail "lock after first \\i"

# The checksum in the script is the file's sha256, so an edit changes it.
sum="$(sha256sum "$chart/migrations/001_candle_reconciliation_views.sql" | cut -d' ' -f1)"
grep -qF "$sum" <<<"$out" || fail "checksum not in script"
cp -R "$chart/migrations" "$work/m"
echo "-- edited" >>"$work/m/001_candle_reconciliation_views.sql"
edited="$(gen "$work/m")" || fail "generator failed on the edited migration"
grep -qF "'001_candle_reconciliation_views.sql'" <<<"$edited" || fail "edited script lacks the migration"
if grep -qF "$sum" <<<"$edited"; then fail "edit did not change the checksum"; fi
# The ledger is a schema of its own, created with RLS in one transaction.
grep -qF 'CREATE SCHEMA IF NOT EXISTS migrations' <<<"$out" || fail "ledger schema"
if grep -qE '(FROM|INTO) schema_migrations' <<<"$out"; then fail "unqualified ledger reference"; fi

# Bad input is rejected before psql is ever reached.
mkdir "$work/dup" "$work/bad" "$work/empty" "$work/broken"
touch "$work/dup/001_a.sql" "$work/dup/001_b.sql" "$work/bad/Bad.sql" "$work/broken/002_ok.sql"
ln -s "$work/does-not-exist.sql" "$work/broken/001_gone.sql"   # unreadable: must not be skipped
for d in dup bad empty broken; do
  if gen "$work/$d" >/dev/null 2>&1; then fail "accepted $d"; fi
done

echo "ok"
