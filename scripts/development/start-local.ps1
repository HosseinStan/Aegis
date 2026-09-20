param([switch]$Foreground)

$ErrorActionPreference = "Stop"
$repoRoot = (Resolve-Path (Join-Path $PSScriptRoot "..\..")).Path
$appRoot = Join-Path $repoRoot "apps\aegis-api"
$pidPath = Join-Path $appRoot "data\aegis-api.pid"
$binary = Join-Path $appRoot "bin\aegis-api.exe"

if (Test-Path -LiteralPath $pidPath) {
    $existingPid = [int](Get-Content -LiteralPath $pidPath -Raw)
    if (Get-Process -Id $existingPid -ErrorAction SilentlyContinue) {
        throw "Aegis API is already running with PID $existingPid"
    }
    Remove-Item -LiteralPath $pidPath
}

if (-not (Test-Path -LiteralPath (Join-Path $appRoot ".env"))) {
    throw "Missing apps/aegis-api/.env. Run 'go run ./cmd/setup-local' from apps/aegis-api first."
}

Push-Location $appRoot
try {
    New-Item -ItemType Directory -Force -Path (Join-Path $appRoot "bin") | Out-Null
    go build -trimpath -o $binary ./cmd/server
    if ($LASTEXITCODE -ne 0) { throw "Aegis API build failed" }
    if ($Foreground) { & $binary; exit $LASTEXITCODE }
    $stdout = Join-Path $appRoot "data\aegis-api.stdout.log"
    $stderr = Join-Path $appRoot "data\aegis-api.stderr.log"
    $process = Start-Process -FilePath $binary -WorkingDirectory $appRoot -RedirectStandardOutput $stdout -RedirectStandardError $stderr -WindowStyle Hidden -PassThru
    Set-Content -LiteralPath $pidPath -Value $process.Id -NoNewline
    Write-Output "Aegis API started with PID $($process.Id)"
} finally {
    Pop-Location
}

