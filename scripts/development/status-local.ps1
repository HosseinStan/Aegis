$ErrorActionPreference = "Stop"
$repoRoot = (Resolve-Path (Join-Path $PSScriptRoot "..\..")).Path
$pidPath = Join-Path $repoRoot "apps\aegis-api\data\aegis-api.pid"

if (-not (Test-Path -LiteralPath $pidPath)) {
    Write-Output "Aegis API is not running (no PID file)."
    exit 1
}

$serverPid = [int](Get-Content -LiteralPath $pidPath -Raw)
$process = Get-Process -Id $serverPid -ErrorAction SilentlyContinue
if (-not $process) {
    Write-Output "Aegis API is not running (stale PID file: $serverPid)."
    exit 1
}

$health = Invoke-RestMethod -Uri "http://127.0.0.1:8080/healthz" -TimeoutSec 3
$ready = Invoke-RestMethod -Uri "http://127.0.0.1:8080/readyz" -TimeoutSec 3
Write-Output "Aegis API PID: $serverPid"
Write-Output "Health: $($health.data.status)"
Write-Output "Readiness: $($ready.data.status)"
Write-Output "URL: http://127.0.0.1:8080"

