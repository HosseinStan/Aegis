# Aegis local MVP preview

**DEMO — SYNTHETIC DATA — UNVERIFIED RISK FINDINGS**

Inventory and device profiles now use the existing Go API, native authentication/RBAC, DeviceService and SQLite repository. Backup history and comparison keep their Node synthetic HTTP API. Dashboard and risk review remain fixture demonstrations. This zero-dependency Node/HTML preview is not the final Vue implementation; the [Vue architecture](../../../docs/vue-integration.md) remains the intended frontend direction.

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

The supervisor prints only non-secret Go/preview URLs and a sanitized authentication result. Open the preview URL with #/devices. The ignored runtime.json holds the preview URL/PID, never credentials. The supervisor accepts these local terminal commands:

- **stop-api:** stop only its Go child; the browser preview and Node backup API stay running.
- **start-api:** create a fresh disposable SQLite database and identity session; then press Retry in the browser.
- **quit** or Ctrl+C: stop this supervisor and its Go child.

Each Go start creates a new aegis-inventory-demo-* directory under the external parent. It never accepts a database DSN or opens an existing database. Disposable directories are retained on shutdown; the supervisor never cleans, restores or automatically reruns them.

Running server.mjs alone still serves the synthetic backup/comparison experience, but Go Inventory explicitly shows unavailable. It never substitutes the Node Inventory fixture.

## Real Go Inventory flow

    Browser GET /api/go/v1/devices
      -> same-origin Node adapter
      -> native Go POST /api/v1/auth/login (ephemeral viewer, server-held)
      -> native Go GET /api/v1/devices?limit=100
      -> Authenticate / Require(devices.read)
      -> DeviceService / SQLite repository / disposable database

Device details use /api/go/v1/devices/:id -> /api/v1/devices/:id.

The dedicated cmd/inventory-demo entrypoint registers ONLY internal/connectors/mock. It does not import Cisco SSH, load production config/.env, construct configuration/backup services, or accept device-operation routes. Four disabled records use documentation-range addresses and stable demo IDs. Native Go records are returned by existing services; the adapter only projects presentation fields.

A viewer is created with the existing password hashing, JWT and role implementation. Random bootstrap credentials travel through an anonymous stdin pipe, never argv, environment, logs or public files. A fresh random JWT signing key stays in Go process memory. Tokens stay in the Node process, never browser storage, responses or source. Native unauthenticated Inventory requests still return 401.

The existing schema requires a device credential FK. Bootstrap inserts one **inert reference with no username/password/private-key/enabled-secret blobs**, not a usable device credential. No credential store is instantiated and no connector operation is invoked. This preserves the existing schema and business code without handling device credentials.

The development entrypoint exposes only health, login/me and Inventory list/profile routes. The browser adapter exposes only Inventory GETs; all writes, configuration reads, discovery, credential APIs and other native operations are blocked. Its upstream is the loopback child started by the supervisor, with a required isolated-demo response marker.

Inventory shows **Aegis Go API — Isolated Demo**, actual GET paths/status and the native Go request ID. Model/software/serial facts absent from the Inventory contract are labeled Not supplied; risk is Unknown / not assessed. No fixture profile fields are presented as Go results. Loading, empty/no-match, 404, unavailable and Retry states remain explicit.

## Synthetic flows retained

- GET /api/mock/v1/devices/:id/backups: synthetic metadata without configuration text.
- GET /api/mock/v1/devices/:id/backups/:backupId: selected synthetic configuration text.
- These retain the Local Mock API source, synthetic markers and request IDs. The original mock device endpoints remain available for fixture demonstrations, but Inventory never calls them.
- Backup History has 36 initial snapshots across 12 fixture devices. Device filtering, View and device-detail Backup history use these HTTP endpoints.
- Compare fetches selected Node snapshots; the unchanged client-side line-diff algorithm supports Side-by-side, Unified, Changes only, selectors and Swap.
- Create demo backup is separate browser-memory state, explicitly NOT persisted. It does not write to either API or database.
- Dashboard counts/topology, the labeled sample Configuration tab and prewritten risk findings remain synthetic. Reviewing a finding does not create authorization or security acceptance.

## Minimal validation and limits

Node syntax checks, a build of only cmd/inventory-demo, HTTP smoke checks, browser Inventory/detail interaction, unauthorized reads, and a controlled Go stop/start with Retry cover this milestone. These are not Phase E, race, capacity, performance or release validation.

No real SSH, external device addresses, credential handling, production database, discovery or configuration changes are enabled.

Known CI issue reported by independent review: **GO-2026-6809** in **Go 1.26.9**, fixed in **Go 1.26.10**. The local demo uses the verified 1.26.10 toolchain. Repository-wide go.mod/CI/Docker correction remains a separate task; this change does not alter those files or disable govulncheck.

**Phase E: DEFERRED / NOT ACCEPTED. Production release: BLOCKED.** Historical E1 Go cases passed their executed scenarios, but Controller acceptance failed. No merge/deployment, Phase F or Task #7 acceptance is granted.
