# Aegis API

## Local evaluation

After running the repository's local setup and start scripts, open
`http://127.0.0.1:8080/test-console/`. Sign in as `aegis-admin`; the generated
password is stored in the uncommitted `.env` file as
`BOOTSTRAP_ADMIN_PASSWORD`.

The evaluation console is enabled by default only in development. Production
startup fails if `LOCAL_TEST_CONSOLE_ENABLED=true`.

Aegis API is the first production application in the Aegis monorepo. It
provides authenticated, permission-controlled read access to approved Cisco
IOS/IOS XE devices. Device protocols and credentials remain behind the API.

## Implemented milestone

- Local JWT authentication with seeded Viewer, Network Operator, Network
  Administrator, and System Administrator permission sets.
- Optional per-user device-group restrictions in the data model.
- SQLite migrations and repository interfaces suitable for a later PostgreSQL
  implementation.
- AES-256-GCM encrypted password, private-key, and enable-secret fields.
- Cisco IOS/IOS XE SSH connector with fixed operations, strict OpenSSH
  known-host verification, timeouts, output limits, ANSI stripping, prompt and
  CLI-error detection, password/key authentication, and optional enable mode.
- Running/startup configurations, facts, interface status, downloads, caching,
  single-flight suppression, and global/per-device connection limits.
- Manual backup, SHA-256 deduplication, history, authenticated downloads, and
  unified diff.
- Structured logging, audit records, request IDs, CORS allowlisting, rate
  limiting, health/readiness, protected metrics, and embedded Swagger UI.

There is no Telnet client, generic CLI command endpoint, configuration-write
operation, or unregistered destination endpoint.

## Local setup

Requirements: Go 1.26.11 or a compatible newer Go 1.26 release.

1. Copy `.env.example` values into your shell or a local uncommitted `.env`.
   The application intentionally does not auto-load `.env` in production.
2. Generate independent keys:

   ```bash
   openssl rand -base64 32
   openssl rand -base64 32
   ```

3. Set `JWT_SIGNING_KEY`, `CREDENTIAL_MASTER_KEY`, and a one-time local
   bootstrap account. The password must contain at least 12 characters.
4. Create `data/known_hosts` using a host key obtained through a trusted,
   out-of-band process. Do not use an unverified `ssh-keyscan` result as trust.
5. Start the API:

   ```bash
   cd apps/aegis-api
   go run ./cmd/server
   ```

Automated Windows setup and background execution:

```powershell
cd apps/aegis-api
go run ./cmd/setup-local
cd ../..
scripts\development\start-local.cmd
```

The setup command generates ignored `.env` files and local secrets without
printing them. The server runs on `http://127.0.0.1:8080`; its PID and logs are
under `apps/aegis-api/data`. Stop it with
`scripts\development\stop-local.cmd`. The generated administrator password is
stored only in the ignored `apps/aegis-api/.env` file.
Check the live process with `scripts\development\status-local.cmd`.

PowerShell example:

```powershell
$env:JWT_SIGNING_KEY = "<first generated key>"
$env:CREDENTIAL_MASTER_KEY = "<second generated key>"
$env:BOOTSTRAP_ADMIN_USERNAME = "aegis-admin"
$env:BOOTSTRAP_ADMIN_PASSWORD = "replace-with-a-long-random-password"
go run ./cmd/server
```

Remove the bootstrap password variables after the first successful startup.
Migrations run automatically. To open Swagger UI, send an authenticated
`POST /api/v1/auth/docs-session` from the same browser and then navigate to
`/docs`. The short-lived cookie is HTTP-only and scoped to `/docs`. The
canonical contract is also served to bearer-authenticated clients at
`/api/openapi.yaml`.

## Docker Compose

From the monorepo root, copy `.env.example` to an uncommitted `.env`, populate
both keys and the initial administrator, then run:

```bash
docker compose build
docker compose up -d
docker compose ps
curl -k https://localhost/healthz
```

The local Caddy configuration uses its internal certificate authority. Trust
the Caddy root certificate for local use; production must use the
organization's approved DNS name and certificate process. The API has no host
port and is reachable only through the reverse proxy. SQLite, known hosts, and
backups share the `aegis_data` persistent volume.

## API walkthrough

Authenticate:

```bash
curl -sS https://aegis.internal/api/v1/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"username":"aegis-admin","password":"..."}'
```

Use the returned access token without placing it in shell history in real
environments. Create an encrypted credential:

```bash
curl -sS https://aegis.internal/api/v1/credentials \
  -H "Authorization: Bearer $AEGIS_TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{"name":"office-switches","auth_type":"password","username":"aegis","password":"...","enable_secret":"..."}'
```

Register only an approved management IP:

```bash
curl -sS https://aegis.internal/api/v1/devices \
  -H "Authorization: Bearer $AEGIS_TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{"display_name":"Office SW1","hostname":"OFFICE-SW1","management_ip":"192.0.2.10","management_port":22,"vendor":"cisco","platform":"ios-xe","protocol":"ssh","credential_id":"<uuid>","device_group":"branch-office"}'
```

Read and back up configuration:

```bash
curl -sS -H "Authorization: Bearer $AEGIS_TOKEN" \
  https://aegis.internal/api/v1/devices/<device-id>/config/running

curl -sS -X POST -H "Authorization: Bearer $AEGIS_TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{"config_type":"running"}' \
  https://aegis.internal/api/v1/devices/<device-id>/backups
```

See [`docs/vue-integration.md`](../../docs/vue-integration.md) for the Axios
client pattern and [`docs/operations/cisco-preparation.md`](../../docs/operations/cisco-preparation.md)
for device-side preparation.

## Verification

```bash
go test ./...
go test -race ./...
go vet ./...
go build ./...
```

The API integration test covers login, encrypted credential creation, device
registration, live configuration retrieval through the mock connector, backup
deduplication, changed backup creation, diff, authorization, and audit listing.

## Environment

All supported settings and defaults are documented in `.env.example`.
Production startup requires an HTTPS `PUBLIC_BASE_URL`, a JWT signing key, and
an independent 32-byte credential master key. SQLite is `data/aegis.db` and
configuration backups are below `data/backups` by default.

## Known limitations and next phase

- Local JWT is the only implemented identity adapter; existing-application JWT
  and OIDC should be the next security integration.
- Device-group restrictions are enforced when group assignments exist, but the
  local administration API for assigning users to groups is deferred to the
  production identity/RBAC integration.
- Logout does not maintain a token revocation list.
- Scheduled backups, job history, VLAN/MAC/neighbor APIs, RESTCONF, NETCONF,
  SNMP, and non-Cisco connectors are not implemented.
- Backup-file encryption and online master-key rotation are deferred.
- Cisco parsing must be acceptance-tested against the organization's exact
  models, IOS releases, banners, prompts, and CLI views before production use.

The recommended next development phase is production identity integration,
followed by the embedded scheduler and expanded read-only inventory operations.
