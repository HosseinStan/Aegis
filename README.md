# Aegis

**Read-only Cisco configuration visibility, backup history, and change review for infrastructure teams.**

Aegis is an early-stage Go-based infrastructure-management project with a JSON API for approved Cisco IOS/IOS XE devices. It aims to help teams inspect device configurations and understand changes without exposing a generic device command or configuration-write interface.

> **Project status:** Aegis remains pre-release and **production use is not approved**. The maintainer has selected Apache-2.0 in [draft PR #15](https://github.com/HosseinStan/Aegis/pull/15), but the default branch still contains a placeholder license; this is **not yet a licensed open-source release**. Phase E remains deferred.

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
| Go 1.26.11 security update | [PR #14](https://github.com/HosseinStan/Aegis/pull/14) | Draft; CI passed; supersedes outdated Go 1.26.10 proposal in PR #11 |
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

The original Go 1.26.9 pin on `main` has known standard-library advisories; prefer the validated Go 1.26.11 patch in draft PR #14 and do not treat baseline CI as production approval.

For Docker Compose, authentication, Swagger, and examples see [apps/aegis-api/README.md](apps/aegis-api/README.md). To assess API behavior without a physical switch, start with the mocked connector integration tests. The richer synthetic-device Vue demo is documented in [the demo status guide](docs/DEMO.md) and the draft PRs above, not released on `main`.

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

See [CONTRIBUTING.md](CONTRIBUTING.md) and the [roadmap](docs/ROADMAP.md). A licensing change is approved and proposed in draft PR #15, but the default branch is still unlicensed; external redistribution and code contributions must wait for publication of the license and contribution policy. Security vulnerabilities must not be disclosed in public issues.

## License

The maintainer has chosen **Apache License 2.0**, proposed with a copyright notice in [draft PR #15](https://github.com/HosseinStan/Aegis/pull/15). The default branch's existing [LICENSE](LICENSE) is still a restrictive placeholder until that PR is merged. Public visibility does not imply redistribution rights. This section must be updated after the license merge.
