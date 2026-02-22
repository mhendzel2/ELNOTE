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

if ($HostIp -eq "0.0.0.0") {
  Write-Host "Serving tablet web GUI on all interfaces (0.0.0.0:$Port)."
  Write-Host "Open from this PC: http://localhost:$Port"
  $lanIps = Get-NetIPAddress -AddressFamily IPv4 -ErrorAction SilentlyContinue |
    Where-Object { $_.IPAddress -match '^10\.|^192\.168\.|^172\.(1[6-9]|2[0-9]|3[0-1])\.' } |
    Select-Object -ExpandProperty IPAddress -Unique

  foreach ($lanIp in $lanIps) {
    Write-Host "Open from tablet: http://$lanIp`:$Port"
  }
}
else {
  Write-Host "Serving tablet web GUI on http://$HostIp`:$Port ..."
}
Push-Location $buildWebPath
try {
  python -m http.server $Port --bind $HostIp
}
finally {
  Pop-Location
}
