# Security policy

Aegis is an early-stage infrastructure project. The public repository is not
an authorization to use the software in production. Phase E remains DEFERRED /
NOT ACCEPTED, and production release remains BLOCKED.

## Reporting a vulnerability

**Do not disclose a suspected vulnerability, exploit details, device
configurations, credentials, or customer information in a public GitHub issue,
pull request, or discussion.**

The maintainer is preparing GitHub's **private vulnerability reporting**
feature. Once the maintainer confirms it is enabled, go to the repository's
Security / Advisories page and select **Report a vulnerability** to file a
confidential report. Until that workflow is enabled and verified, this
repository does not claim to have a functioning public-facing private
vulnerability intake channel. Please do not publish security-sensitive report
details while an appropriate channel is being established.

Maintainer setup instructions:
https://docs.github.com/en/code-security/how-tos/report-and-fix-vulnerabilities/configure-vulnerability-reporting/configure-for-a-repository

## Sensitive material

Never commit Aegis JWT signing keys, credential master keys, device credentials,
SSH private keys, known-host databases, configuration backups, production
database files, or real network topology.

Details of deployment boundaries, device least privilege and known limitations
are in [API security assumptions](apps/aegis-api/SECURITY.md) and
[device preparation](docs/operations/cisco-preparation.md).
