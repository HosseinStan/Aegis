# Contributing to Aegis

Thank you for your interest in making infrastructure configuration review safer.

> Aegis is distributed under **Apache-2.0** (see LICENSE). Contributions should be original, authorized for submission, and compatible with this project license. Do not submit company-owned or third-party source without authorization.

## Before opening an issue

- Check the [README](README.md) and [roadmap](docs/ROADMAP.md) for current versus draft capabilities.
- Describe the device model or platform generically. Do not include customer names, real configuration text, keys, passwords, live IP addresses, or internal network identifiers.
- For possible security vulnerabilities, **do not file a public exploit report**. Use the active [GitHub private vulnerability reporting channel](https://github.com/HosseinStan/Aegis/security/advisories) described in [SECURITY.md](SECURITY.md).

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

Public issue and PR templates are available for non-sensitive changes. Maintainers may review incoming contributions, but merging code is at their discretion. No existing third-party contribution or adoption is implied.
