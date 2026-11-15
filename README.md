# Aegis

**Read-only Cisco configuration visibility, backup history, and change review for infrastructure teams.**

Aegis is an early-stage Go-based infrastructure-management project with a JSON API for approved Cisco IOS/IOS XE devices. It aims to help teams inspect device configurations and understand changes without exposing a generic device command or configuration-write interface.

> **Project status:** Aegis source is offered under **Apache License 2.0** (see [LICENSE](LICENSE) and [NOTICE](NOTICE)). It is an **early-stage, pre-release development project**, not production-approved. Phase E remains deferred / not accepted. The isolated, synthetic Go-backed Vue demo was merged to `main` in [PR #17](https://github.com/HosseinStan/Aegis/pull/17).

## Why Aegis?

Network configuration review is often handled with device-by-device logins or scripts that are difficult to audit. Aegis separates authenticated users and browsers from network-device credentials and SSH access, maintains configuration snapshots, and makes change review repeatable.

## Implemented on `main`

- Authenticated REST API with seeded local JWT roles and permission checks.
- Cisco IOS/IOS XE SSH connector with fixed read operations, strict host-key verification, timeouts, and bounded output.
- Encrypted credential fields (AES-256-GCM); encrypted credentials stay behind the API.
- Configuration reads, manually triggered backups, SHA-256 deduplication, history, downloads, and unified diffs.
- Device registration, audit records, structured logging, health/readiness endpoints, and API documentation.
- Go build, test, race-test, static-analysis, vulnerability-scan, and container-build CI steps.

These features are implemented for a defined milestone; they do **not** imply acceptance for a production environment. Device-specific CLI parsing and the stated security assumptions still require validation.

## Local demonstration on `main`

- A **synthetic, read-only Go → Node → Vue workspace** with Dashboard, Inventory, Saved Configuration, Backup History, side-by-side and unified comparisons, and Change Review is included on `main` via [PR #17](https://github.com/HosseinStan/Aegis/pull/17).
- Demo data comprises **four disabled mock devices and twelve disposable configuration snapshots** (three per device), not live device access. The demo entrypoint is separate from the production Go server.
- The Windows launcher uses independently prepared tools, verified toolchain checksums, frozen pnpm dependencies, and fail-closed lockfile checks. See [demo setup and validation](docs/DEMO.md) and [Vue workspace instructions](apps/aegis-ui/vue-mvp/README.md).

## What is still under development?

| Work | Where to review | Status |
| --- | --- | --- |
| Public synthetic Go/Vue MVP | [PR #17](https://github.com/HosseinStan/Aegis/pull/17) | **Merged on `main`; local evaluation only** |
| Isolated local lab device manager | [PR #12](https://github.com/HosseinStan/Aegis/pull/12) | Draft; **not** merged or hardware-accepted |
| Go 1.26.11 security baseline | [PR #16](https://github.com/HosseinStan/Aegis/pull/16) | Merged; automated checks enabled |
| Deterministic change-risk development snapshot | [PR #9](https://github.com/HosseinStan/Aegis/pull/9) | Deferred; not accepted |

**Phase E remains DEFERRED / NOT ACCEPTED. Production release remains BLOCKED.** A passing CI run is not a substitute for production security and hardware acceptance.

## Architecture at a glance

~~~text
Browser / client
      | authenticated HTTPS / JSON
      v
Aegis API (Go) ----> SQLite metadata and backup history
      |
      +----> encrypted credential store
      |
      +----> approved Cisco IOS/IOS XE devices over SSH
               (fixed read operations, pinned host keys)
~~~

Browsers do not connect directly to switches. There is no Telnet client, arbitrary CLI endpoint, unregistered destination endpoint, or device configuration-write API.

## Try the current API locally

1. Install a compatible Go 1.26 toolchain, and consult the [API setup guide](apps/aegis-api/README.md) for the current version/security requirements.
2. Follow the local setup instructions to generate ignored development-only secrets and configure a trusted SSH `known_hosts` file.
3. From `apps/aegis-api`, run `go run ./cmd/setup-local` where supported. On Windows, use `scripts/development/start-local.cmd` from the repository root.
4. Open the development-only evaluation console at `http://127.0.0.1:8080/test-console/`, or use the authenticated API. Do not enable the test console in production.

The `main` branch pins Go 1.26.11, which was validated by CI. Passing CI does not indicate production security acceptance.

For Docker Compose, authentication, Swagger, and examples see [apps/aegis-api/README.md](apps/aegis-api/README.md). To assess API behavior without a physical switch, start with the mocked connector integration tests. The synthetic Go-backed Vue demonstration is included on `main`, documented in [the demo guide](docs/DEMO.md); it is not a tagged or production release yet.

## Try the synthetic Vue demo locally

This is an **offline-capable, localhost-only synthetic evaluation workflow**, not a connection to real switches.

1. Follow [the Vue MVP prerequisites](apps/aegis-ui/vue-mvp/README.md): Windows x64, PowerShell 7.3+, Node.js 22.13+ (the verified example uses Node 24.22.1), pnpm 11.25.0, and Go 1.26.11. Download and verify tools **outside** the repository as documented; the launcher never installs or downloads tools or weakens integrity checks.
2. Prepare the frozen Vue dependencies and verified Go module cache outside live infrastructure; copy and edit [the example tools configuration](apps/aegis-ui/vue-mvp/local-tools.example.json) into an external `local-tools.json` with absolute local paths.
3. In PowerShell 7, from the repository root, run the launcher with that external configuration:

```powershell
& ".\apps\aegis-ui\vue-mvp\start-local.ps1" -ToolsFile "C:\path\to\local-tools.json"
```

4. Only after `AEGIS LOCAL DEMO — READY` appears, open the **printed** localhost Vue URL. Four disabled mock devices and 12 synthetic backups should appear. Press `Q` or `Ctrl+C` in that terminal to stop the private demo session.

See [verification and limitations](docs/DEMO.md) for the observed Windows walkthrough and the CI-tested Go → Node → Vue flow. No SSH connection or real network configuration is required.

## Development checks

~~~sh
make check
~~~

The root Makefile runs Go tests, `go vet`, and a build for the API. GitHub Actions also runs additional formatting, race, static-analysis, vulnerability, and Docker checks. See [.github/workflows/ci.yaml](.github/workflows/ci.yaml).

## Security and operational limitations

- Use least-privilege read-only device credentials and independently verified SSH host keys.
- Never commit real device configurations, credentials, keys, databases, or network topology.
- Backup files are not independently encrypted at rest in the current milestone.
- Production identity integration, key rotation, and device-model acceptance remain incomplete.

Read [SECURITY.md](SECURITY.md), [API security assumptions](apps/aegis-api/SECURITY.md), and [device preparation](docs/operations/cisco-preparation.md) before attempting a deployment.

## Contributing and roadmap

See [CONTRIBUTING.md](CONTRIBUTING.md) and the [roadmap](docs/ROADMAP.md). Contributions are welcome under the published license, subject to the security requirements and review process in CONTRIBUTING.md. Do not include operational or confidential device information. Security vulnerabilities must not be disclosed in public issues.

## License

Aegis is licensed under the **Apache License, Version 2.0**. Read the full [LICENSE](LICENSE) and the copyright [NOTICE](NOTICE). Third-party components remain subject to their respective licenses.
