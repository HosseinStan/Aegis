# Aegis Lab Device Manager · L1

The Windows Lab profile serves this Vue app and its API from one loopback origin. It has its own SQLite inventory, encrypted SSH credential store and reserved backup directory. The four-device Demo and its launcher are independent.

## Start on Windows

Use the existing pinned Vue dependencies and Go 1.26.10 toolchain. Use a new external working directory whose parent exists. Keep Go build caches outside Git and use the already available module cache. In the following example, use your actual external paths and installed tools.

1. From `apps/aegis-ui/vue-mvp`, build the Lab UI with `pnpm exec vite build --config vite.lab.config.js --outDir C:/AegisLab/ui`.
2. From `apps/aegis-api`, build with `go build -mod=readonly -trimpath -o C:/AegisLab/aegis-lab.exe ./cmd/lab-manager`.
3. In PowerShell 7, enter two distinct local passwords of 12–72 UTF-8 bytes. Reuse the same data directory on later launches to retain devices and their credentials.

```powershell
$env:AEGIS_LAB_OPERATOR_PASSWORD = Read-Host 'Local Lab operator password' -MaskInput
$env:AEGIS_LAB_VIEWER_PASSWORD = Read-Host 'Local Lab viewer password' -MaskInput
try {
    & 'C:/AegisLab/aegis-lab.exe' -data-dir 'C:/AegisLab/data' -ui-dir 'C:/AegisLab/ui'
} finally {
    Remove-Item Env:AEGIS_LAB_OPERATOR_PASSWORD, Env:AEGIS_LAB_VIEWER_PASSWORD -ErrorAction SilentlyContinue
}
```

Open the printed HTTP URL. Ctrl+C stops the process. The server clears inherited password variables, hashes the passwords using the existing authentication service and keeps sessions in memory for 20 minutes. Credentials, session values and configuration responses are excluded from the frontend bundle and local browser storage. Use this development profile on a trusted local workstation. Loopback HTTP has no TLS and is intended for the local browser only.

The new data root and its `backups` directory have Windows ACLs limited to the owning user and SYSTEM. The existing AES-GCM credential store uses a separate Lab key protected by Windows user-scoped DPAPI. Keep the database and its DPAPI key together under the original Windows account. The server refuses a Git checkout, an occupied uninitialized directory or another running instance using that data directory.

## Register and review readiness

- Sign in as **Lab operator**. Create a credential label for a dedicated read-only device account. Passwords and unencrypted private keys supported by the existing SSH library can be stored. The API returns the label, type and ID.
- Register or edit a Cisco IOS/IOS XE device with its actual details and credential. Saving a record only writes local inventory. The **Read-only viewer** can inspect records and saved backups without access to credential management or SSH.
- SSH starts with **zero approved destinations**. To prepare for a separately authorized connection, the Lab owner must independently obtain each device's host public key through a trusted channel and explicitly approve its exact IP and port. The local approvals file has a `targets` array of one or two objects, each with an `address` literal IP:port and a `host_key` OpenSSH public-key string. Keep this file outside Git. Start with both `-enable-ssh` and `-approvals` only after that approval. Re-save devices that were registered in the disabled profile.
- **Test SSH** checks the approved target and the key during handshake. **Read running configuration** uses the fixed Cisco read command and displays the result without saving a snapshot. The last test shown belongs to this running Lab session. The connector rejects an unapproved IP:port before opening a socket and rejects a mismatched pinned key during handshake.
- **Create manual backup** is locked. The current BackupService writes plaintext snapshots and SQLite change evidence can include configuration commands. A private directory ACL alone does not establish approved protection for that content at rest. Existing BackupService List, Content and Diff paths are wired and tested against synthetic saved snapshots.

Focused checks use the existing fake Cisco SSH server on loopback and temporary synthetic SQLite records. Physical switches are outside L1 validation.
