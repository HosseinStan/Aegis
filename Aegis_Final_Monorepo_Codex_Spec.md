# Aegis
## Final Monorepo Codex Implementation Specification

**Product name:** Aegis  
**Product vision:** Enterprise Infrastructure Management Platform  
**Initial module:** Network Configuration and Device Management  
**Backend:** Go  
**Frontend:** Existing Vue.js application  
**Architecture:** Modular Monorepo  
**Initial device support:** Cisco IOS / IOS XE  
**Initial protocol:** SSH  
**Future protocols:** RESTCONF, NETCONF, SNMP  
**Database:** SQLite initially, PostgreSQL-ready  
**Deployment:** Docker / Docker Compose  

---

# Aegis Brand and Module Naming

Aegis is the main product name.

Use these module names consistently:

```text
Aegis Core
Aegis API
Aegis Network
Aegis Config
Aegis Backup
Aegis Inventory
Aegis Discovery
Aegis Monitor
Aegis Vault
Aegis Agent
Aegis CLI
Aegis UI
```

The first implementation focuses on:

```text
Aegis API
Aegis Network
Aegis Config
Aegis Backup
Aegis Vault
Aegis UI Integration
```

Do not name the project Cisco Config Reader. Cisco support is only the first connector inside Aegis.

---

# Final Monorepo Decision

Aegis must be created as a monorepo so all current and future modules can evolve under one product and one repository.

Recommended root structure:

```text
aegis/
├── apps/
│   ├── aegis-api/
│   │   ├── cmd/
│   │   ├── internal/
│   │   ├── migrations/
│   │   ├── api/
│   │   ├── testdata/
│   │   ├── go.mod
│   │   ├── go.sum
│   │   └── README.md
│   ├── aegis-ui/
│   │   └── README.md
│   ├── aegis-agent/
│   │   └── README.md
│   └── aegis-cli/
│       └── README.md
├── packages/
│   ├── sdk-go/
│   │   └── README.md
│   ├── sdk-js/
│   │   └── README.md
│   ├── shared-contracts/
│   │   ├── openapi/
│   │   └── schemas/
│   └── shared-config/
│       └── README.md
├── infrastructure/
│   ├── docker/
│   ├── compose/
│   ├── reverse-proxy/
│   ├── monitoring/
│   └── migrations/
├── docs/
│   ├── architecture/
│   ├── api/
│   ├── deployment/
│   ├── security/
│   ├── operations/
│   └── adr/
├── scripts/
│   ├── development/
│   ├── deployment/
│   └── maintenance/
├── .github/
│   └── workflows/
├── .env.example
├── .gitignore
├── Makefile
├── compose.yaml
├── README.md
├── SECURITY.md
├── CONTRIBUTING.md
└── LICENSE
```

## Monorepo Rules

- `apps/aegis-api` is the first production application.
- `apps/aegis-ui` initially contains integration documentation and optional Vue components because the organization already has a Vue.js application.
- Do not rewrite the existing Vue.js office application unless explicitly requested.
- `apps/aegis-agent` and `apps/aegis-cli` should begin as documented placeholders only.
- `packages/shared-contracts/openapi` contains the canonical OpenAPI specification.
- `packages/sdk-js` will later provide a reusable Vue-compatible JavaScript client.
- `packages/sdk-go` will later support agents, CLI tools, and integrations.
- Do not create fake implementations for future modules.
- Placeholder modules must clearly state their planned purpose and current status.
- The first Codex milestone must remain focused on a working Aegis API MVP.

---

# Repository and Artifact Naming

Use the following defaults:

```text
Repository: aegis
Go module: company.local/aegis/apps/aegis-api
Docker image: aegis-api
Container name: aegis-api
SQLite database: aegis.db
Backup root: ./data/backups
API title: Aegis API
OpenAPI file: packages/shared-contracts/openapi/aegis-api.yaml
```

Use configurable organization or domain placeholders rather than inventing a real company domain.

---

# Product Boundary

Aegis is an infrastructure-management platform.

The initial network module must not create unnecessary coupling with future modules such as:

- VMware
- Active Directory
- Windows Server
- Linux
- Storage
- Backup infrastructure
- Cloud services

The root product can grow, but the first implementation must remain maintainable and deployable as a focused network-management API.

---

## Final Codex Implementation Specification

**Backend:** Go  
**Frontend integration:** Existing Vue.js application  
**Primary database:** SQLite, with a clear migration path to PostgreSQL  
**Initial network vendor:** Cisco IOS / IOS XE  
**Initial protocol:** SSH  
**Future protocols:** RESTCONF, NETCONF, SNMP  
**Deployment:** Docker / Docker Compose  
**Primary purpose:** Provide a secure REST API that allows the existing Vue.js office application to read and manage network-device information without exposing SSH, Telnet, credentials, or device CLI access to end users.

---

# 1. Executive Summary

Build an internal, production-quality **Aegis Network Management REST API** in Go.

The existing office application is written in **Vue.js**. Vue.js must communicate only with this backend through HTTPS REST APIs.

The backend is responsible for communicating with network devices.

```text
Vue.js Office Application
            |
            | HTTPS / REST API
            v
Aegis API
            |
            | SSH for MVP
            | RESTCONF / NETCONF / SNMP later
            v
Cisco and Other Network Devices
```

The first version must allow users to click a button inside the Vue.js application and retrieve the running configuration of an approved Cisco switch.

The user must never:

- Open an SSH client.
- Use Telnet.
- See switch credentials.
- Send arbitrary CLI commands.
- Connect directly from the browser to the switch.

The project must be designed as an extensible internal network-management platform, not as a one-off script.

---

# 2. Core Architectural Decision

The system must use a REST API between Vue.js and the Go backend.

Vue.js example:

```javascript
const response = await axios.get(
  "/api/v1/devices/office-sw1/config/running"
);

console.log(response.data.config);
```

The Go backend then retrieves the configuration from the switch using a secure internal connector.

For the MVP:

```text
Vue.js -> HTTPS REST API -> Go Backend -> SSH -> Cisco Switch
```

For supported newer devices:

```text
Vue.js -> HTTPS REST API -> Go Backend -> RESTCONF -> Cisco IOS XE
```

The API response format must remain consistent regardless of the device vendor or connector protocol.

---

# 3. Product Vision

Develop a small internal alternative to parts of tools such as SolarWinds or ManageEngine, focused on the organization's actual requirements.

Initial capability:

- Read Cisco running configuration.

Planned capabilities:

- Read startup configuration.
- Download configuration.
- Backup configuration automatically.
- Compare configuration versions.
- Show interfaces.
- Show VLANs.
- Show MAC address tables.
- Show CDP and LLDP neighbors.
- Show device inventory and software version.
- Monitor device availability.
- Read device logs.
- Support multiple vendors.
- Integrate with Active Directory or the existing office authentication system.
- Expose all capabilities through a stable REST API for Vue.js.

---

# 4. Project Goals

The system must:

1. Expose secure REST APIs.
2. Integrate cleanly with the existing Vue.js frontend.
3. Support read-only Cisco IOS access in the MVP.
4. Hide all device-management protocols from users.
5. Store device metadata and audit records.
6. protect network credentials.
7. support role-based access control.
8. Create configuration backups.
9. Support configuration history and diff.
10. Be extensible to additional vendors and protocols.
11. Run in Docker.
12. Provide OpenAPI documentation.
13. Include unit and integration tests.
14. Use production-quality security practices.
15. Remain functional as a single-node SQLite deployment while allowing future PostgreSQL migration.

---

# 5. Non-Goals for the MVP

The initial release must not:

- Modify switch configurations.
- Enter Cisco configuration mode.
- Reload or reboot devices.
- Upgrade firmware.
- Write memory.
- Delete files.
- Execute arbitrary commands.
- Provide a generic web terminal.
- Use Telnet.
- Expose credentials.
- Store plain-text passwords.
- Perform uncontrolled network discovery.
- Accept a raw IP address supplied by a normal API user.
- Connect to devices not registered in the device allowlist.

---

# 6. Recommended Technology Stack

## Backend

- Go 1.26 or latest supported stable version
- `net/http` with `chi`, or Gin if justified
- Prefer `go-chi/chi` for lightweight routing
- `golang.org/x/crypto/ssh`
- `database/sql`
- SQLite with `modernc.org/sqlite`
- `log/slog`
- `github.com/google/uuid`
- `github.com/joho/godotenv` for local development only
- `golang.org/x/sync/singleflight`
- `golang.org/x/time/rate`
- OpenAPI 3 specification
- Swagger UI
- `github.com/stretchr/testify` for tests

## Frontend

The frontend already exists in Vue.js.

The backend must provide:

- JSON APIs
- CORS configuration where required
- Bearer-token authentication
- predictable error responses
- pagination
- filtering
- download endpoints
- optional WebSocket or Server-Sent Events in later phases

## Database

MVP:

- SQLite

Future:

- PostgreSQL

Repositories must be defined behind interfaces so the database can be replaced without changing handlers or business logic.

## Deployment

- Docker
- Docker Compose
- Reverse proxy with HTTPS
- Non-root container
- Persistent volume for database and backups

---

# 7. Clean Architecture

Use a layered architecture:

```text
HTTP Handlers
      |
Application Services
      |
Domain Models and Interfaces
      |
Repositories and Device Connectors
      |
SQLite / SSH / RESTCONF / NETCONF / SNMP
```

Rules:

- HTTP handlers must remain thin.
- Business logic belongs in services.
- Database access belongs in repositories.
- Device communication belongs in connectors.
- Vendor-specific parsing must not exist in handlers.
- Connector selection must be based on device vendor, platform, and protocol.
- Authentication must be separate from device communication.
- No cyclic dependencies.

---

# 8. Recommended Folder Structure

```text
aegis/apps/aegis-api/
├── cmd/
│   ├── server/
│   │   └── main.go
│   └── admin/
│       └── main.go
├── internal/
│   ├── api/
│   │   ├── handlers/
│   │   │   ├── devices.go
│   │   │   ├── configs.go
│   │   │   ├── interfaces.go
│   │   │   ├── vlans.go
│   │   │   ├── backups.go
│   │   │   ├── auth.go
│   │   │   └── admin.go
│   │   ├── middleware/
│   │   │   ├── authentication.go
│   │   │   ├── authorization.go
│   │   │   ├── request_id.go
│   │   │   ├── logging.go
│   │   │   ├── rate_limit.go
│   │   │   ├── recovery.go
│   │   │   └── cors.go
│   │   ├── routes.go
│   │   └── responses.go
│   ├── auth/
│   │   ├── service.go
│   │   ├── jwt.go
│   │   ├── oidc.go
│   │   ├── ldap.go
│   │   └── permissions.go
│   ├── domain/
│   │   ├── device.go
│   │   ├── credential.go
│   │   ├── config.go
│   │   ├── backup.go
│   │   ├── interface.go
│   │   ├── vlan.go
│   │   ├── neighbor.go
│   │   ├── audit.go
│   │   └── errors.go
│   ├── services/
│   │   ├── device_service.go
│   │   ├── config_service.go
│   │   ├── backup_service.go
│   │   ├── diff_service.go
│   │   ├── inventory_service.go
│   │   ├── monitoring_service.go
│   │   └── auth_service.go
│   ├── connectors/
│   │   ├── connector.go
│   │   ├── registry.go
│   │   ├── cisco_ios_ssh/
│   │   │   ├── client.go
│   │   │   ├── shell.go
│   │   │   ├── prompt.go
│   │   │   ├── commands.go
│   │   │   ├── parser.go
│   │   │   └── sanitizer.go
│   │   ├── cisco_restconf/
│   │   │   └── client.go
│   │   ├── netconf/
│   │   │   └── client.go
│   │   ├── snmp/
│   │   │   └── client.go
│   │   ├── mikrotik/
│   │   │   └── connector.go
│   │   ├── fortinet/
│   │   │   └── connector.go
│   │   ├── aruba/
│   │   │   └── connector.go
│   │   ├── juniper/
│   │   │   └── connector.go
│   │   └── mock/
│   │       └── connector.go
│   ├── repositories/
│   │   ├── interfaces.go
│   │   ├── sqlite/
│   │   │   ├── device_repository.go
│   │   │   ├── audit_repository.go
│   │   │   ├── backup_repository.go
│   │   │   ├── user_repository.go
│   │   │   └── migrations.go
│   │   └── postgres/
│   │       └── README.md
│   ├── credentials/
│   │   ├── store.go
│   │   ├── encrypted_store.go
│   │   └── vault_store.go
│   ├── scheduler/
│   │   ├── scheduler.go
│   │   ├── backup_job.go
│   │   └── health_check_job.go
│   ├── cache/
│   │   └── memory.go
│   ├── config/
│   │   └── config.go
│   ├── observability/
│   │   ├── logger.go
│   │   ├── metrics.go
│   │   └── tracing.go
│   └── security/
│       ├── encryption.go
│       ├── known_hosts.go
│       └── secret_redaction.go
├── migrations/
│   ├── 001_initial.sql
│   ├── 002_backups.sql
│   ├── 003_users_roles.sql
│   └── 004_schedules.sql
├── api/
│   └── openapi.yaml
├── web/
│   └── swagger/
├── testdata/
│   ├── cisco_running_config.txt
│   ├── cisco_startup_config.txt
│   ├── cisco_show_version.txt
│   ├── cisco_interfaces_status.txt
│   └── ssh_transcripts/
├── scripts/
│   ├── create_admin.go
│   ├── add_device.go
│   ├── rotate_key.go
│   └── import_known_host.go
├── backups/
│   └── .gitkeep
├── data/
│   └── .gitkeep
├── .env.example
├── .gitignore
├── Dockerfile
├── compose.yaml
├── Makefile
├── go.mod
├── go.sum
├── README.md
├── SECURITY.md
└── CONTRIBUTING.md
```

---

# 9. Connector Architecture

Define a vendor-neutral connector interface.

```go
type DeviceConnector interface {
    TestConnection(
        ctx context.Context,
        device Device,
        credential Credential,
    ) error

    GetRunningConfig(
        ctx context.Context,
        device Device,
        credential Credential,
    ) (ConfigResult, error)

    GetStartupConfig(
        ctx context.Context,
        device Device,
        credential Credential,
    ) (ConfigResult, error)

    GetFacts(
        ctx context.Context,
        device Device,
        credential Credential,
    ) (DeviceFacts, error)

    GetInterfaces(
        ctx context.Context,
        device Device,
        credential Credential,
    ) ([]InterfaceStatus, error)

    GetVLANs(
        ctx context.Context,
        device Device,
        credential Credential,
    ) ([]VLAN, error)

    GetNeighbors(
        ctx context.Context,
        device Device,
        credential Credential,
    ) ([]Neighbor, error)
}
```

Connector registry:

```go
type ConnectorRegistry interface {
    Resolve(vendor string, platform string, protocol string) (DeviceConnector, error)
}
```

The first implementation must be:

```text
vendor=cisco
platform=ios
protocol=ssh
```

Future implementations:

- Cisco IOS XE RESTCONF
- Cisco NETCONF
- MikroTik API
- Fortinet REST API or SSH
- Aruba API or SSH
- Juniper NETCONF
- Generic SNMP monitoring connector

---

# 10. Cisco SSH MVP

The backend performs SSH in the background.

Required commands:

```cisco
terminal length 0
show running-config
show startup-config
show version
show inventory
show interfaces status
show ip interface brief
show vlan brief
show mac address-table
show cdp neighbors detail
show lldp neighbors detail
```

The API must never accept a raw command string from a normal user.

Use strongly typed operations:

```go
const (
    OperationRunningConfig Operation = "running_config"
    OperationStartupConfig Operation = "startup_config"
    OperationVersion       Operation = "version"
    OperationInterfaces    Operation = "interfaces"
    OperationVLANs         Operation = "vlans"
)
```

Map operations to fixed commands inside the connector.

## SSH requirements

- Support password authentication.
- Support private-key authentication.
- Support optional enable password.
- Verify SSH host keys.
- Use a known-hosts file.
- Never use insecure host-key verification in production.
- Use context cancellation.
- Enforce connection and command timeouts.
- Enforce output-size limits.
- Close sessions and connections correctly.
- Strip ANSI escape sequences.
- Remove echoed commands.
- Detect Cisco CLI errors.
- Support Cisco prompts ending with `>` or `#`.
- Never intentionally enter configuration mode.
- Never call `configure terminal`.

Recommended defaults:

```text
TCP timeout: 5 seconds
SSH handshake timeout: 10 seconds
Command timeout: 30 seconds
Overall API timeout: 45 seconds
Maximum output: 5 MiB
```

---

# 11. REST API Design

Base path:

```text
/api/v1
```

## Authentication

```http
POST /api/v1/auth/login
POST /api/v1/auth/refresh
POST /api/v1/auth/logout
GET  /api/v1/auth/me
```

If the existing office application already has authentication, support trusted JWT or OIDC integration instead of creating duplicate user accounts.

## Devices

```http
GET    /api/v1/devices
POST   /api/v1/devices
GET    /api/v1/devices/{deviceID}
PATCH  /api/v1/devices/{deviceID}
DELETE /api/v1/devices/{deviceID}
POST   /api/v1/devices/{deviceID}/test-connection
```

Only administrators may create, edit, or disable devices.

## Configuration

```http
GET /api/v1/devices/{deviceID}/config/running
GET /api/v1/devices/{deviceID}/config/startup
GET /api/v1/devices/{deviceID}/config/running/download
GET /api/v1/devices/{deviceID}/config/startup/download
```

## Device information

```http
GET /api/v1/devices/{deviceID}/facts
GET /api/v1/devices/{deviceID}/interfaces
GET /api/v1/devices/{deviceID}/vlans
GET /api/v1/devices/{deviceID}/mac-table
GET /api/v1/devices/{deviceID}/neighbors
```

## Backups

```http
GET  /api/v1/devices/{deviceID}/backups
POST /api/v1/devices/{deviceID}/backups
GET  /api/v1/devices/{deviceID}/backups/{backupID}
GET  /api/v1/devices/{deviceID}/backups/{backupID}/download
GET  /api/v1/devices/{deviceID}/backups/{leftID}/diff/{rightID}
```

## Schedules

```http
GET    /api/v1/backup-schedules
POST   /api/v1/backup-schedules
PATCH  /api/v1/backup-schedules/{scheduleID}
DELETE /api/v1/backup-schedules/{scheduleID}
```

## Audit logs

```http
GET /api/v1/audit-logs
GET /api/v1/audit-logs/{auditID}
```

## Health

```http
GET /healthz
GET /readyz
GET /metrics
```

Protect `/metrics` or expose it only internally.

---

# 12. Response Examples

## Running configuration

```json
{
  "data": {
    "device_id": "office-sw1",
    "hostname": "OFFICE-SW1",
    "vendor": "cisco",
    "platform": "ios",
    "protocol": "ssh",
    "configuration_type": "running",
    "retrieved_at": "2026-07-28T12:30:00Z",
    "from_cache": false,
    "content_hash": "sha256-value",
    "config": "version 15.2\nservice timestamps..."
  },
  "meta": {
    "request_id": "c7347f8a-..."
  }
}
```

## Error response

```json
{
  "error": {
    "code": "DEVICE_CONNECTION_FAILED",
    "message": "The device configuration could not be retrieved.",
    "request_id": "c7347f8a-..."
  }
}
```

Error codes:

- `UNAUTHORIZED`
- `FORBIDDEN`
- `VALIDATION_FAILED`
- `DEVICE_NOT_FOUND`
- `DEVICE_DISABLED`
- `DEVICE_CONNECTION_FAILED`
- `DEVICE_AUTHENTICATION_FAILED`
- `SSH_HOST_KEY_MISMATCH`
- `COMMAND_TIMEOUT`
- `OUTPUT_TOO_LARGE`
- `BACKUP_NOT_FOUND`
- `RATE_LIMITED`
- `INTERNAL_ERROR`

Do not expose internal stack traces, credentials, or raw SSH errors.

---

# 13. Vue.js Integration

The Go backend must be easy to consume from Vue.js.

Example API module:

```javascript
import axios from "axios";

const api = axios.create({
  baseURL: "/api/v1",
  timeout: 60000
});

api.interceptors.request.use((config) => {
  const token = localStorage.getItem("access_token");

  if (token) {
    config.headers.Authorization = `Bearer ${token}`;
  }

  return config;
});

export async function getRunningConfig(deviceId) {
  const response = await api.get(
    `/devices/${encodeURIComponent(deviceId)}/config/running`
  );

  return response.data.data;
}
```

Suggested Vue components:

```text
DeviceList.vue
DeviceDetails.vue
RunningConfigViewer.vue
StartupConfigViewer.vue
DeviceInterfaces.vue
DeviceVLANs.vue
ConfigBackupList.vue
ConfigDiffViewer.vue
AuditLogList.vue
```

The frontend should include:

- Device list
- Device status
- View running config button
- View startup config button
- Copy config
- Download config
- Search within config
- Trigger backup
- Show backup history
- Compare two backups
- Show errors using request IDs
- Permission-aware buttons

The backend specification does not require Codex to rewrite the existing Vue.js app unless explicitly asked. It must provide example integration code and API documentation.

---

# 14. Authentication and RBAC

Support these integration modes:

1. Existing application JWT
2. OpenID Connect
3. Active Directory / LDAP
4. Local JWT authentication for standalone development
5. Static API key only as a temporary MVP option

Preferred production approach:

- Reuse the office application's identity system.
- Validate signed JWTs.
- Check issuer, audience, expiration, and signature.
- Map claims to application roles.

## Roles

### Viewer

Can:

- List approved devices.
- View configurations.
- View interfaces and VLANs.
- Download approved configuration output.

Cannot:

- Create devices.
- Change credentials.
- Schedule backups.
- View all audit logs.
- Change system settings.

### Network Operator

Can:

- Perform Viewer actions.
- Trigger manual backups.
- View backup history.
- Compare backups.
- Test device connectivity.

### Network Administrator

Can:

- Perform all Network Operator actions.
- Add and edit devices.
- Disable devices.
- Manage credentials.
- Manage backup schedules.
- View audit logs.
- Configure connector settings.
- Manage device access permissions.

### System Administrator

Can:

- Manage application roles.
- Manage authentication integration.
- Rotate encryption keys.
- Change global application settings.

Use permission checks, not only role-name comparisons.

Example permissions:

```text
devices.read
devices.manage
configs.read
backups.read
backups.create
backups.schedule
audit.read
credentials.manage
system.manage
```

---

# 15. Device Access Control

Users may not automatically access every device.

Support optional per-device or per-device-group permissions.

Examples:

```text
network-core
network-access
server-room
branch-office
lab
```

A user with permission for `branch-office` must not necessarily access `network-core`.

---

# 16. Credential Security

Do not store credentials in the device record.

Use:

```text
device -> credential_id -> protected credential store
```

Preferred production stores:

- HashiCorp Vault
- Docker secrets
- Kubernetes secrets
- Operating-system secret store
- Encrypted database records

MVP encrypted credential storage:

- AES-256-GCM
- Random nonce per record
- Master key from environment or secret file
- Key never committed to Git
- Strict file permissions
- Credential rotation support

Environment variable:

```env
CREDENTIAL_MASTER_KEY=
```

Never log:

- Passwords
- Private keys
- Enable secrets
- API keys
- JWT tokens
- Authorization headers
- Full configuration contents

---

# 17. Database Schema

## Devices

```sql
CREATE TABLE devices (
    id TEXT PRIMARY KEY,
    display_name TEXT NOT NULL,
    hostname TEXT,
    management_ip TEXT NOT NULL,
    management_port INTEGER NOT NULL DEFAULT 22,
    vendor TEXT NOT NULL,
    platform TEXT NOT NULL,
    protocol TEXT NOT NULL,
    credential_id TEXT NOT NULL,
    device_group TEXT,
    description TEXT,
    enabled INTEGER NOT NULL DEFAULT 1,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    last_success_at TEXT,
    last_failure_at TEXT,
    last_error_code TEXT
);
```

## Credentials

```sql
CREATE TABLE credentials (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    auth_type TEXT NOT NULL,
    username_encrypted BLOB,
    password_encrypted BLOB,
    private_key_encrypted BLOB,
    enable_secret_encrypted BLOB,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);
```

## Users

```sql
CREATE TABLE users (
    id TEXT PRIMARY KEY,
    external_subject TEXT,
    username TEXT NOT NULL,
    display_name TEXT,
    email TEXT,
    enabled INTEGER NOT NULL DEFAULT 1,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);
```

## Roles and permissions

```sql
CREATE TABLE roles (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL UNIQUE,
    description TEXT
);

CREATE TABLE permissions (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL UNIQUE,
    description TEXT
);

CREATE TABLE user_roles (
    user_id TEXT NOT NULL,
    role_id TEXT NOT NULL,
    PRIMARY KEY (user_id, role_id)
);

CREATE TABLE role_permissions (
    role_id TEXT NOT NULL,
    permission_id TEXT NOT NULL,
    PRIMARY KEY (role_id, permission_id)
);
```

## Audit logs

```sql
CREATE TABLE audit_logs (
    id TEXT PRIMARY KEY,
    request_id TEXT NOT NULL,
    user_id TEXT,
    username TEXT,
    source_ip TEXT,
    device_id TEXT,
    action TEXT NOT NULL,
    resource_type TEXT,
    resource_id TEXT,
    succeeded INTEGER NOT NULL,
    error_code TEXT,
    duration_ms INTEGER NOT NULL,
    created_at TEXT NOT NULL
);
```

## Configuration backups

```sql
CREATE TABLE config_backups (
    id TEXT PRIMARY KEY,
    device_id TEXT NOT NULL,
    config_type TEXT NOT NULL,
    storage_path TEXT NOT NULL,
    content_hash TEXT NOT NULL,
    size_bytes INTEGER NOT NULL,
    trigger_type TEXT NOT NULL,
    triggered_by TEXT,
    retrieved_at TEXT NOT NULL,
    created_at TEXT NOT NULL,
    FOREIGN KEY (device_id) REFERENCES devices(id)
);
```

## Backup schedules

```sql
CREATE TABLE backup_schedules (
    id TEXT PRIMARY KEY,
    device_id TEXT,
    device_group TEXT,
    schedule_expression TEXT NOT NULL,
    config_type TEXT NOT NULL DEFAULT 'running',
    enabled INTEGER NOT NULL DEFAULT 1,
    created_by TEXT NOT NULL,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);
```

---

# 18. Backup and Configuration Versioning

The system must support manual and scheduled backups.

Backup flow:

1. Resolve device.
2. Check permission.
3. Retrieve running or startup configuration.
4. Normalize output.
5. Calculate SHA-256 hash.
6. Compare with latest backup.
7. Avoid storing duplicate content unless policy requires every execution.
8. Save configuration to protected storage.
9. Save metadata to the database.
10. Write audit log.

Suggested backup path:

```text
backups/{device_id}/{year}/{month}/{timestamp}_{config_type}.txt
```

Configuration files must not be publicly served.

Downloads must pass through authenticated handlers.

Optional at-rest encryption should be supported.

---

# 19. Configuration Diff

Provide line-based unified diff.

Example response:

```json
{
  "data": {
    "left_backup_id": "backup-1",
    "right_backup_id": "backup-2",
    "changed": true,
    "added_lines": 4,
    "removed_lines": 2,
    "diff": "--- backup-1\n+++ backup-2\n..."
  }
}
```

Ignore volatile lines only through configurable normalization rules.

Do not silently remove meaningful configuration.

Potential volatile items:

- retrieval timestamps inserted by the system
- non-configuration command echo
- terminal prompts

---

# 20. Background Scheduler

Implement an internal scheduler for:

- Scheduled backups
- Device reachability checks
- Optional inventory refresh
- Optional configuration-change checks

Requirements:

- Prevent duplicate concurrent jobs.
- Record job execution.
- Apply per-device locking.
- Add retry with bounded exponential backoff.
- Do not retry authentication failures indefinitely.
- Support graceful shutdown.
- Expose job status to administrators.

For multi-instance deployment later, replace the embedded scheduler with a distributed job system.

---

# 21. Caching and Concurrency

Use short-lived cache for live read requests.

Defaults:

```text
running config cache: 15 seconds
facts cache: 5 minutes
interface cache: 30 seconds
VLAN cache: 1 minute
```

Requirements:

- Use single-flight per device and operation.
- Limit global device connections.
- Limit per-device connections.
- Return cache metadata.
- Never cache authentication failures.
- Invalidate cache after device settings change.
- Manual refresh may bypass cache only for authorized users.

---

# 22. Observability

## Logging

Use structured JSON logging with `log/slog`.

Include:

- request ID
- user ID
- device ID
- action
- duration
- success or failure
- error code
- connector type
- cache hit
- backup ID where applicable

Do not include full configurations or credentials.

## Metrics

Expose Prometheus-compatible metrics:

```text
http_requests_total
http_request_duration_seconds
device_connection_attempts_total
device_connection_failures_total
device_command_duration_seconds
backup_jobs_total
backup_job_failures_total
cache_hits_total
cache_misses_total
active_device_connections
```

## Tracing

Design middleware so OpenTelemetry can be added later.

---

# 23. Security Requirements

- HTTPS for all frontend-to-backend traffic.
- No Telnet implementation.
- No arbitrary command endpoint.
- Strict device allowlist.
- SSH known-host verification.
- Certificate validation for RESTCONF.
- JWT signature validation.
- RBAC and permission checks.
- Rate limiting.
- Secure headers.
- Input validation.
- SQL parameterization.
- Output escaping in Vue.js.
- CORS allowlist.
- Request-size limits.
- Response-size limits.
- Timeouts.
- Secret redaction.
- Audit logging.
- Non-root containers.
- Dependency vulnerability scanning.
- No default production password.
- No secrets in Git.
- No public internet exposure unless behind VPN and identity-aware access controls.

---

# 24. Cisco Read-Only Account

Use a dedicated service account.

Do not reuse a normal administrator account.

Where supported, use Cisco CLI Views or parser views to allow only required commands.

Minimum intended commands:

```cisco
terminal length 0
show running-config
show startup-config
show version
show inventory
show interfaces status
show ip interface brief
show vlan brief
show mac address-table
show cdp neighbors detail
show lldp neighbors detail
```

Deny:

```cisco
configure terminal
reload
write erase
copy
delete
archive
debug
```

Note:

Some Cisco IOS versions require elevated privilege to show the full running configuration. Test the read-only design on the exact switch model and IOS version.

---

# 25. OpenAPI

Create:

```text
api/openapi.yaml
```

Requirements:

- OpenAPI 3.1
- All endpoints documented
- Request and response schemas
- Error schemas
- Bearer authentication scheme
- Permission notes
- Examples
- Pagination format
- Download content types

Expose Swagger UI only to authenticated internal users or administrators.

---

# 26. Docker and Deployment

## Dockerfile

Requirements:

- Multi-stage build
- Pinned base image versions
- Non-root runtime user
- Minimal runtime image
- No build tools in runtime image
- Health check
- No secrets copied into image
- Read-only root filesystem where possible

## Docker Compose

Services:

```text
aegis-api
reverse-proxy
```

Optional future services:

```text
postgres
redis
prometheus
grafana
vault
```

Bind the API only to localhost when reverse proxy is used.

Use a persistent volume for:

- SQLite
- backup files
- known_hosts
- application state

---

# 27. Environment Configuration

Provide `.env.example`:

```env
APP_ENV=development
HTTP_LISTEN_ADDRESS=:8080
PUBLIC_BASE_URL=http://localhost:8080

DATABASE_DRIVER=sqlite
DATABASE_DSN=./data/network_api.db

AUTH_MODE=local_jwt
JWT_ISSUER=aegis-api
JWT_AUDIENCE=office-vue-app
JWT_ACCESS_TOKEN_TTL=15m
JWT_REFRESH_TOKEN_TTL=168h
JWT_SIGNING_KEY=

OIDC_ISSUER_URL=
OIDC_CLIENT_ID=
OIDC_AUDIENCE=

LDAP_URL=
LDAP_BASE_DN=
LDAP_BIND_DN=
LDAP_BIND_PASSWORD=

CREDENTIAL_MASTER_KEY=
SSH_KNOWN_HOSTS_FILE=./data/known_hosts

SSH_CONNECT_TIMEOUT=5s
SSH_HANDSHAKE_TIMEOUT=10s
DEVICE_COMMAND_TIMEOUT=30s
REQUEST_TIMEOUT=45s
MAX_OUTPUT_BYTES=5242880

CONFIG_CACHE_TTL=15s
FACTS_CACHE_TTL=5m
INTERFACE_CACHE_TTL=30s

MAX_CONCURRENT_DEVICE_CONNECTIONS=10
MAX_CONNECTIONS_PER_DEVICE=1

BACKUP_STORAGE_PATH=./backups
BACKUP_ENCRYPTION_ENABLED=false

CORS_ALLOWED_ORIGINS=http://localhost:5173
TRUSTED_PROXY_CIDRS=

LOG_LEVEL=info
METRICS_ENABLED=true
```

Validate all required variables during startup.

Fail fast when production security settings are missing.

---

# 28. Testing Strategy

## Unit tests

Test:

- Device validation
- IP and port validation
- Permission checks
- JWT validation
- Credential encryption
- Connector registry
- Cisco prompt detection
- Command mapping
- Output sanitization
- ANSI removal
- Cisco error detection
- Cache behavior
- Single-flight behavior
- Rate limiting
- Backup hashing
- Duplicate backup handling
- Diff generation
- Audit logging
- Error mapping

## Integration tests

Test:

- HTTP API
- Authentication
- RBAC
- SQLite migrations
- Device repository
- Backup repository
- Mock connector
- Audit writes
- Download endpoints
- OpenAPI validation

## SSH tests

Use a fake SSH server or controlled transcript fixtures.

Test:

- Successful connection
- Password login
- SSH-key login
- Host-key mismatch
- Authentication failure
- Timeout
- Prompt detection
- Pagination
- Cisco CLI error
- Connection reset
- Oversized output
- Context cancellation

## Manual acceptance tests

Use an approved non-production switch.

Confirm:

- Configuration retrieval works.
- User never uses SSH.
- Read-only account cannot configure the switch.
- Full output is returned.
- Host key is verified.
- Credentials do not appear in logs.
- Vue.js can consume the API.
- Backup and diff work.
- Audit record is created.

---

# 29. CI/CD

Add GitHub Actions or equivalent pipeline.

Pipeline steps:

1. Checkout
2. Set up Go
3. Cache dependencies
4. Run `gofmt` verification
5. Run `go vet`
6. Run tests with race detector where practical
7. Run static analysis
8. Run dependency vulnerability scan
9. Validate OpenAPI
10. Build binary
11. Build Docker image
12. Generate test report
13. Do not publish unless explicitly configured

Suggested tools:

```text
go test
go vet
staticcheck
govulncheck
golangci-lint
```

---

# 30. Implementation Phases for Codex

## Phase 1: Foundation

- Create Go module.
- Create folder structure.
- Implement configuration loading.
- Implement logging.
- Implement graceful shutdown.
- Implement health and readiness endpoints.
- Add Docker and Compose.
- Add Makefile.

## Phase 2: Database

- Add SQLite.
- Add migrations.
- Implement repository interfaces.
- Implement device repository.
- Implement audit repository.
- Implement user and role repositories.

## Phase 3: Authentication and RBAC

- Implement local JWT for development.
- Implement permission middleware.
- Add role seeds.
- Design OIDC and LDAP interfaces.
- Document existing-app JWT integration.

## Phase 4: Credential Store

- Implement AES-256-GCM encrypted store.
- Load master key securely.
- Add credential CRUD for administrators.
- Add secret redaction.
- Add rotation support.

## Phase 5: Connector Framework

- Define vendor-neutral connector interface.
- Implement connector registry.
- Implement mock connector.
- Add test fixtures.

## Phase 6: Cisco IOS SSH

- Implement known-host verification.
- Implement password and key authentication.
- Implement Cisco interactive shell.
- Implement prompt detection.
- Implement fixed operations.
- Implement output sanitization.
- Add timeouts and output limits.
- Add tests.

## Phase 7: Core REST API

- Device CRUD.
- Test connection.
- Running config.
- Startup config.
- Facts.
- Interfaces.
- VLANs.
- MAC table.
- Neighbors.
- Download endpoints.
- Error mapping.
- Request IDs.
- Rate limiting.

## Phase 8: Vue.js Integration

- Add CORS.
- Add frontend examples.
- Document Axios client.
- Document authentication flow.
- Provide sample response schemas.
- Provide optional simple Vue components only as examples.

## Phase 9: Backups and Diff

- Manual backup.
- Backup storage.
- Hashing.
- Backup history.
- Download.
- Diff.
- Retention policy.

## Phase 10: Scheduler

- Scheduled backups.
- Device health checks.
- Job locking.
- Retry policy.
- Job logs.
- Graceful shutdown.

## Phase 11: Observability

- Metrics.
- Structured audit logs.
- Admin job status.
- Optional tracing hooks.

## Phase 12: Future Connectors

- Cisco RESTCONF.
- NETCONF.
- SNMP monitoring.
- Multi-vendor connectors.

Do not implement all future vendors in the MVP. Create interfaces and placeholder packages with documentation.

---

# 31. MVP Scope

The first deliverable must include:

- Go REST API
- Vue.js-compatible JSON responses
- JWT or existing-token authentication
- Viewer and Network Administrator permissions
- SQLite
- Device allowlist
- Encrypted credentials
- Cisco IOS SSH connector
- Known-host verification
- Running config endpoint
- Startup config endpoint
- Device facts endpoint
- Interface-status endpoint
- Download endpoint
- Manual backup
- Backup history
- Basic configuration diff
- Audit logging
- Rate limiting
- OpenAPI
- Docker
- Unit and integration tests
- README
- Security documentation

The MVP should not become blocked by future vendor support.

---

# 32. Acceptance Criteria

The project is accepted when:

- `go build ./...` succeeds.
- `go test ./...` succeeds.
- `go vet ./...` succeeds.
- Docker image builds.
- Docker Compose starts the service.
- SQLite migrations run automatically.
- A test administrator can register a Cisco device.
- Credentials are encrypted.
- An authenticated Vue.js client can list devices.
- An authenticated user can retrieve running configuration.
- An authenticated user can retrieve startup configuration.
- Unauthorized users receive HTTP 401.
- Users without permission receive HTTP 403.
- Unknown devices return HTTP 404.
- Device connection failures return safe HTTP 502 responses.
- Device timeouts return HTTP 504 responses.
- The backend verifies SSH host keys.
- No Telnet code exists.
- No arbitrary CLI endpoint exists.
- Configuration output is not written to normal application logs.
- A user can trigger a manual backup.
- A user can view backup history.
- A user can compare two backups.
- Every sensitive operation creates an audit record.
- OpenAPI documentation is complete.
- README explains Vue.js integration.
- README explains Cisco-side preparation.
- The application runs as a non-root container.

---

# 33. Codex Rules

Codex must follow these instructions:

- Implement production-quality Go code.
- Keep the project runnable after every phase.
- Do not use Telnet.
- Do not expose a generic command execution API.
- Do not hard-code secrets.
- Do not disable SSH host-key verification.
- Do not log full configurations.
- Do not mix vendor-specific code into handlers.
- Use context cancellation and explicit timeouts.
- Use dependency injection through constructors.
- Prefer small interfaces.
- Use repository interfaces.
- Use migrations.
- Use parameterized SQL.
- Use structured logging.
- Add tests with each component.
- Use `gofmt`.
- Use `go vet`.
- Add OpenAPI.
- Add Docker.
- Add `.env.example`.
- Add `SECURITY.md`.
- Mark incomplete security-sensitive code with `TODO SECURITY`.
- Do not claim RESTCONF support works until implemented and tested.
- Do not implement placeholder features as fake working features.
- Be explicit about known limitations.

At the end of implementation, Codex must provide:

1. Final directory tree.
2. Setup steps.
3. Environment configuration.
4. Database migration steps.
5. Local run commands.
6. Docker run commands.
7. Cisco switch preparation.
8. Vue.js integration examples.
9. API examples with `curl`.
10. Test results.
11. Security assumptions.
12. Known limitations.
13. Recommended next development phase.

---

# 34. Final Design Summary

Use this final architecture:

```text
Existing Vue.js Office Application
                |
                | HTTPS / JSON REST API
                v
Aegis API in Go
                |
                +-- Authentication and RBAC
                +-- Device Registry
                +-- Encrypted Credential Store
                +-- Audit Logs
                +-- Backup Scheduler
                +-- Configuration Versioning
                +-- Configuration Diff
                +-- Connector Registry
                |
                +-- Cisco IOS SSH Connector (MVP)
                +-- Cisco RESTCONF Connector (Future)
                +-- NETCONF Connector (Future)
                +-- SNMP Connector (Future)
                +-- Multi-vendor Connectors (Future)
                |
                v
        Approved Network Devices
```

The user interacts only with the Vue.js application.

The Vue.js application interacts only with the REST API.

The REST API securely performs the required network communication in the background.

This creates a secure, extensible, organization-specific network-management platform rather than a temporary configuration-reading script.

---

# Codex Starting Instruction

Give Codex this repository-level instruction:

```text
Create the Aegis monorepo described in this specification.

Start with a production-ready MVP of apps/aegis-api.

Do not attempt to implement every future Aegis module in the first pass.

The first working milestone must include:

- Go REST API
- Vue.js-compatible JSON API
- SQLite
- authentication and RBAC
- encrypted credential storage
- Cisco IOS SSH connector
- SSH host-key verification
- running configuration retrieval
- startup configuration retrieval
- device facts and interface status
- manual configuration backup
- backup history
- basic configuration diff
- audit logging
- rate limiting
- OpenAPI
- Docker
- tests
- documentation

Create placeholder documentation only for aegis-agent, aegis-cli, sdk-go, sdk-js,
Aegis Discovery, Aegis Monitor, RESTCONF, NETCONF, SNMP, and multi-vendor support.

Keep the repository runnable and testable after every implementation phase.
```

---

# Recommended First Codex Milestone

Codex should complete the following before moving to advanced functionality:

1. Create the monorepo.
2. Build `apps/aegis-api`.
3. Add configuration loading and graceful shutdown.
4. Add SQLite migrations.
5. Add device, credential, audit, and backup repositories.
6. Add local development JWT authentication.
7. Add permission-based RBAC.
8. Add AES-256-GCM credential encryption.
9. Add connector registry.
10. Add Cisco IOS SSH connector.
11. Add REST endpoints required by the Vue.js frontend.
12. Add manual backup and diff.
13. Add OpenAPI.
14. Add Docker Compose.
15. Add automated tests.
16. Add complete README and security documentation.

The first milestone must produce a usable system, not only scaffolding.

---

# Final Aegis Architecture

```text
Aegis Monorepo
│
├── apps/aegis-api
│       |
│       +-- REST API
│       +-- Authentication and RBAC
│       +-- Device Registry
│       +-- Encrypted Credential Store
│       +-- Cisco SSH Connector
│       +-- Configuration Reader
│       +-- Backup and Diff
│       +-- Audit Logs
│
├── apps/aegis-ui
│       +-- Vue.js integration documentation
│       +-- Optional reusable components
│
├── apps/aegis-agent
│       +-- Future Windows/Linux agent
│
├── apps/aegis-cli
│       +-- Future administration CLI
│
├── packages/sdk-js
│       +-- Future Vue.js-compatible API client
│
├── packages/sdk-go
│       +-- Future Go integrations
│
└── infrastructure
        +-- Docker
        +-- Reverse proxy
        +-- Monitoring
        +-- Deployment assets
```

The existing Vue.js office application communicates only with Aegis API through HTTPS.

Aegis API securely communicates with approved infrastructure devices through internal connectors.

For the MVP, the connector is Cisco IOS over SSH.

The user never directly uses SSH or Telnet.
