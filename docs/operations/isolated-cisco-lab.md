# Isolated Cisco SSH lab: first physical-device validation (L1)

> **Development / isolated hardware only.** Phase E is DEFERRED / NOT ACCEPTED; production deployment is BLOCKED. This runbook does not authorize connecting to corporate networks. Lab backup creation remains locked. The `v0.1.0-alpha` synthetic demo does **not** include this Lab feature.

## Goal and topology

An operator should be able to register a single non-production Cisco IOS / IOS XE switch, validate the SSH server identity and read-only access, then display (but not store) its running configuration in the separate Lab Vue interface.

```text
Windows development PC
  dedicated Ethernet/USB-Ethernet adapter: 192.168.77.10/24
            |
            | dedicated cable / small isolated unmanaged switch
            |
Non-production Cisco switch
  reachable management SVI/interface: 192.168.77.2/24
  SSH TCP/22; dedicated read-only account
```

The addresses are **examples only**: use a clean, non-overlapping private subnet, and verify the actual switch management VLAN, interface and SSH port. A direct Ethernet cable is sufficient on interfaces supporting Auto-MDI/MDIX; an unmanaged lab switch is also acceptable. Do **not** connect any Lab port or uplink to the production LAN, Internet router or corporate VLAN trunks.

## 0. Hardware and rollback prerequisites

- Use an authorized **spare/test** Cisco switch, not a live production switch. Identify its exact model, IOS / IOS XE version, available console cable, and physical management/access port. Some switches require an SVI with a participating up/up access port; others have a separate management port.
- Preserve the current switch configuration before making lab setup changes, via an authorized secure backup outside the Aegis Lab application; never commit the backup or its credentials to Git.
- Use a dedicated Windows network adapter. Record its existing IP, DHCP, gateway and DNS settings before changes. Do not change the primary corporate adapter, enable Internet Connection Sharing, network bridging or IP forwarding, or connect to production.
- Keep `C:\AegisOSSPreview\repo` unchanged until explicitly working in the new integration branch; use a new working directory for any Lab trial. The Lab runtime data directory must be outside any Git checkout.

## 1. Set up the isolated PC link

With PowerShell **as Administrator**, after confirming the interface name refers only to the spare lab adapter:

```powershell
Get-NetAdapter
Get-NetIPConfiguration -InterfaceAlias 'Ethernet Lab'
New-NetIPAddress -InterfaceAlias 'Ethernet Lab' -IPAddress 192.168.77.10 -PrefixLength 24
```

This example deliberately omits `-DefaultGateway` and DNS. If there is already an incompatible address on that adapter, have an operator review/remove that specific address before applying the example; do not blindly alter another interface. Confirm the interface has no default gateway, and check for unintended routes with `route print`. Preserve a way to restore the adapter's original settings.

The Cisco switch must *already* have an authorized SSH-capable management IP such as `192.168.77.2/24` in the correct VLAN/interface. If not, configure it **locally through its console** using its model- and firmware-specific procedure, only with authorization. Aegis itself cannot assign the first IP, enable SSH or configure the switch.

## 2. Verify reachability and manual read-only policy

From Windows:

```powershell
Test-Connection 192.168.77.2 -Count 2
Test-NetConnection 192.168.77.2 -Port 22
```

Ping may be disabled on the switch even when SSH works. Do not mistake TCP/22 availability for authentication or access approval. Verify the dedicated account with a manual SSH client using a **previously verified host key**. Confirm the account can perform `terminal length 0` and `show running-config`, and that it **cannot** enter configuration mode, reload or write. On releases that support Cisco CLI/parser views, use a narrowly scoped view rather than an unrestricted privilege-15 admin user; the exact view setup is device/version dependent. The existing connector uses a shell, may send `terminal length 0`, and requests `show running-config`; inspect your device's actual behavior before proceeding.

Never use your daily administrative credentials. Do not paste full configuration output, private keys or passwords in GitHub issues, screenshots or chats.

## 3. Independently validate the switch SSH host key

Obtain the **host public key or its fingerprint through a trusted out-of-band source**, such as the physical console/device records. An `ssh-keyscan` result by itself is **untrusted discovery**, *not verification*. If your IOS version cannot independently expose the host key or trusted fingerprint, stop and establish an approved verification method before connecting Aegis.

Example discovery on Windows, only to compare against the verified key:

```powershell
ssh-keyscan -p 22 192.168.77.2 2>$null
```

For the Lab approvals file, provide one exact `address` and the corresponding **verified** OpenSSH-formatted host public key, without the IP address prefix:

```json
{
  "targets": [
    {
      "address": "192.168.77.2:22",
      "host_key": "ssh-rsa REPLACE_WITH_THE_VERIFIED_HOST_PUBLIC_KEY"
    }
  ]
}
```

Keep this JSON file private and **outside Git**; never check in actual host keys coupled with real management addresses or credentials. The built Lab manager supports **at most two pinned endpoints**. With no file and no `-enable-ssh`, the default is zero permitted connections. Do not enable SSH merely to see whether a switch answers.

## 4. Launch the independent Lab UI

The Lab UI is a **different build** from the existing four-device synthetic demo; the `start-local.ps1` demo launcher is not the Lab launcher. Use the integration branch code with the current Go 1.26.11 / Node 24.22.1 / pnpm 11.25.0 toolchain, installed/cached and verified separately. Adapt paths as necessary:

```powershell
cd C:\AegisLabSource\repo\apps\aegis-ui\vue-mvp
pnpm exec vite build --config vite.lab.config.js --outDir C:/AegisLab/ui

cd C:\AegisLabSource\repo\apps\aegis-api
go build -mod=readonly -trimpath -o C:/AegisLab/aegis-lab.exe ./cmd/lab-manager
```

Create two different local operator/viewer passwords (12–72 UTF-8 bytes) and launch in PowerShell 7:

```powershell
$env:AEGIS_LAB_OPERATOR_PASSWORD = Read-Host 'Lab operator password' -MaskInput
$env:AEGIS_LAB_VIEWER_PASSWORD = Read-Host 'Lab viewer password' -MaskInput
try {
    & 'C:/AegisLab/aegis-lab.exe' -data-dir 'C:/AegisLab/data' -ui-dir 'C:/AegisLab/ui'
} finally {
    Remove-Item Env:AEGIS_LAB_OPERATOR_PASSWORD, Env:AEGIS_LAB_VIEWER_PASSWORD -ErrorAction SilentlyContinue
}
```

This first run, **without** `-enable-ssh`, is the local inventory-only gate. Verify you can create a credential and register a disabled device, that no network connection occurs, and that a viewer cannot access operator actions. The app binds its own HTTP service to `127.0.0.1`. The external `C:/AegisLab/data` directory is created as a private Windows directory with DPAPI-backed credential key protection. Do not pre-create it with permissive ACLs.

Only after the independent host key and read-only account checks pass, stop the Lab process and start it again with **both** `-enable-ssh` and `-approvals 'C:/AegisLab/approvals.json'`. Re-save the device under this approved profile, then click **Test SSH**, and only afterwards **Read running configuration**. The latter displays potentially sensitive configuration without saving a Lab snapshot.

## 5. Evidence, stop conditions and acceptance

Record only sanitized evidence: switch model and IOS family, link reachability, exact test stage reached, SSH-host-key match or rejection, outcome code, whether the read-only command succeeded, and test date. Redact actual IPs, usernames, configurations, secrets and fingerprints in public GitHub threads. Use `NOT RUN`, `PASS`, `FAIL` or `BLOCKED` per gate; **never** label a test as passed merely from CI or a demo.

Stop on unexpected routing/bridging, host-key mismatch, SSH auth failure, unauthorized CLI capability, excessive or sensitive output, or unexpected on-disk configuration content. Do not bypass host-key validation or loosen account restrictions to make a test pass.

After the test, Ctrl+C the Lab, disconnect the isolated cable, restore the dedicated Windows adapter to its recorded previous state, and check that no routes remain. Lab backups remain **disabled** pending an accepted encrypted-at-rest design for snapshots **and** SQLite change evidence. Passing this L1 test does not clear Phase E or permit production release.
