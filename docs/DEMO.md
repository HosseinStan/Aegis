# Evaluating Aegis without a physical Cisco switch

Aegis has two distinct public development states. Avoid confusing the stable `main` API with experimental Vue features in draft PRs.

## Current main branch

- The Go API has automated integration tests using an in-process mock connector, so its core read-only API and backup behavior can be reviewed without real equipment.
- See [Aegis API verification](../apps/aegis-api/README.md) and the CI workflow. This integration candidate uses the CI-tested Go 1.26.11 toolchain baseline; keep the demo toolchain aligned with this fixed version.
- The development test console is not an isolated synthetic-device Vue demo. It is not a reason to connect real switches or deploy to production.

## Synthetic-device Go-backed UI (draft)

- [PR #10](https://github.com/HosseinStan/Aegis/pull/10) contains a standalone Node/HTML preview and a Vue workspace. Its seed provides **four disabled synthetic devices and twelve synthetic saved snapshots** through a disposable SQLite database.
- [Preview setup](https://github.com/HosseinStan/Aegis/blob/codex/aegis-mvp-preview-20261028/apps/aegis-ui/preview/README.md) documents the isolated Go/Node preview.
- [Vue setup](https://github.com/HosseinStan/Aegis/blob/codex/aegis-mvp-preview-20261028/apps/aegis-ui/vue-mvp/README.md) documents the Windows launcher and offline dependency requirements.
- [PR #12](https://github.com/HosseinStan/Aegis/pull/12) adds a separate localhost-only lab manager; it has not been physically tested on switches.

These are development branches, **not merged releases**. The current launcher expects its documented, externally prepared Go/Node/pnpm toolchains and caches. Its earlier Go pins must be reconciled with the security patch before a publishable demo/release. Do not bypass integrity checks, bind a development service to a public interface, use real corporate config dumps or claim production acceptance.

## What a reviewer should see once validated

A dashboard, device inventory, saved configuration, backup history, native comparison and change-review screens. These operate on synthetic records only. Risk classification remains unverified and production release is blocked. A future tagged preview should provide an accurate reproducible launch script and screenshots from that same validated build.
