# Aegis public roadmap (early-stage)

This roadmap distinguishes implemented baseline code from work preserved in draft pull requests. It is not a promise of dates or production readiness.

## Baseline on `main`

- Read-only Cisco IOS/IOS XE access through the Go API, manually triggered configuration backups, history, diffs, and authenticated audit records.
- Authentication, local credential protection, API documentation, automated Go checks, and container build configuration.

## Next: shareable local developer evaluation

- Review and integrate the Go 1.26.10 security update in [PR #11](https://github.com/HosseinStan/Aegis/pull/11) once validated.
- Reconcile and validate the synthetic-device Go/Vue MVP in [PR #10](https://github.com/HosseinStan/Aegis/pull/10).
- Validate the lab device manager in stacked [PR #12](https://github.com/HosseinStan/Aegis/pull/12) independently of corporate networks, maintaining explicit SSH allowlists and host-key verification.
- Publish reproducible synthetic fixtures, a demonstration walkthrough, and an honest pre-release once the necessary checks pass.

## Open-source readiness gates

1. Confirm copyright ownership and authorization for all code and materials.
2. Choose and publish an approved OSI-compatible license; replace the placeholder `LICENSE` only after authorization.
3. Audit Git history and public fixtures for secrets, confidential topology, and third-party materials.
4. Establish a working private vulnerability reporting process and clear external contribution rules.
5. Invite genuine outside tests and feedback without making claims about adoption until evidenced.

## Later: production-readiness work

- Device-specific CLI compatibility and least-privilege acceptance on approved lab hardware.
- Complete deferred Phase E security/performance/release gates.
- Production identity integration, at-rest backup protection, and key-management hardening.
- Scheduled backups and broader inventory features.

**Release status:** Phase E is DEFERRED / NOT ACCEPTED. Production release is BLOCKED. [PR #9](https://github.com/HosseinStan/Aegis/pull/9) is a preserved development snapshot, not a verified production feature.
