# Aegis Vue MVP slice

Additional Vue development frontend for Dashboard, Inventory, basic Go device profiles and native Saved Configuration. The standalone [Node/HTML preview](../preview/README.md) is retained unchanged. No office Vue application source was supplied or assumed.

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
- Saved Configuration opens natively inside Vue, including Dashboard snapshot links. Backup History, the modal Snapshot Viewer, native Compare and Change Review remain in the full existing preview. Their links explicitly open it separately, using the same Node origin and current Go UUIDs.
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

Run `pnpm test:config` for the focused Node built-in contract checks and `pnpm build`. In the browser, open current/older saved configurations, inspect metadata and Compare links, and use only the new supervisor's stop-api/start-api to verify stale UUID 404, Retry retention and Refresh recovery. Check desktop/mobile layouts, Dashboard/Inventory/profile navigation and the unchanged full preview. These are development smoke checks, not Phase E/E1 or release acceptance.
