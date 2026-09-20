# Operations runbook

## Database migrations

Migrations in `apps/aegis-api/migrations` are embedded into the binary and run
automatically in filename order during startup. Applied filenames are recorded
in `schema_migrations`. Never edit an applied migration; add a new numbered
file. Back up the SQLite database before upgrading.

## Key rotation

The current milestone encrypts each credential with AES-256-GCM and binds each
field to its record through authenticated additional data. Rotation tooling is
not yet implemented. Keep the current master key available for restoration and
re-enter credentials under a newly initialized deployment when rotation is
required. This is a documented MVP limitation, not an authorization to store
plain text secrets.

## Backup recovery

Backups reside below `BACKUP_STORAGE_PATH` with mode `0600`; metadata resides in
SQLite. Restore the database and backup tree together so paths and metadata
remain consistent. Configuration files are never served by the reverse proxy;
downloads pass through authenticated handlers.

## Health

`/healthz` reports process health. `/readyz` checks SQLite connectivity.
`/metrics` requires `system.manage` and exposes Prometheus text metrics.

