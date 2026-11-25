# TimescaleDB schema migrations

Schema changes to the `market` database reach the live cluster through an ArgoCD PostSync Job, not `kubectl exec`.
The CloudNativePG bootstrap SQL (`configmap-init-sql.yaml`) runs once, when the cluster is first created, so it cannot carry a change to an existing database.

## How it works

- Migrations are numbered files in `k8s/apps/schwab/timescaledb/migrations/`, named `NNN_snake_case.sql`.
- Helm puts them in the `timescaledb-migrations` ConfigMap; the `timescaledb-migrate` Job (`templates/migrate-job.yaml`, hook `PostSync`) mounts it and runs `files/migrate.sh`.
- The script runs one `psql` session that takes `pg_advisory_lock`, so two syncs cannot interleave.
- For each file in filename order it looks the name up in the `migrations.schema_migrations` table (`name`, `checksum`, `applied_at`, `baseline`).
  - Not recorded: run the file and insert its row in one transaction. A failed file records nothing.
  - Recorded with the same checksum: skip.
  - Recorded with a different checksum: fail the Job. Applied files are never edited.
- The Job connects as `postgres` with the `timescaledb-superuser` Secret that CloudNativePG generates (`enableSuperuserAccess: true`).
  A fresh cluster gets that password at initdb. On a cluster that already existed when the flag was turned on, CloudNativePG creates the Secret but leaves the role without a password: see "Existing cluster: one-time password" below.
  The views, functions and continuous aggregates are owned by `postgres`, so a lesser role could not replace them.
  The credential lives only in Kubernetes: it is deliberately not in OpenBao, because the namespace's shared `default` ServiceAccount can read everything under `secret/data/schwab-ddowell/*`.
- Before running anything the Job waits up to 120 s for the primary to accept that login, because CloudNativePG sets the password shortly after the database reports ready.
- The ledger sits in its own schema `migrations`, created by the Job in the same transaction that turns on row-level security (no policy), so the application roles (which hold `pg_write_all_data` and can create in `public`) can neither read nor forge ledger rows. The Job refuses a ledger it does not own or that has triggers.
- The script copies the mounted files once and hashes and runs that copy, so a ConfigMap refresh during a run cannot put one file's checksum on another's SQL.

## Add a migration

1. Add `migrations/NNN_name.sql` with the next number. Do not edit an existing file.
2. Write forward-only SQL. It runs inside a transaction, so no `BEGIN`/`COMMIT`, and nothing that cannot run in a transaction (for example creating a continuous aggregate `WITH DATA`).
3. Make it safe to read: the Job takes locks on what it changes, and on this single-instance cluster that is a brief stall, not a failover.
4. Merge. ArgoCD syncs, the Job applies the file, and `SELECT * FROM migrations.schema_migrations` lists it.

If a migration has a first line `-- baseline-if: <one-line SQL returning boolean>`, the Job records the file as applied without running it when that query is true while the file is still unrecorded.
Migrations 001 and 002 use it with a fingerprint of the live definitions (and, for 002, the three scheduled jobs at their intervals, enabled), so a cluster whose objects differ runs the file instead of recording it.
The live cluster matches, so it is baselined; a fresh cluster runs them.

## Read the result

A clean run leaves no Job behind (`HookSucceeded` deletes it), so read the table:

```bash
kubectl exec -n schwab-ddowell timescaledb-1 -c postgres -- psql -U postgres -d market -c "SELECT name, left(checksum, 12), applied_at, baseline FROM migrations.schema_migrations ORDER BY name"
```

A failed Job stays for its logs until the next sync: `kubectl logs -n schwab-ddowell job/timescaledb-migrate`.
The ArgoCD Application shows the sync as failed.

## Failure modes

- **Checksum mismatch** (`was edited after it was applied`): revert the edit and add a new numbered file instead.
- **`primary not reachable with these credentials`**: the `timescaledb-superuser` Secret is missing or `enableSuperuserAccess` is off. Check `kubectl get secret -n schwab-ddowell timescaledb-superuser`.
- **A migration fails**: nothing is recorded for it and the Job fails. Fix the file (it was never applied, so editing it is safe) or add a corrective migration, and merge.
- **A removed file**: the Job does not notice. Recorded rows stay; delete a row by hand only to re-run a file you know did not take effect.
- **No rollback**: migrations are forward-only. Undo with a new migration.

## Existing cluster: one-time password

Turning `enableSuperuserAccess` on for a cluster that already exists creates the `timescaledb-superuser` Secret but does not set the password inside Postgres, so the Job fails with `password authentication failed for user "postgres"`.
Check with `select rolpassword is not null from pg_authid where rolname = 'postgres'` (false means it is not set).
Set it once from the Secret, without printing it, then start a new sync (a failed sync does not retry by itself):

```bash
PW="$(kubectl get secret -n schwab-ddowell timescaledb-superuser -o jsonpath='{.data.password}' | base64 -d)"
printf "SET log_statement = 'none';\nALTER ROLE postgres PASSWORD :'pw';\n" | kubectl exec -i -n schwab-ddowell timescaledb-1 -c postgres -- psql -U postgres -d market -v ON_ERROR_STOP=1 -v pw="$PW"
unset PW
kubectl -n argocd patch application schwab-ddowell-timescaledb --type merge -p '{"operation":{"sync":{}}}'
```

This was done on the live cluster on 2026-11-25. Rotating the Secret later needs the same `ALTER ROLE`.

## What stays in the bootstrap SQL

Tables, hypertables, continuous aggregates, retention and refresh policies, and grants stay in `configmap-init-sql.yaml` and run only at cluster creation.
The `candle_*` views (migration 001) and the `merge_quote_candles*` functions with their `add_job` schedules (migration 002) moved out, so a fresh cluster gets them from the Job after the cluster is healthy.
