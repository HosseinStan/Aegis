$ErrorActionPreference = "Stop"
$repoRoot = (Resolve-Path (Join-Path $PSScriptRoot "..\..")).Path
$pidPath = Join-Path $repoRoot "apps\aegis-api\data\aegis-api.pid"

if (-not (Test-Path -LiteralPath $pidPath)) {
    Write-Output "Aegis API is not running (no PID file)."
    exit 0
}

$serverPid = [int](Get-Content -LiteralPath $pidPath -Raw)
$process = Get-Process -Id $serverPid -ErrorAction SilentlyContinue
if ($process) {
    Stop-Process -Id $serverPid
    $process.WaitForExit()
    Write-Output "Aegis API stopped (PID $serverPid)."
}
Remove-Item -LiteralPath $pidPath

