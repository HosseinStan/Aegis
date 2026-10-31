# Standalone Aegis MVP preview

**DEMO — SYNTHETIC DATA — UNVERIFIED RISK FINDINGS**

An isolated, zero-dependency browser preview of the Aegis workflow. It is a lightweight fallback, **not the final Vue implementation**. Vue/Vite migration is deferred until dependencies are available. The existing [Vue integration architecture](../../../docs/vue-integration.md) remains the intended integration path.

## Run locally

Use an existing Node.js installation. No package installation, environment variables, credentials, or database setup is required.

```sh
cd apps/aegis-ui/preview
node server.mjs
```

Open the loopback URL printed by the process. The server binds only to `127.0.0.1` on an available port; the current URL and PID are written to the ignored `runtime.json`. Stop this foreground process with Ctrl+C.

An optional `node server.mjs --port=4318` selects a fixed loopback port, useful for a controlled stop/restart and Retry check. The default remains an available port; the bind address cannot be changed.

## Inventory HTTP Mock API

- `GET /api/mock/v1/devices` returns the synthetic Inventory collection.
- `GET /api/mock/v1/devices/:id` returns a synthetic device profile, or JSON `404 DEVICE_NOT_FOUND`.
- Responses use `{data, meta}` with `source: "Local Mock API"`, `synthetic: true`, and a fresh response request ID. API errors use `{error, meta}`. Only GET is accepted.
- The server projects device metadata from the existing reviewed fixture. It does not return configuration text, backups or risk findings through these endpoints.

Inventory, its filters/export, and device-profile facts use browser `fetch` requests to these endpoints. A visible **Local Mock API** indicator shows the actual HTTP status, request URL, elapsed time and response request ID. Refresh and Retry issue new requests; failed requests never fall back to the static Inventory collection.

Loading, empty collection, no search matches, device-not-found and API-error states are explicit. To inspect the unavailable state, stop only this local preview process while leaving the loaded Inventory tab open and press Refresh inventory. Restart it on the same port, then press Retry.

## Backup and snapshot HTTP Mock API

- GET /api/mock/v1/devices/:id/backups returns synthetic snapshot metadata, without configuration text.
- GET /api/mock/v1/devices/:id/backups/:backupId returns the selected synthetic snapshot including its configuration text.
- Both follow the same response envelope, GET-only policy and request-ID headers as Inventory. Every backup carries device_id, source SYNTHETIC_DEMO and synthetic true. Configuration text must carry the reviewed synthetic banner.
- Unknown devices return JSON 404 DEVICE_NOT_FOUND; unknown or cross-device snapshot IDs return 404 BACKUP_NOT_FOUND.

Backup History fetches metadata over HTTP. All-devices history combines the 12 per-device responses; a device filter fetches just that device's history. View fetches a snapshot through its endpoint. Device-detail Backup history uses the same HTTP path.

Configuration Compare first fetches history for the version selectors, then fetches both selected HTTP snapshots. The existing browser-side line-diff algorithm calculates additions/removals. Side-by-side, Unified, Changes only, selectors and Swap remain available. Source details expose the actual GET paths and response request IDs.

History, snapshot viewing and comparison show loading, empty, 404 and API-error states with Retry. Failed reads do not substitute static configuration text. To inspect an unavailable state, leave the page loaded, stop only its preview process, then press Refresh history or change a comparison selector. Restart on the same loopback port and press Retry. A missing snapshot can be inspected with #/compare?device=core-sw-01&left=missing&right=core-sw-01-v3.

**Create demo backup is separate:** it copies synthetic fixture text into this tab's memory. These copies appear under Browser-memory snapshots and are labeled NOT persisted. They can be viewed/compared locally, with explicit BROWSER MEMORY version labels. They never issue an API write or change HTTP history; reloading discards them. Dashboard and risk review remain fixture demonstrations. The device's Configuration tab retains its labeled sample fixture text.

## Explore

- **Overview:** dashboard, 7/30-day backup chart, clickable sample topology.
- **Device inventory:** HTTP-fetched records; search names, addresses, sites and models; filter by site or impact.
- **Device details:** HTTP-fetched profile and backup history, with sample configuration and risk-review navigation.
- **Configuration backups:** 36 initial HTTP snapshots plus a separate browser-memory copy section.
- **Compare configurations:** HTTP-fetched snapshot text, device/version selectors, side-by-side and unified line differences, changes-only filtering and snapshot swapping.
- **Risk reviews:** prewritten sample findings, impact filters, expandable explanations and in-memory demo review tracking. UNKNOWN is distinct from LOW.

All 12 device records, configurations, topology and findings are synthetic. Addresses use documentation ranges. Configuration differences are calculated from the fixture text; risk findings are **not** generated by a verified risk engine or API. Demo review actions create no authorization or audit record. Reloading resets in-memory snapshots and reviews.

No actual SSH, device operation, authenticated Aegis backend API, production database or configuration write is enabled. The connection button is deliberately disabled. The local HTTP Mock API is real HTTP communication with synthetic data; authenticated backend integration remains future work.

## Validation and status

The source can be checked without a build:

```sh
node --check server.mjs
node --check public/app.js
```

Browser smoke checks cover the core navigation, search/detail flow, backup history, interactive configuration comparison and demo risk review. These checks are not security, capacity or release acceptance.

Phase E is **DEFERRED / NOT ACCEPTED**. Historical E1 Go cases passed their executed scenarios, but Controller acceptance failed. Production release remains **BLOCKED**. Phase F and Task #7 gain no acceptance or authorization. This draft does not authorize merge or deployment.

Known CI blocker, reported in the independent review: govulncheck identifies **GO-2026-6809** in **Go 1.26.9**, with a fix available in **Go 1.26.10**. Record this for a separate focused toolchain fix. This MVP change does not alter CI, disable the scanner or upgrade a toolchain.

