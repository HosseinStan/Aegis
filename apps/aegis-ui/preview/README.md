# Aegis local MVP preview

**DEMO — SYNTHETIC DATA — UNVERIFIED RISK FINDINGS**

Inventory, device profiles, Backup History, snapshot viewing and Configuration Comparison use the existing Go API, native authentication/RBAC, services and disposable SQLite. Dashboard and risk review remain explicitly fixture-based demonstrations. This zero-dependency Node/HTML preview is not the final Vue implementation; the [Vue architecture](../../../docs/vue-integration.md) remains the intended frontend direction.

## Run the isolated Go-backed preview

Use an already verified **Go 1.26.10** installation and existing Node.js. Do not launch cmd/server: that production entrypoint registers Cisco SSH connectors.

Work in a separate development clone. Choose an absolute external DemoRoot outside Git for the binary, caches and disposable runs. In the build process only, select the verified GOROOT and set:

- GOTOOLCHAIN=local, GOENV=off, GOWORK=off, GOPROXY=off.
- GOMODCACHE to a private copy of already available dependencies verified against go.sum. No downloads or module upgrades are required.
- GOCACHE and GOTMPDIR to fresh locations under DemoRoot.
- CGO_ENABLED=0. The existing SQLite driver is pure Go.

Build from apps/aegis-api, using the verified executable explicitly:

    <verified-go-root>/bin/go build -mod=readonly -trimpath -o <DemoRoot>/aegis-inventory-demo.exe ./cmd/inventory-demo

Then run from any directory, replacing the absolute paths:

    node <clone>/apps/aegis-ui/preview/run-go-demo.mjs --go-binary=<DemoRoot>/aegis-inventory-demo.exe --run-parent=<DemoRoot>/disposable

On non-Windows systems the binary may be named aegis-inventory-demo. An optional --port=4318 fixes only the preview port. Both servers bind to 127.0.0.1; no bind-address override exists.

The supervisor prints only non-secret Go/preview URLs, seed counts and a sanitized authentication result. Open the preview URL with #/devices, #/backups or #/compare. The ignored runtime.json holds the preview URL/PID, never credentials. The supervisor accepts these local terminal commands:

- **stop-api:** stop only its Go child; Inventory/history/comparison show unavailable while the preview and fixture-based Dashboard/Risk Review remain available.
- **start-api:** create a fresh disposable SQLite database and identity session; then press Retry in the browser.
- **quit** or Ctrl+C: stop this supervisor and its Go child.

Each Go start creates a new aegis-inventory-demo-* directory under the external parent. It never accepts a database DSN or opens an existing database. Disposable directories are retained on shutdown; the supervisor never cleans, restores or automatically reruns them.

Running server.mjs alone still serves Dashboard/Risk Review fixtures, but Go Inventory, Backup History and Compare explicitly show unavailable. They never substitute Node fixture results.

## Real Go Inventory flow

    Browser GET /api/go/v1/devices
      -> same-origin Node adapter
      -> native Go POST /api/v1/auth/login (ephemeral viewer, server-held)
      -> native Go GET /api/v1/devices?limit=100
      -> Authenticate / Require(devices.read)
      -> DeviceService / SQLite repository / disposable database

Device details use /api/go/v1/devices/:id -> /api/v1/devices/:id.

The dedicated cmd/inventory-demo entrypoint registers ONLY internal/connectors/mock. It does not import Cisco SSH, load production config/.env, construct ConfigService, or accept device-operation routes. Four disabled records use documentation-range addresses and stable demo IDs. Native Go records are returned by existing services; the adapter only projects presentation fields.

A viewer is created with the existing password hashing, JWT and role implementation. Random bootstrap credentials travel through an anonymous stdin pipe, never argv, environment, logs or public files. A fresh random JWT signing key stays in Go process memory. Tokens stay in the Node process, never browser storage, responses or source. Native unauthenticated Inventory requests still return 401.

The existing schema requires a device credential FK. Bootstrap inserts one **inert reference with no username/password/private-key/enable-secret blobs**, not a usable device credential. No credential store is instantiated and no connector operation is invoked. This preserves the existing schema and business code without handling device credentials.

The development entrypoint exposes only health, login/me, Inventory list/profile and the backup List/Content/Diff GET routes described below. The browser adapter exposes only those Inventory/backup GETs. All HTTP writes, backup creation/download, configuration reads, discovery, credential APIs and unrelated native operations remain blocked. Its upstream is the loopback child started by the supervisor, with a required isolated-demo response marker.

Inventory shows **Aegis Go API — Isolated Demo**, actual GET paths/status and the native Go request ID. Model/software/serial facts absent from the Inventory contract are labeled Not supplied; risk is Unknown / not assessed. No fixture profile fields are presented as Go results. Loading, empty/no-match, 404, unavailable and Retry states remain explicit.

## Go-backed synthetic backup history

- Bridge GET /api/go/v1/devices/:id/backups -> native GET /api/v1/devices/:id/backups.
- Bridge GET /api/go/v1/devices/:id/backups/:backupId -> native GET /api/v1/devices/:id/backups/:backupId.
- Native API handlers use BackupService.List and BackupService.Content unchanged. Metadata is in SQLite; Content opens the synthetic snapshot file and verifies its SHA-256. No competing backup API or database reader is implemented in Node.
- Application-side seed.go writes three small, banner-marked synthetic files per device: **12 Go backups**. It generates actual UUIDs, computes byte sizes/SHA-256, uses the same device/year/month/timestamp_running_UUID.txt path convention, persists via Store.CreateBackup, and verifies each seed through BackupService.Content. It never calls BackupService.Create or contacts a connector.
- BackupService is constructed with ConfigService=nil for this read-only scope. HTTP allowlisting permits List/Content/Diff and prevents reaching Create or device operations.
- A dedicated isolated_demo_backup_reader role adds only the existing backups.read permission to the ephemeral viewer. Only the brand-new disposable database is initialized; production RBAC definitions/migrations are unchanged. No backups.create permission is granted.
- The adapter preserves native backup IDs, timestamps, type, size and hash. Native snapshot responses contain backup/config; the adapter flattens that envelope into the UI model. Display labels are derived from the real UUID, not fabricated IDs or replacement data.
- History first reads Go Inventory and uses only its four devices, including the device-detail Backup tab. No 12-device fixture history is merged. The source label is **Aegis Go API — Disposable SQLite Demo** with GET paths and native request IDs.
- Unknown/cross-device IDs return native 404; missing/invalid native authorization returns 401 without content. Loading, empty, unavailable, 404 and Retry states are explicit. Restart creates fresh IDs/database contents; Refresh history obtains the new records.

## Native Go configuration comparison

- Bridge GET /api/go/v1/devices/:id/backups/:leftId/diff/:rightId -> native GET /api/v1/devices/:id/backups/:leftId/diff/:rightId.
- The existing native handler and BackupService.Diff are used unchanged, including both stored-content integrity checks. The Node bridge preserves the native unified string, changed flag and added/removed counts. It calculates no diff.
- Compare first reads Go Inventory and backup metadata, then requests both selected Go contents and the native Diff through the authenticated same-origin bridge. Only Go UUIDs appear in selectors; fixture IDs and browser-memory copies never join this collection.
- **Counts always come from Go. Unified displays the native response text exactly** when Changes only is off. Changes only hides context lines while retaining the native header/hunk/change lines in order.
- Side-by-side reuses the existing browser line-alignment presentation with the same two fetched Go snapshots. It is explicitly labeled browser-side presentation; its computed counts are not used.
- Device/version selectors, Swap, Changes only, identical selections and native 404/unavailable/Retry states are supported. Go Backup History Compare opens the selected persisted snapshot and an adjacent Go version.
- Canonical comparison URLs contain the actual selected UUIDs. A Go restart regenerates them: Retry preserves the requested IDs and shows not-found for stale selections; **Refresh snapshots** explicitly clears selection and reads the current history. No UUID is silently replaced.
- Long IDs and HTTP metadata are collapsed by default. Narrow layouts wrap code in both side-by-side panes, keeping both columns visible.
- The existing mock-only connector, disposable initialization, server-held identity and backups.read role are unchanged. No extra permission or write route is introduced.

## Explicit legacy fixture material

- GET /api/mock/v1/devices/:id/backups: synthetic metadata without configuration text.
- GET /api/mock/v1/devices/:id/backups/:backupId: selected synthetic configuration text.
- These retain the Local Mock API source, synthetic markers and request IDs as legacy material. Inventory, Backup History and Compare never use them as fallback.
- The active Compare screen uses Go snapshots only. Older independent demo workspaces retain their previous fixture comparison experience.
- Browser-memory copies can still be viewed, but their Compare action is removed and they are explicitly excluded from Go comparison.
- Create demo backup copies a legacy fixture only into browser memory, in a separate section labeled NOT persisted to Go or Node. It creates no HTTP backup, SQLite row or file. Reload discards it.
- Dashboard counts/topology, the sample Configuration tab and prewritten risk findings remain explicitly fixture-based. Reviewing a finding does not create authorization or security acceptance.

## Minimal validation and limits

Node syntax checks, a build of only cmd/inventory-demo with offline dependency verification, focused HTTP Diff/content/count/authorization/404 checks, browser comparison controls, narrow-layout checks and a controlled Go stop/start with stale-selection refresh cover this milestone. Existing Inventory and Backup History remain usable. These are not Phase E, race, capacity, performance or release validation.

No real SSH, external device addresses, credential handling, production database, discovery or configuration changes are enabled.

Known CI issue reported by independent review: **GO-2026-6809** in **Go 1.26.9**, fixed in **Go 1.26.10**. The local demo uses the verified 1.26.10 toolchain. Repository-wide go.mod/CI/Docker correction remains a separate task; this change does not alter those files or disable govulncheck.

**Phase E: DEFERRED / NOT ACCEPTED. Production release: BLOCKED.** Historical E1 Go cases passed their executed scenarios, but Controller acceptance failed. No merge/deployment, Phase F or Task #7 acceptance is granted.
