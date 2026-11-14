# Contributing to Aegis

Thank you for your interest in making infrastructure configuration review safer.

> **Licensing gate:** Aegis has not yet been released under an open-source license. Its current `LICENSE` does not grant redistribution or reuse rights. Please do not submit external code contributions until the maintainers publish a confirmed contribution and licensing policy.

## Before opening an issue

- Check the [README](README.md) and [roadmap](docs/ROADMAP.md) for current versus draft capabilities.
- Describe the device model or platform generically. Do not include customer names, real configuration text, keys, passwords, live IP addresses, or internal network identifiers.
- For possible security vulnerabilities, **do not file a public exploit report**. See [SECURITY.md](SECURITY.md); a functioning private reporting channel must be established before public intake of security reports.

## Developer workflow (for authorized maintainers)

1. Work in a dedicated branch; keep unrelated changes out of the diff.
2. Preserve security boundaries: **no Telnet**, arbitrary CLI execution, disabled host-key verification, configuration writes, or sensitive config logging.
3. Run the repository checks from the root:

~~~sh
make check
~~~

4. For deeper verification run the documented [API checks](apps/aegis-api/README.md), including race tests, and check GitHub Actions.
5. Explain the behavior change, testing performed, known limitations, and whether acceptance or production security review is still outstanding.
6. Never promote deferred or unverified work as production-ready. Keep credentials, backups, databases, and local runtime data out of Git.

## Planned public contribution process

After licensing and authorization are resolved, maintainers intend to add reproducible development fixtures, first-issue guidance, and PR/issue templates. This section is a plan, not a claim that outside contributions are currently accepted.
