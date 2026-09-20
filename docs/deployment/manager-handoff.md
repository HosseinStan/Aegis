# Aegis operational handoff

## What is ready

This milestone is a working, read-only Cisco IOS/IOS-XE configuration service.
It provides authenticated inventory, encrypted device credentials, strict SSH
host-key checking, running/startup configuration retrieval, device facts and
interfaces, configuration backups and diffs, role-based access control, audit
records, health checks, metrics, an OpenAPI contract, and container packaging.

The local evaluation console is for acceptance testing only. The organization's
existing application should integrate with the REST API described by the
OpenAPI contract. Production deliberately refuses to expose the evaluation
console.

## Recommended pilot server

- One current Linux virtual machine with 2 vCPU, 4 GB RAM, and at least 20 GB
  of protected persistent storage. Increase storage according to device count,
  configuration size, backup frequency, and retention policy.
- Docker Engine with the Compose plugin, or an equivalent managed container
  runtime. The supplied Compose deployment runs the API and Caddy reverse proxy.
- Internal DNS name and an organization-trusted TLS certificate.
- Inbound TCP 443 only from approved user/VPN networks.
- Outbound TCP 22 only to the approved Cisco management addresses. No Internet
  access is required at runtime.
- A protected backup of the persistent Aegis data volume. The volume contains
  SQLite data, audit records, SSH host keys, and configuration backup files.

SQLite is appropriate for a single-node pilot. High availability, multiple API
replicas, or larger organizational scale requires a planned database migration
before rollout.

## Security inputs the organization must provide

1. A dedicated read-only account on each pilot switch. It must be limited on the
   device to the commands needed for version, interfaces, and running/startup
   configuration inspection.
2. Verified SSH host keys collected over a trusted administrative channel and
   imported into Aegis `known_hosts`. Aegis has no host-key bypass.
3. Two independent 32-byte random secrets: `JWT_SIGNING_KEY` and
   `CREDENTIAL_MASTER_KEY`. Store them in the organization's secret manager,
   not in source control or an image.
4. The approved application URL for CORS, trusted reverse-proxy network ranges,
   TLS certificate ownership, backup retention, and log retention requirements.
5. An identity integration decision. This milestone implements local JWT users;
   organizational OIDC/SSO, LDAP, and existing-application token integration are
   planned but not yet implemented.

## Pilot acceptance sequence

1. Deploy to an isolated management-network pilot host with HTTPS.
2. Keep `LOCAL_TEST_CONSOLE_ENABLED=false` and remove bootstrap administrator
   values after the first successful start.
3. Confirm `/healthz` and `/readyz`, then connect the existing application using
   the OpenAPI contract.
4. Import the verified SSH host key for one non-production Cisco switch.
5. Store the switch's read-only credential, register the switch, and test SSH.
6. Read running and startup configurations, facts, and interfaces. Confirm no
   configuration change is made on the switch.
7. Create two backups around an approved lab configuration change and verify the
   diff and audit record.
8. Verify access restrictions, TLS, firewall rules, monitoring, restore from
   backup, and secret-recovery procedures before adding production devices.

## Required decisions before organization-wide production

- Complete SSO/identity integration and user lifecycle management.
- Choose a database/high-availability design if a single-node service is not
  acceptable.
- Decide whether configuration backup files require application-level
  encryption; this milestone relies on protected storage and backup controls.
- Define key rotation, retention, disaster recovery, alerting, and maintenance
  ownership.
- Perform an organizational security review and a controlled network-device
  acceptance test.

See `docs/operations/runbook.md`, `docs/operations/cisco-preparation.md`, and
`docs/security/README.md` for the detailed operating controls.
