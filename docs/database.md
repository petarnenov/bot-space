# Database Operations

All durable state resides in PostgreSQL. Each application process has a bounded
pool (`DB_MAX_CONNS`, default 10, permitted 1–1000). Account for every replica,
migrator, and administrator when budgeting database connections. New connections
have a two-second timeout; readiness shares a two-second acquisition/ping/schema
deadline.

## Migrations

```sh
go run ./cmd/mailbox migrate
# Using the configured local Compose environment:
docker compose run --rm migrate
```

The migration command uses a dedicated connection and PostgreSQL advisory lock
to serialize concurrent invocations. Each numbered SQL migration and its ledger
entry commit in one transaction. The ledger records version, SHA-256 checksum,
and application timestamp. Rerunning the command skips completed migrations.
The entire migration command has a five-minute deadline, including lock wait.

Serving HTTP does not apply migrations. Readiness refuses missing migrations,
changed checksums, migration gaps, and unknown newer versions. Do not edit applied
SQL. Add a forward migration and update the relevant OpenSpec specifications.
Schema checks are strict: old binaries are not promised to remain ready after
new migrations. Plan upgrades accordingly; zero downtime is not guaranteed.

Failed migration SQL rolls back that migration, while earlier committed versions
remain intact. Fix the forward migration before retrying. Do not automatically
roll back production schema or delete the database volume. Restore only from a
verified backup under a documented maintenance procedure.

## Backup and Restore Preparation

Before an authorized production deployment, confirm database backup retention,
access control, encryption, responsible operator, and a scheduled restore drill.
These are owner-supplied deployment settings, not resources provisioned here.

PostgreSQL logical backup/restore commands, intended for an isolated drill:

```sh
pg_dump --format=custom --file=mailbox.dump "$DATABASE_URL"
pg_restore --exit-on-error --no-owner --dbname="$RESTORE_DATABASE_URL" mailbox.dump
```

Use a separate empty database for `RESTORE_DATABASE_URL`; do not point it at the
live database. Keep dump files outside this public repository and out of logs.
Protect database URLs from shell history and process inspection in production.
Verify the restored migration ledger and application readiness, then verify
messages and credential state once those capabilities exist. A production backup
and restore has not yet been executed.
