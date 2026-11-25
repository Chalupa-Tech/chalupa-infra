# TimescaleDB schema migrations

Schema changes to the `market` database reach the live cluster through an ArgoCD PostSync Job, not `kubectl exec`.
The CloudNativePG bootstrap SQL (`configmap-init-sql.yaml`) runs once, when the cluster is first created, so it cannot carry a change to an existing database.

## How it works

- Migrations are numbered files in `k8s/apps/schwab/timescaledb/migrations/`, named `NNN_snake_case.sql`.
- Helm puts them in the `timescaledb-migrations` ConfigMap; the `timescaledb-migrate` Job (`templates/migrate-job.yaml`, hook `PostSync`) mounts it and runs `files/migrate.sh`.
- The script runs one `psql` session that takes `pg_advisory_lock`, so two syncs cannot interleave.
- For each file in filename order it looks the name up in the `schema_migrations` table (`name`, `checksum`, `applied_at`, `baseline`).
  - Not recorded: run the file and insert its row in one transaction. A failed file records nothing.
  - Recorded with the same checksum: skip.
  - Recorded with a different checksum: fail the Job. Applied files are never edited.
- The Job connects as `postgres` with the `timescaledb-superuser` Secret that CloudNativePG generates (`enableSuperuserAccess: true`).
  The views, functions and continuous aggregates are owned by `postgres`, so a lesser role could not replace them.
  The credential lives only in Kubernetes: it is deliberately not in OpenBao, because the namespace's shared `default` ServiceAccount can read everything under `secret/data/schwab-ddowell/*`.
- Before running anything the Job waits up to 120 s for the primary to accept that login, because CloudNativePG sets the password shortly after the database reports ready.
- `schema_migrations` has row-level security on and no policy, so the application roles (which hold `pg_write_all_data`) can neither read nor forge ledger rows.

## Add a migration

1. Add `migrations/NNN_name.sql` with the next number. Do not edit an existing file.
2. Write forward-only SQL. It runs inside a transaction, so no `BEGIN`/`COMMIT`, and nothing that cannot run in a transaction (for example creating a continuous aggregate `WITH DATA`).
3. Make it safe to read: the Job takes locks on what it changes, and on this single-instance cluster that is a brief stall, not a failover.
4. Merge. ArgoCD syncs, the Job applies the file, and `SELECT * FROM schema_migrations` lists it.

If a migration has a first line `-- baseline-if: <one-line SQL returning boolean>`, the Job records the file as applied without running it when that query is true while the file is still unrecorded.
Migrations 001 and 002 use it with a fingerprint of the live definitions (and, for 002, the three scheduled jobs), so a cluster whose objects differ runs the file instead of recording it.
The live cluster matches, so it is baselined; a fresh cluster runs them.

## Read the result

A clean run leaves no Job behind (`HookSucceeded` deletes it), so read the table:

```bash
kubectl exec -n schwab-ddowell timescaledb-1 -c postgres -- psql -U postgres -d market -c "SELECT name, left(checksum, 12), applied_at, baseline FROM schema_migrations ORDER BY name"
```

A failed Job stays for its logs until the next sync: `kubectl logs -n schwab-ddowell job/timescaledb-migrate`.
The ArgoCD Application shows the sync as failed.

## Failure modes

- **Checksum mismatch** (`was edited after it was applied`): revert the edit and add a new numbered file instead.
- **`primary not reachable with these credentials`**: the `timescaledb-superuser` Secret is missing or `enableSuperuserAccess` is off. Check `kubectl get secret -n schwab-ddowell timescaledb-superuser`.
- **A migration fails**: nothing is recorded for it and the Job fails. Fix the file (it was never applied, so editing it is safe) or add a corrective migration, and merge.
- **A removed file**: the Job does not notice. Recorded rows stay; delete a row by hand only to re-run a file you know did not take effect.
- **No rollback**: migrations are forward-only. Undo with a new migration.

## What stays in the bootstrap SQL

Tables, hypertables, continuous aggregates, retention and refresh policies, and grants stay in `configmap-init-sql.yaml` and run only at cluster creation.
The `candle_*` views (migration 001) and the `merge_quote_candles*` functions with their `add_job` schedules (migration 002) moved out, so a fresh cluster gets them from the Job after the cluster is healthy.
