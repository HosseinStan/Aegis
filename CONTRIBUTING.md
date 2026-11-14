# Contributing to Aegis

Thank you for your interest in making infrastructure configuration review safer.

> **Licensing gate:** The maintainer approved Apache-2.0 in draft [PR #15](https://github.com/HosseinStan/Aegis/pull/15), but the default branch's existing `LICENSE` does not grant redistribution or reuse rights. Please wait until that change is merged and the public contribution policy is finalized before contributing code.

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

Issue and pull-request templates are provided to help structure non-sensitive public reports. Once the license has merged and the maintainer has completed the remaining disclosure/acceptance review, contribution guidelines can be opened to external code. No existing third-party contribution or adoption is implied.
