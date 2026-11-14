# Aegis

**Read-only Cisco configuration visibility, backup history, and change review for infrastructure teams.**

Aegis is an early-stage Go-based infrastructure-management project with a JSON API for approved Cisco IOS/IOS XE devices. It aims to help teams inspect device configurations and understand changes without exposing a generic device command or configuration-write interface.

> **Project status:** Source code is publicly viewable, but the current `LICENSE` is a placeholder and grants **no permission to redistribute or reuse the code**. Aegis is **not yet an open-source release**. Legal ownership and licensing must be resolved before distributing it as OSS. Production use is not approved.

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

## What is still under development?

| Work | Where to review | Status |
| --- | --- | --- |
| Go-backed Vue dashboard, inventory, backups, and change review | [PR #10](https://github.com/HosseinStan/Aegis/pull/10) | Draft; not on `main` |
| Isolated local lab device manager | [PR #12](https://github.com/HosseinStan/Aegis/pull/12) | Draft; stacked on PR #10 |
| Go toolchain security patch | [PR #11](https://github.com/HosseinStan/Aegis/pull/11) | Draft; review before integration |
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

For Docker Compose, authentication, Swagger, and examples see [apps/aegis-api/README.md](apps/aegis-api/README.md). To assess API behavior without a physical switch, start with the mocked connector integration tests. The richer synthetic-device Vue demo is documented in the draft PRs above, not released on `main`.

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

See [CONTRIBUTING.md](CONTRIBUTING.md) and the [roadmap](docs/ROADMAP.md). External code contributions cannot be accepted under an open-source license until the ownership and licensing decisions are resolved. Security vulnerabilities must not be disclosed in public issues.

## License

**No open-source license has been granted yet.** The current [LICENSE](LICENSE) file explicitly reserves redistribution rights. Do not assume public GitHub visibility permits copying, modification, or distribution.
