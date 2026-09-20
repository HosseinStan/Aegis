# Cisco IOS/IOS XE preparation

Use a dedicated Aegis service account on every approved device. Never reuse a
normal network administrator account. Where the exact IOS release supports it,
use parser views/CLI views to permit only these read operations:

```text
terminal length 0
show running-config
show startup-config
show version
show inventory
show interfaces status
```

Explicitly deny configuration mode, reload, copy, delete, archive, debug, and
write operations. Some IOS releases require elevated privilege for the complete
running configuration; verify the least-privilege view on every deployed model
and software release.

Before registering a device, import its SSH host key from a trusted,
out-of-band source. Do not trust a key first observed over an unverified
network path. The OpenSSH known-hosts entry must use `[address]:port` for a
non-default port. Mount or place the resulting file at the configured
`SSH_KNOWN_HOSTS_FILE` path and make it readable only by the Aegis runtime user.

Test the account manually against a non-production switch and confirm that
configuration commands are rejected before enabling Aegis access.

