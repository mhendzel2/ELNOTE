param(
  [string]$HostIp = "0.0.0.0",
  [int]$Port = 8090,
  [switch]$Rebuild
)

$ErrorActionPreference = "Stop"

$repoRoot = Split-Path -Parent $PSScriptRoot
$clientPath = Join-Path $repoRoot "client_flutter"
$buildWebPath = Join-Path $clientPath "build\web"

if ($Rebuild) {
  Write-Host "Rebuilding Flutter web bundle ..."
  Push-Location $clientPath
  try {
    flutter build web --release
  }
  finally {
    Pop-Location
  }
}

if (-not (Test-Path $buildWebPath)) {
  Write-Error "Web build not found at '$buildWebPath'. Run 'flutter build web --release' in client_flutter or use -Rebuild."
  exit 1
}

Write-Host "Serving tablet web GUI on http://$HostIp`:$Port ..."
Push-Location $buildWebPath
try {
  python -m http.server $Port --bind $HostIp
}
finally {
  Pop-Location
}
