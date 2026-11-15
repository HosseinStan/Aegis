# Aegis public roadmap (early-stage)

This roadmap distinguishes implemented baseline code from work preserved in draft pull requests. It is not a promise of dates or production readiness.

## Baseline on `main`

- Read-only Cisco IOS/IOS XE access through the Go API, manually triggered configuration backups, history, diffs, and authenticated audit records.
- Authentication, local credential protection, API documentation, automated Go checks, and container build configuration.

## Current milestone: shareable local developer evaluation

- The Go 1.26.11 security update is merged to `main` via [PR #16](https://github.com/HosseinStan/Aegis/pull/16); keep this security baseline in future branches.
- **Completed:** the synthetic Go/Vue MVP and dedicated Windows launcher were merged via [PR #17](https://github.com/HosseinStan/Aegis/pull/17); a maintainer demonstrated the local Overview, inventory, backups, comparison and change-review screens.
- Validate the lab device manager in stacked [PR #12](https://github.com/HosseinStan/Aegis/pull/12) independently of corporate networks, maintaining explicit SSH allowlists and host-key verification.
- Publish reproducible synthetic fixtures, a demonstration walkthrough, and an honest pre-release once the necessary checks pass.

## Open-source readiness gates

1. **Maintainer confirmed** personal ownership of Aegis; check attribution for any third-party or generated artifacts before publishing releases.
2. **Published on `main`:** Apache License 2.0 and `NOTICE` via [PR #16](https://github.com/HosseinStan/Aegis/pull/16).
3. **In progress:** Audit all public branches/history and synthetic fixtures for secrets, confidential topology, and third-party materials; existing spot checks are not a comprehensive audit.
4. **Enabled and user-verified:** GitHub private vulnerability reporting. Keep `SECURITY.md` and public issue intake aligned.
5. **Planned:** Invite genuine outside tests and feedback without implying existing adoption.

## Later: production-readiness work

- Device-specific CLI compatibility and least-privilege acceptance on approved lab hardware.
- Complete deferred Phase E security/performance/release gates.
- Production identity integration, at-rest backup protection, and key-management hardening.
- Scheduled backups and broader inventory features.

**Release status:** Phase E is DEFERRED / NOT ACCEPTED. Production release is BLOCKED. [PR #9](https://github.com/HosseinStan/Aegis/pull/9) is a preserved development snapshot, not a verified production feature.
