# Aegis Vue MVP slice

Additional Vue development frontend for Dashboard, Inventory, basic Go device profiles, native Saved Configuration, Backup History, Configuration Compare and Change Review. The standalone [Node/HTML preview](../preview/README.md) is retained unchanged as a reference. No office Vue application source was supplied or assumed.

**SYNTHETIC / DEMO — RISK NOT EVALUATED. Phase E deferred / not accepted. Production blocked.**

## Dependency preparation

The manifest pins Vue **3.5.44**, Vite **8.3.5** and @vitejs/plugin-vue **6.0.10**; pnpm-lock.yaml records the resolved dependency graph and integrity values. Local validation uses Node **24.22.1** and pnpm **11.25.0**. There is no router, UI kit, state library, Axios or test framework dependency.

Use project-local dependencies and retain the lockfile. In controlled environments, obtain approval before downloading missing packages. No system toolchain or unrelated office application source is needed.

```text
pnpm install --frozen-lockfile --ignore-scripts --store-dir=<DemoRoot>/pnpm-store --cache-dir=<DemoRoot>/pnpm-cache
```

The private external store/cache avoids modifying an existing dependency cache. Once populated, add --offline for a network-free install. pnpm-workspace.yaml disables lifecycle scripts and automatic peer installation; dependency mismatches fail instead of silently installing during run commands. If pnpm is not on PATH, use `node <existing-pnpm-package>/bin/pnpm.cjs` in place of `pnpm`.

## Launch

### One-command Windows workspace

Use **PowerShell 7.3+ on Windows x64**, an existing Node 22.13+ installation, the pinned pnpm 11.25.0, installed pinned Vue dependencies and the already verified Go 1.26.10 Windows SDK. Nothing is installed or downloaded by this launcher; system PATH is never changed.

Copy [local-tools.example.json](local-tools.example.json) **outside Git** and fill in the six existing absolute local paths. It contains tool/cache locations only; never put passwords, tokens or application configuration in it. `moduleCacheSeed` is an already available Go module cache containing the versions in go.sum. `runtimeRoot` must be fresh and outside every Git checkout; subsequent launches reuse its matching ownership marker.

From a terminal, run one command (replace the three absolute paths with your existing PowerShell, clone and tools file):

```powershell
& 'C:/Tools/PowerShell/pwsh.exe' -NoProfile -File 'C:/AegisDemo/repo/apps/aegis-ui/vue-mvp/start-local.ps1' -ToolsFile 'C:/AegisDemo/local-tools.json'
```

Click the actual Vue HTTP URL printed after **AEGIS LOCAL DEMO — READY**. Keep this terminal open. **Ctrl+C or Q** stops this session's Vue, Node supervisor and Go child; it never searches for or kills other processes by name or port. Existing occupied ports are left alone: all three services bind to 127.0.0.1 using OS-allocated free ports. Do not open index.html through file://.

- The PowerShell entrypoint compiles a small in-memory .NET Windows Job host. The trusted Node helper waits on stdin until it is assigned to that unnamed, non-breakaway Job. All later descendants inherit it. Normal stop requests graceful shutdown; a 20-second stop deadline and kill-on-Job-close reap only this session's remaining descendants. If Job assignment fails, startup fails without a containment fallback. No administrator installation is needed. See [Windows Job Objects](https://learn.microsoft.com/en-us/windows/win32/procthread/job-objects).
- The existing run-go-demo.mjs is reused unchanged. Its credentials/bootstrap and JWT remain private to its existing pipes/bridge. Only the actual public Node origin is supplied to Vite. Existing Vite routes, GET restrictions and CSP remain unchanged; inherited VITE_* variables and dotenv loading are disabled for this isolated launch.
- Node/pnpm versions, exact Vue package pins and the installed lockfile receipt are checked. The approved Go executable SHA-256, official cached ZIP SHA-256 and SDK version are checked against the existing 1.26.10 receipt. The SDK and package contents must come from those trusted local preparations; this developer launcher is not a hostile-user sandbox or a full installation attestation.
- Go tools use process-scoped GOTOOLCHAIN=local, GOPROXY=off, -mod=readonly, CGO_ENABLED=0 and private external caches. The dependency seed is copied, never modified. Interrupted copies stay unpublished in their session directory. The fixed target is **./cmd/inventory-demo**, never cmd/server. First build verifies cached modules; later launches reuse the binary only when its SHA-256 receipt, toolchain, build flags and current application Go/embed/module inputs agree. Cache drift fails rather than silently restoring files.
- Binaries, Go/Vite caches, temporary files and fresh disposable databases are under the external owned runtime. Session launch.json contains public origins, PIDs and lifecycle/build-reuse status only. No credentials or private bootstrap are written. Retained session data is not automatically deleted.
- READY requires real Go readiness, authenticated bridge reads of four disabled mock devices and twelve unique synthetic snapshots, a served Vue entrypoint, and matching actual snapshot UUIDs through Vue's same-origin proxy. A startup/service failure stops the session without fixture fallback.
- One active launch is allowed per runtime root. A second concurrent launch fails with SESSION_LOCKED and leaves the first alone. After a hard host termination, inspect any stale lock manually or choose a fresh external runtime; the launcher never kills a recorded PID or auto-cleans an unknown directory.

Missing tools, packages, offline dependencies or an unsafe/foreign runtime produce a specific error. Prepare missing dependencies manually from the existing frozen offline cache using the instructions above; the launcher deliberately does not install them. Run `pnpm test:launcher` for focused contract/cache tests. Ctrl+C, Q, occupied-port isolation, failure cleanup and binary reuse also require the focused Windows smoke check.

**PR #11 is not merged or cherry-picked here.** PR #10's Go/CI/Docker pins remain unchanged; using local Go 1.26.10 does not establish that PR #10 passed GitHub CI. Phase E remains deferred / not accepted and production blocked.

### Per-service launch for troubleshooting

Start the unchanged isolated Go/Node supervisor from the separate development clone, using the existing verified Go 1.26.10 demo binary and a fresh external disposable parent:

```text
node <clone>/apps/aegis-ui/preview/run-go-demo.mjs --go-binary=<DemoRoot>/aegis-inventory-demo.exe --run-parent=<DemoRoot>/disposable
```

Keep that terminal open. In a second terminal, from this directory, set only the public Node preview origin printed by the supervisor:

```powershell
$env:AEGIS_PREVIEW_ORIGIN='http://127.0.0.1:<printed-preview-port>'
pnpm dev
```

Open the Vite HTTP URL with #/overview or #/devices. Do not open index.html with file://. Vite binds to 127.0.0.1:5173 with strictPort; choose an explicit unused port if necessary. `pnpm build` makes a local bundle, and `pnpm preview` serves it on loopback port 5174 using the same configured bridge. Neither command deploys anything.

When preserving a previous Vue demo on port 5173, run `pnpm dev --port 5175` for the new isolated clone instead of stopping that demo.

## Data and frontend boundaries

Browser HTTP GET → Vite same-origin proxy → existing authenticated Node bridge → existing Go services/repository → disposable SQLite.

- Vue uses Inventory list/profile, backup-history metadata and selected saved content through the existing /api/go/v1/devices routes. It imports no device or backup fixture collection and implements no mock API or Go business logic.
- Dashboard derives counts, coverage, latest snapshot, groups and recent backup links from the actual Go responses. Loading/failure clears displayed records; Retry never substitutes fixtures.
- Inventory has reactive search/group filters and device navigation. Basic profiles show only native Go fields, with absent software/serial information explicitly marked unavailable.
- All core read-only screens, including Change Review, open natively inside Vue through the Sidebar, Dashboard and device shortcuts. The complete standalone frontend, including its modal Snapshot Viewer, remains available through the explicit full-preview link.
- Vue components and composables own rendering, requests, cancellation and reactive state. The old app.js is not wrapped, imported or copied into the application.
- Only the public Node origin is bundled. Go credentials and tokens remain exclusively in the existing supervisor/bridge process; no browser token, localStorage or sessionStorage is used.
- No SSH, live configuration endpoint, production database, device operation, backup write or discovery is added. The existing mock-only backend, RBAC, seeds and read-only bridge are unchanged.

## Native Saved Configuration

- Open #/devices/:id?tab=configuration. The component reuses the API client and useGoResource; it reads this Go device's saved history and defaults to its newest running snapshot. It does not copy the legacy preview implementation.
- The selected actual UUID is kept in #/devices/:id?tab=configuration&backup=:uuid, including browser Back/Forward and reload. Retry retains it. An expired/cross-device UUID fails without selecting another record; **Refresh versions** explicitly reloads current history and selects its latest version.
- The display always states **Saved configuration — not live device state.** Older selections are explicitly marked. Timestamp (UTC), UUID, SHA-256, UTF-8 byte size and read-only text come from the selected Go result.
- The client validates response/source markers, device/backup IDs, running/demo_seed type and immutable metadata against the history. It also checks received UTF-8 byte length and SHA-256 with browser Web Crypto. Content uses Vue text interpolation, never v-html. Native BackupService.Content still performs its existing integrity verification.
- Loading, empty, 401/403, 404, unavailable and Retry states show no substitute content. Query changes synchronize the Vue route without reloading the device profile or silently re-requesting a failed selection.
- The Vite proxy adds only the anchored /api/go/v1/devices/:id/backups/:uuid content path. A middleware rejects non-GET requests on that path in dev and preview; the existing metadata allowlist and server-side bridge restrictions remain unchanged. No broad /api proxy or new upstream route is added.
- Compare opens native Vue with the selected actual Go UUID and an adjacent saved version.

## Focused checks

Run `pnpm test:config`, `pnpm test:history`, `pnpm test:compare`, `pnpm test:review` and `pnpm build`. In the browser, verify 12 Go backups across four devices, three per device filter and native Vue configuration/Compare/Review links. Compare synthetic revision 2 to revision 3 (+2/-2), identical selections (0/0), exact native Unified text, Side-by-side, Swap and Changes Only. Verify that Viewed resets on ordered pair/device changes and reload. Use only the new supervisor's stop-api/start-api to verify unavailable/Retry, stale UUID 404 and explicit Refresh recovery. Check desktop/mobile layouts, Dashboard/Inventory/profile navigation and the unchanged full preview. These are development smoke checks, not Phase E/E1 or release acceptance.

## Native Backup History

- Open `#/backups` or `#/backups?device=core-sw-01`. The reactive device filter is URL-backed; Back/Forward and reload retain its scope. Requests use the existing API client, cancellation composable and Go Inventory/history endpoints. No new proxy path, backend, database migration or dependency is needed.
- All-device history aggregates current metadata from Go Inventory and each corresponding device history. Filtering reads only the selected device's history while retaining all device options. Dashboard shares the same collection loader. An unknown filter preserves the native device-profile 404; successful but inconsistent metadata is rejected without inventing HTTP status codes.
- Cards show actual device identity, UTC timestamp, UUID, SHA-256 metadata, configuration type, saved byte size and demo seed source. The hash is labeled Go metadata here; opening Saved Configuration separately verifies received content bytes. No snapshot content request is needed to list history.
- View Configuration opens native Vue with the exact device/UUID. Compare opens native Vue with two saved UUIDs from the same device. No legacy fixture or browser-memory backup is mixed into this history.
- Loading and errors clear previous records. Empty, 401/403, 404 and unavailable states do not show partial histories or substitute fixtures. Retry and Refresh history request current records; a Go restart generates new UUIDs. Existing configuration deep links retain their stale UUID and fail 404 until Refresh versions is explicitly chosen.
- This screen has no create/delete action and does not persist review or configuration changes. Tokens remain exclusively in the existing server-side Node bridge.

## Native Configuration Compare

- Open `#/compare`, `#/compare?device=core-sw-01` or an explicit `#/compare?device=:id&left=:uuid&right=:uuid`. Default selections use the current history's previous/latest running snapshots; a single stored version may be selected on both sides. A device change explicitly resets the pair to that device's current versions.
- The client reuses Inventory/history reads and the same validated content loader as Saved Configuration: ownership, requested UUID, synthetic markers, current history membership, immutable metadata, UTF-8 byte size and SHA-256. No fixture content or substitute UUID is accepted.
- Native `GET /api/go/v1/devices/:deviceId/backups/:leftId/diff/:rightId` supplies the authoritative changed flag, added/removed counts and Unified text. The client validates IDs/source/types and consistency with the selected content. `native-diff.js` parses the native patch and checks its hunks, context and count convention against verified snapshots; it does not generate a competing diff or calculate a risk rating. Unsupported or inconsistent patches fail explicitly.
- Side-by-side presents the native hunk ordering and pairs adjacent removed/added blocks positionally. Unchanged gaps come from the same verified contents. **Changes only** filters side-by-side rows. Unified is always the exact native string, including context and whitespace; that filter is disabled in Unified to preserve the response faithfully. Configuration and patch text use Vue interpolation, never `v-html`.
- The existing cancellation/sequence composable prevents late old requests from overwriting new selections. Canonical URLs retain the exact selected UUIDs across navigation/reload. Retry retains explicit selections, including stale 404s after Go restart. **Refresh snapshots** explicitly clears the pair, reloads current history and chooses its current previous/latest IDs. Loading/errors clear rendered content.
- Vite adds only an anchored native Diff path, retaining all previous narrow proxy routes. The same middleware rejects non-GET Diff and snapshot-content requests before forwarding, in both dev and preview. No broad `/api` proxy, Go/RBAC change or browser bearer token is introduced.
- Empty, 401/403, 404 and unavailable states are explicit. This is saved synthetic change evidence, not live state, a security assessment, approval or persisted audit record. The full standalone frontend and backend are unchanged.

## Native Change Review

- Open `#/review`, `#/review?device=core-sw-01` or `#/review?device=:id&left=:uuid&right=:uuid`. Device Details opens the selected device's review; Compare opens the exact current ordered pair. Review links back to native Compare and each selected Saved Configuration without changing their UUIDs.
- `ChangeReviewView.vue` and `ConfigurationCompareView.vue` compose one `SavedComparisonWorkspace.vue`. The existing selectors, Swap, cancellation, route synchronization, error states, native diff presentation and metadata renderer were extracted from Compare instead of copied into a second implementation. Both use the unchanged `getConfigurationComparison` and `native-diff.js` validation/presentation.
- Review shows actual Go identity/group, synthetic disabled mock status, before/after UUIDs and UTC timestamps, native changed/count results, SHA-256 verification evidence and HTTP request IDs. Loading/errors clear old evidence. No inventory/history fixture is substituted.
- The prominent **RISK NOT EVALUATED — DEMO / UNVERIFIED** notice applies even while loading or unavailable. This screen visualizes saved configuration differences only. It produces no severity, compliance, vulnerability, approval or security recommendation result. PR #9 is not integrated.
- **Viewed in this demo — not approved or persisted** is an optional `ref` scoped to the component. `useDemoViewed` resets it on any change to the exact device/before/after tuple, including Swap and returning to a previous pair. Reload/remount creates a fresh false value. It uses no storage, request or backend audit/review record; presentation changes for the same pair retain it.
- Retry preserves explicit UUIDs. After a Go restart, stale IDs return native 404 with no replacement content. Refresh snapshots explicitly reloads the current history and selects current previous/latest versions. Empty inventory/history, 401/403 and unavailable handling share the Compare implementation.
- No new API endpoint, proxy route, backend service, migration, permission, dependency or write operation is introduced. Go/Node and the standalone reference preview remain unchanged.
