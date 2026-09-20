# ADR 0001: Aegis API MVP boundaries

Status: accepted for the first milestone.

- The repository root is the current `Aegis` directory; no nested `aegis`
  directory is created.
- `packages/shared-contracts/openapi/aegis-api.yaml` is the only canonical HTTP
  contract. The server reads that file locally and the container copies it to
  `/app/openapi.yaml`.
- Runtime state is normalized below `data`: `data/aegis.db`,
  `data/known_hosts`, and `data/backups`.
- Local JWT is implemented behind an authentication service for development.
  Production identity adapters remain explicit future work and unsupported
  modes fail startup.
- Manual backup is in the MVP. The scheduler, schedules API, and distributed
  job behavior are deferred.
- Running/startup configuration, facts, and interface status are implemented.
  VLAN, MAC-table, and neighbor operations remain outside this milestone.

