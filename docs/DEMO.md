# Aegis synthetic local demo

The **Go → Node → Vue** demo is available on `main` following [PR #17](https://github.com/HosseinStan/Aegis/pull/17), alongside the Go API and the existing standalone HTML/Node reference preview. The local demo remains **pre-release evaluation software**, separate from the production SSH application.

> **NO LIVE DEVICE ACCESS. RISK NOT EVALUATED. Phase E DEFERRED / NOT ACCEPTED; production use BLOCKED.** Neither screenshots nor green CI establish hardware or production security acceptance.

## What it demonstrates

- Dashboard showing **4 disabled mock devices**, **12 synthetic saved snapshots** and **3 device groups**.
- Read-only device inventory and profile pages, with saved running configurations displayed from disposable Go backup history.
- Backup History (3 saved versions per device), SHA-256 metadata, timestamped UUIDs, and deep links to the exact saved content.
- Native Go `BackupService.Diff` comparison: Side-by-side, Unified, Changes Only, Swap and same-snapshot `0/0` comparisons.
- Change Review that describes saved differences only; **Viewed** is local component state, **never an approval, persistent record or security score**.

The demo uses a dedicated `cmd/inventory-demo` Go entrypoint, synthetic fixture generation, disposable SQLite, and a localhost Node bridge to Vue. It does **not** start the production `cmd/server` connector, open a Telnet route, expose arbitrary CLI commands or connect to Cisco devices.

## Run locally on Windows

1. Follow the maintained [Vue MVP instructions](../apps/aegis-ui/vue-mvp/README.md) for the pinned Node, pnpm and Go versions, download verification, frozen dependencies, and an isolated external tools JSON. Use **PowerShell 7.3+**.
2. Clone this repository separately from any real Aegis workspace; avoid committing local credentials, backups or network data.
3. Start the PowerShell launcher with the external config:

```powershell
& ".\apps\aegis-ui\vue-mvp\start-local.ps1" -ToolsFile "C:\path\to\local-tools.json"
```

4. Confirm `AEGIS LOCAL DEMO — READY`, `Go API: Healthy`, `Connector: Mock only`, `Dataset: Disposable synthetic SQLite`, and `Device access: Disabled`. Open the printed **127.0.0.1** URL(s). End with `Q` or `Ctrl+C`; the shell prints `AEGIS LOCAL DEMO — STOPPED`.

The Windows launcher reuses independently verified Go/Node/pnpm tools and caches: it **does not download or install them**. Do not remove its hash or frozen-lock checks, expose the service on a public network, or replace synthetic data with corporate configuration dumps.

## Verification evidence

- [Latest Go, Vue, Windows integrity and synthetic E2E CI](https://github.com/HosseinStan/Aegis/actions/runs/43597913507) completed green on the integrated PR head; the post-merge `main` CI must also pass before cutting a tag.
- Automated Go → Node → Vue integration validates four disabled mock devices, 12 unique saved backup IDs, HTTP 401 for unauthenticated direct Go inventory access, and HTTP 405 for blocked writes.
- The maintainer demonstrated the Windows launcher and localhost Overview in Chrome, and supplied screenshots of device search, saved configuration, Backup History filtered to **3** records, native `+2/-2` side-by-side and Unified comparison, Changes Only, same-snapshot `0/0`, and Change Review. Those screenshots are maintained **outside** the public repository at this point.
- Manual stop output reported `AEGIS LOCAL DEMO — STOPPED`. A dedicated fresh-restart/port-reuse smoke and manual Swap/Viewed-reset observation were not independently supplied; the corresponding unit-level behavior is covered by tests.

## Release and safety limits

- No Go/Vue packaged executable is signed or provided; GitHub source archives, if a pre-release is published, are source only.
- Lab device manager in [PR #12](https://github.com/HosseinStan/Aegis/pull/12) is not merged. Change-risk classification in [PR #9](https://github.com/HosseinStan/Aegis/pull/9) remains deferred.
- This repository contains both real-device API code and a **separate synthetic-only demo**. Do not confuse the demo's passing tests with approval to configure, scan or audit production infrastructure.
- Known limitations: security acceptance, CLI compatibility across device models, production identity integration and backup-at-rest encryption remain unfinished.
