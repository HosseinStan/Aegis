# Aegis Vue MVP slice

Additional Vue development frontend for Dashboard, Inventory, basic Go device profiles, native Saved Configuration and Backup History. The standalone [Node/HTML preview](../preview/README.md) is retained unchanged. No office Vue application source was supplied or assumed.

**SYNTHETIC / DEMO — RISK NOT EVALUATED. Phase E deferred / not accepted. Production blocked.**

## Dependency preparation

The manifest pins Vue **3.5.44**, Vite **8.3.5** and @vitejs/plugin-vue **6.0.10**; pnpm-lock.yaml records the resolved dependency graph and integrity values. Local validation uses Node **24.22.1** and pnpm **11.25.0**. There is no router, UI kit, state library, Axios or test framework dependency.

Use project-local dependencies and retain the lockfile. In controlled environments, obtain approval before downloading missing packages. No system toolchain or unrelated office application source is needed.

```text
pnpm install --frozen-lockfile --ignore-scripts --store-dir=<DemoRoot>/pnpm-store --cache-dir=<DemoRoot>/pnpm-cache
```

The private external store/cache avoids modifying an existing dependency cache. Once populated, add --offline for a network-free install. pnpm-workspace.yaml disables lifecycle scripts and automatic peer installation; dependency mismatches fail instead of silently installing during run commands. If pnpm is not on PATH, use `node <existing-pnpm-package>/bin/pnpm.cjs` in place of `pnpm`.

## Launch

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
- Saved Configuration and Backup History open natively inside Vue, including Sidebar, Dashboard and device shortcuts. The modal Snapshot Viewer, native Compare and Change Review remain in the full existing preview. External links use the same Node origin and current Go UUIDs.
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
- Compare opens the full Node preview with the selected actual Go UUID and an adjacent saved version. Compare is not migrated into Vue in this milestone.

## Focused checks

Run `pnpm test:config`, `pnpm test:history` and `pnpm build`. In the browser, verify 12 Go backups across four devices, three per device filter, actual Vue configuration links and external Compare links. Use only the new supervisor's stop-api/start-api to verify unavailable/Retry, stale configuration UUID 404 and explicit history Refresh recovery. Check desktop/mobile layouts, Dashboard/Inventory/profile navigation and the unchanged full preview. These are development smoke checks, not Phase E/E1 or release acceptance.

## Native Backup History

- Open `#/backups` or `#/backups?device=core-sw-01`. The reactive device filter is URL-backed; Back/Forward and reload retain its scope. Requests use the existing API client, cancellation composable and Go Inventory/history endpoints. No new proxy path, backend, database migration or dependency is needed.
- All-device history aggregates current metadata from Go Inventory and each corresponding device history. Filtering reads only the selected device's history while retaining all device options. Dashboard shares the same collection loader. An unknown filter preserves the native device-profile 404; successful but inconsistent metadata is rejected without inventing HTTP status codes.
- Cards show actual device identity, UTC timestamp, UUID, SHA-256 metadata, configuration type, saved byte size and demo seed source. The hash is labeled Go metadata here; opening Saved Configuration separately verifies received content bytes. No snapshot content request is needed to list history.
- View Configuration opens native Vue with the exact device/UUID. Compare opens the unchanged full preview with two saved UUIDs from the same device. No legacy fixture or browser-memory backup is mixed into this history.
- Loading and errors clear previous records. Empty, 401/403, 404 and unavailable states do not show partial histories or substitute fixtures. Retry and Refresh history request current records; a Go restart generates new UUIDs. Existing configuration deep links retain their stale UUID and fail 404 until Refresh versions is explicitly chosen.
- This screen has no create/delete action and does not persist review or configuration changes. Tokens remain exclusively in the existing server-side Node bridge.
