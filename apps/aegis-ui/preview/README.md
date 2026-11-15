# Aegis local MVP preview

**DEMO — SYNTHETIC DATA — RISK NOT EVALUATED**

Dashboard, Inventory, device profiles, saved Configuration, Backup History, snapshot viewing, Configuration Comparison and Change Review share the existing Go API, native authentication/RBAC, services and disposable SQLite. Change Review is an evidence preview: **RISK NOT EVALUATED — DEMO / UNVERIFIED**. It does not integrate the unfinished Task #6 risk engine. This zero-dependency Node/HTML preview is not the final Vue implementation; the [Vue architecture](../../../docs/vue-integration.md) remains the intended frontend direction.

## Run the isolated Go-backed preview

Use an already verified **Go 1.26.11** installation and existing Node.js. Do not launch cmd/server: that production entrypoint registers Cisco SSH connectors.

**Do not open public/index.html directly with file://.** Root-relative styles, JavaScript modules and the same-origin API require the HTTP server. Launch the existing Node/Go supervisor below and open its printed **http://127.0.0.1:<port>/** URL in the browser. No npm install or other dependency installation is required. If the stylesheet/server is missing, the HTML includes a plain launch reminder; it is hidden during normal HTTP use.

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

The supervisor prints only non-secret Go/preview URLs, seed counts and a sanitized authentication result. Open the preview URL with #/overview, #/devices, #/backups, #/compare or #/review. The ignored runtime.json holds the preview URL/PID, never credentials. The supervisor accepts these local terminal commands:

- **stop-api:** stop only its Go child; refreshed Go workspace views, including Change Review, show unavailable while the preview server remains available.
- **start-api:** create a fresh disposable SQLite database and identity session; then press Retry in the browser.
- **quit** or Ctrl+C: stop this supervisor and its Go child.

Each Go start creates a new aegis-inventory-demo-* directory under the external parent. It never accepts a database DSN or opens an existing database. Disposable directories are retained on shutdown; the supervisor never cleans, restores or automatically reruns them.

Running server.mjs alone serves the preview shell, but all Go workspace views explicitly show unavailable. They never substitute Node fixture results.

## Unified Go Dashboard

- Dashboard uses only existing bridge reads: GET /api/go/v1/devices and GET /api/go/v1/devices/:id/backups for every returned device. No new Go endpoint, aggregation service or permission is introduced.
- Device count comes from that Inventory response; snapshot count sums its corresponding histories; coverage counts unique Go devices with at least one saved backup. Latest timestamp and recent rows come from those same persisted records. An empty Inventory shows zero counts and unavailable percentage, rather than fixture devices.
- Group distribution counts the actual device_group values. It represents no network links, neighbors, health or risk assessment. No fixture risk totals enter Dashboard metrics.
- Device links open Go profiles, snapshot buttons read native BackupService.Content, and Compare links carry actual current Go backup UUIDs. Refresh Dashboard reads the complete collection again after a Go restart; loading/failure clears the displayed metrics and records instead of retaining a misleading old result.
- The shared Inventory navigation counter uses the successfully loaded Go device count. The history shortcut opens Go Backup History. Source/request details are available in a collapsed disclosure.
- The isolated seed currently gives **4 devices, 12 snapshots, 100% coverage and 3 groups**. The UI calculates these from responses; they are not hardcoded Dashboard values. The Dashboard links to Go-backed Change Review without adding risk statistics.

## Real Go Inventory flow

    Browser GET /api/go/v1/devices
      -> same-origin Node adapter
      -> native Go POST /api/v1/auth/login (ephemeral viewer, server-held)
      -> native Go GET /api/v1/devices?limit=100
      -> Authenticate / Require(devices.read)
      -> DeviceService / SQLite repository / disposable database

Device details use /api/go/v1/devices/:id -> /api/v1/devices/:id.

### Saved device Configuration tab

- Open a Go device and choose Configuration, or use #/devices/:id?tab=configuration. The tab reads that device's existing backup history, filters persisted running-config snapshots and initially selects the newest timestamp. It then reads the selected content through the existing native BackupService.Content bridge path.
- Timestamp, actual backup UUID, SHA-256, byte size and read-only synthetic content come from Go responses. The newest selection states **Latest saved configuration — not live device state.** Older selections state **Selected saved configuration — not live device state** and Older saved version. No live configuration endpoint, connector or SSH is used.
- The version selector contains only that Go device's saved running-config UUIDs. Compare opens the selected UUID and an adjacent saved version in the existing native Go comparison. Fixture configurations and browser-memory copies never join this list.
- The URL preserves the selected UUID. Retry retains it, including after a Go restart; a stale UUID shows not-found instead of resolving to a replacement. **Refresh versions** explicitly discards the selection, reads the current list and selects its latest saved version. Empty, loading, authorization failure, 404 and unavailable states show no fallback content.
- Go device profiles do not merge fixture configuration or finding counts. Their Change review tab opens the same device in the Go-backed review; when viewing a saved Configuration, it carries that selected UUID and an adjacent Go version. Configuration text wraps long lines within its own read-only panel at narrow widths.

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

## Go-backed Change Review

- Open #/review with optional device, left and right parameters. The legacy #/risk route maps to this Go view, preserving its requested selection; it no longer renders fixture findings.
- Review and Configuration Compare share the same loader, validated Go Inventory/history/content/Diff reads, selectors and comparison presentation. No second diff engine, Go service, endpoint, migration or permission is added. Side-by-side uses the existing browser alignment; native Go counts and Unified text remain authoritative.
- The summary shows actual device identity, before/after UUIDs and timestamps, native changed status and line counts. Evidence paths/request IDs remain available. Links open the exact same UUID pair in full Compare or a selected saved Configuration.
- **RISK NOT EVALUATED — DEMO / UNVERIFIED** appears even during loading/failure. No risk classification, compliance finding, vulnerability finding, security approval or Task #6 engine result is produced.
- A checkbox records **Viewed in this demo — not approved or persisted** only for the exact ordered device/snapshot pair in this tab's memory. Reload clears it. No storage, HTTP write, audit or authorization record is used.
- Retry keeps stale UUIDs and shows not-found; Refresh snapshots explicitly reloads current history and resets versions. Missing/cross-device IDs or an unavailable Go API never fall back to fixture data. Review requires disabled mock demo records.

## Explicit legacy fixture material

- GET /api/mock/v1/devices/:id/backups: synthetic metadata without configuration text.
- GET /api/mock/v1/devices/:id/backups/:backupId: selected synthetic configuration text.
- These retain the Local Mock API source, synthetic markers and request IDs as legacy material. Dashboard, Inventory, device Configuration, Backup History and Compare never use them as fallback.
- The active Compare screen uses Go snapshots only. Older independent demo workspaces retain their previous fixture comparison experience.
- Browser-memory copies can still be viewed, but their Compare action is removed and they are explicitly excluded from Go comparison.
- Create demo backup copies a legacy fixture only into browser memory, in a separate section labeled NOT persisted to Go or Node. It creates no HTTP backup, SQLite row or file. Reload discards it.
- Old prewritten risk findings are no longer rendered in the active workspace. Historical demo-data.json bytes remain unchanged for the separate browser-memory copy feature; those copies never join Go Review, Configuration or Compare.

## Minimal validation and limits

For the Change Review milestone, Node syntax checks, focused existing HTTP reads, native change counts/identical/404 checks, browser selection and Compare links, desktop/narrow layouts and a controlled Go stop/start verify the visible flow. For this candidate, build the isolated demo entrypoint with verified Go 1.26.11 before running the preview; the earlier local demo evidence predates this Go toolchain update. Dashboard, Inventory, saved Configuration, Backup History, Content and native Diff remain usable. These checks are not Phase E, race, capacity, performance or release validation.

No real SSH, external device addresses, credential handling, production database, discovery or configuration changes are enabled.

This integration candidate keeps the updated **Go 1.26.11** go.mod, CI and Docker baseline already merged into main, addressing reported GO-2026-6809 and GO-2026-6859 standard-library issues. The Go CI vulnerability scan is enabled; full production release acceptance is still deferred.

**Phase E: DEFERRED / NOT ACCEPTED. Production release: BLOCKED.** Historical E1 Go cases passed their executed scenarios, but Controller acceptance failed. No merge/deployment, Phase F or Task #7 acceptance is granted.
