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

These are development branches, **not merged releases**. The integration candidate uses the current Go 1.26.11 base and has separate Go/Vue checks, but still requires a manual local browser smoke run. The launcher expects externally prepared and verified Go/Node/pnpm toolchains and caches. Do not bypass integrity checks, bind a development service to a public interface, use real corporate config dumps or claim production acceptance.

## What a reviewer should see once validated

A dashboard, device inventory, saved configuration, backup history, native comparison and change-review screens. These operate on synthetic records only. Risk classification remains unverified and production release is blocked. A future tagged preview should provide an accurate reproducible launch script and screenshots from that same validated build.

### Synthetic Vue demo integration candidate

[PR #17](https://github.com/HosseinStan/Aegis/pull/17) is an independent development branch based on the Apache-2.0 and Go 1.26.11 main baseline. It carries the isolated synthetic Go/Node/Vue demo from PR #10 but leaves the older PR #10/#12 branches unchanged. The Go application does not connect to switches in this demo entrypoint.

**Go 1.26.11 receipt verified in GitHub Windows CI:** official archive SHA-256 `71ae086f0137e665f855a811cc119152b6a783e0bbc1b906e709f46d80141eb0`, and contained `go.exe` SHA-256 `c0ea949d000a24883f37b1dd2db794a9fb2b824fbcff11facb469615a50ca6af`. The Windows one-command launcher is now pinned to these verified values, so it fails closed on any mismatch. The full launcher and browser flow still require a real Windows smoke run and are **not yet approved as a release**. The CLI-based isolated preview remains a separate evaluation path. No public deployment or production use is authorized.
