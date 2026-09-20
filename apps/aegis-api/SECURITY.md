# Aegis API security assumptions

- The service is deployed on a trusted management network behind HTTPS, a VPN,
  and the organization's identity-aware access controls.
- Managed devices are explicitly registered by administrators. Normal users
  cannot submit a destination IP address or CLI command.
- SSH host keys are imported through a trusted process and strictly verified.
- Device accounts are read-only and restricted on the device itself.
- JWT and credential master keys are independent, high-entropy secrets supplied
  at runtime. Production startup fails without them or without an HTTPS public
  URL.
- SQLite, `known_hosts`, and the backup tree reside on a protected persistent
  volume. Host backups must preserve their confidentiality.
- Logs and audit records contain identifiers and error codes, never credential
  material or configuration bodies.

Known security limitations of this milestone:

- Local JWT logout is stateless and does not revoke an already issued access
  token; access tokens default to 15 minutes.
- OIDC, existing-application JWT, and LDAP adapters are designed but not yet
  implemented. `AUTH_MODE` therefore fails closed unless set to `local_jwt`.
- Encrypted private keys with passphrases and online credential-master-key
  rotation are not implemented.
- Backup content is protected by filesystem permissions but is not encrypted
  separately at rest.
- The embedded scheduler and scheduled backups are deferred to the next phase.

No Telnet client, generic command endpoint, or host-key bypass exists.

