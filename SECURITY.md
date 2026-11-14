# Security policy

Aegis is an early-stage infrastructure project. The public repository is not
an authorization to use the software in production. Phase E remains DEFERRED /
NOT ACCEPTED, and production release remains BLOCKED.

## Reporting a vulnerability

**Do not disclose a suspected vulnerability, exploit details, device
configurations, credentials, or customer information in a public GitHub issue,
pull request, or discussion.**

**GitHub private vulnerability reporting is enabled and was verified by the
repository maintainer on 2026-11-14.** To submit a confidential report, use
[Security Advisories](https://github.com/HosseinStan/Aegis/security/advisories)
and select **Report a vulnerability** from an eligible non-maintainer account.
The repository administrator may see a different advisory management view.
Do not submit a test or sensitive report through a public issue.

Maintainer setup instructions:
https://docs.github.com/en/code-security/how-tos/report-and-fix-vulnerabilities/configure-vulnerability-reporting/configure-for-a-repository

## Sensitive material

Never commit Aegis JWT signing keys, credential master keys, device credentials,
SSH private keys, known-host databases, configuration backups, production
database files, or real network topology.

Details of deployment boundaries, device least privilege and known limitations
are in [API security assumptions](apps/aegis-api/SECURITY.md) and
[device preparation](docs/operations/cisco-preparation.md).
