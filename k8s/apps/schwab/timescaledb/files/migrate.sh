#!/usr/bin/env bash
# Applies the numbered SQL files in $MIGRATIONS_DIR once each and records them
# in schema_migrations. Runs as the timescaledb-migrate ArgoCD PostSync Job.
#
# The whole run is ONE psql session: it takes pg_advisory_lock first, so two
# syncs cannot interleave, and every file is applied and recorded in a single
# transaction, so a failed file leaves nothing recorded.
#
#   migrations/NNN_name.sql   NNN = 3 digits, applied in filename order
#   -- baseline-if: <SQL>     optional header; if this one-line query returns
#                             true when the file is still unapplied, the file is
#                             recorded as applied WITHOUT running (the objects
#                             already exist live)
#
# A recorded file whose checksum no longer matches fails the run. Migrations
# are forward-only: change something by adding a new file, never by editing one.
# Files must not contain BEGIN/COMMIT; they run inside the transaction above.
#
# DRY_RUN=1 prints the generated psql script instead of running it.
set -euo pipefail

DIR="${MIGRATIONS_DIR:-/migrations}"
LOCK_KEY="${LOCK_KEY:-727015}"
LOCK_TIMEOUT="${LOCK_TIMEOUT:-30s}"

shopt -s nullglob
files=("$DIR"/*.sql)
if [ "${#files[@]}" -eq 0 ]; then
  echo "no migrations found in $DIR" >&2
  exit 1
fi

script="$(mktemp "${TMPDIR:-/tmp}/migrate.XXXXXX")"
trap 'rm -f "$script"' EXIT

prev=""
cat >"$script" <<EOF
\\set ON_ERROR_STOP on
SELECT pg_advisory_lock($LOCK_KEY);
SET lock_timeout = '$LOCK_TIMEOUT';
CREATE TABLE IF NOT EXISTS schema_migrations (
  name       text PRIMARY KEY,
  checksum   text NOT NULL,
  applied_at timestamptz NOT NULL DEFAULT now(),
  baseline   boolean NOT NULL DEFAULT false
);
-- The application roles hold pg_write_all_data, which ignores table grants but
-- not row-level security. With RLS on and no policy they can neither read nor
-- forge ledger rows; superusers (this Job) bypass it.
ALTER TABLE schema_migrations ENABLE ROW LEVEL SECURITY;
EOF

for f in "${files[@]}"; do
  name="$(basename "$f")"
  if ! [[ "$name" =~ ^[0-9]{3}_[a-z0-9_]+\.sql$ ]]; then
    echo "bad migration name: $name (want NNN_snake_case.sql)" >&2
    exit 1
  fi
  if [ "${name:0:3}" = "${prev:0:3}" ]; then
    echo "duplicate migration number: $prev and $name" >&2
    exit 1
  fi
  prev="$name"
  sum="$(sha256sum "$f" | cut -d' ' -f1)"
  baseline_sql="$(sed -n 's/^-- baseline-if: //p' "$f" | head -n 1)"

  cat >>"$script" <<EOF
\\echo == $name
SELECT coalesce(max(checksum), '') AS applied FROM schema_migrations WHERE name = '$name' \\gset
SELECT :'applied' = '' AS pending, :'applied' NOT IN ('', '$sum') AS drifted \\gset
\\if :drifted
  DO \$\$ BEGIN RAISE EXCEPTION 'migration $name was edited after it was applied (recorded checksum differs from $sum)'; END \$\$;
\\endif
\\if :pending
EOF
  if [ -n "$baseline_sql" ]; then
    cat >>"$script" <<EOF
  SELECT coalesce(($baseline_sql), false) AS baseline \\gset
EOF
  else
    echo "  SELECT false AS baseline \\gset" >>"$script"
  fi
  cat >>"$script" <<EOF
  \\if :baseline
    INSERT INTO schema_migrations (name, checksum, baseline) VALUES ('$name', '$sum', true);
    \\echo baselined, not run
  \\else
    BEGIN;
    \\i $f
    INSERT INTO schema_migrations (name, checksum) VALUES ('$name', '$sum');
    COMMIT;
    \\echo applied
  \\endif
\\else
  \\echo already applied, skipped
\\endif
EOF
done

if [ "${DRY_RUN:-}" = "1" ]; then
  cat "$script"
  exit 0
fi

# PGHOST, PGUSER, PGPASSWORD, PGDATABASE and PGSSLMODE come from the Job env.
# Wait (bounded) until the writable primary accepts this login: after a cluster
# is created or the superuser secret is first enabled, CloudNativePG sets the
# password a little after the database reports ready. Only connection and
# authentication failures retry; SQL and checksum failures below fail at once.
deadline=$((SECONDS + ${CONNECT_WAIT:-120}))
until [ "$(psql --no-psqlrc -Atc 'SELECT NOT pg_is_in_recovery()' 2>/dev/null || true)" = "t" ]; do
  if [ "$SECONDS" -ge "$deadline" ]; then
    echo "primary not reachable with these credentials after ${CONNECT_WAIT:-120}s" >&2
    psql --no-psqlrc -Atc 'SELECT 1' >&2 || true
    exit 1
  fi
  echo "waiting for the primary to accept the login..."
  sleep 5
done
psql --no-psqlrc --quiet --file "$script"
echo "migrations complete"
