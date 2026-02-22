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

$sqfliteWorkerSource = Join-Path $clientPath "web\sqflite_sw.js"
$sqfliteWasmSource = Join-Path $clientPath "web\sqlite3.wasm"

if (-not (Test-Path $sqfliteWorkerSource) -or -not (Test-Path $sqfliteWasmSource)) {
  Write-Host "Setting up sqflite web binaries ..."
  Push-Location $clientPath
  try {
    flutter pub run sqflite_common_ffi_web:setup
  }
  finally {
    Pop-Location
  }
}

if (-not (Test-Path $sqfliteWorkerSource) -or -not (Test-Path $sqfliteWasmSource)) {
  Write-Error "Missing required sqflite web files in '$clientPath\\web' (sqflite_sw.js, sqlite3.wasm)."
  exit 1
}

Copy-Item -Path $sqfliteWorkerSource -Destination (Join-Path $buildWebPath "sqflite_sw.js") -Force
Copy-Item -Path $sqfliteWasmSource -Destination (Join-Path $buildWebPath "sqlite3.wasm") -Force

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
