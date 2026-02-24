param(
  [string]$DatabaseUrl = "postgres://elnote:elnote@localhost:5432/elnote?sslmode=disable",
  [string]$JwtSecret = "dev-secret-dev-secret-dev-secret-123",
  [string]$ApiAddr = ":8080",
  [string]$SmtpHost = "smtp.gmail.com",
  [int]$SmtpPort = 587,
  [string]$SmtpUsername = "mhendzellab",
  [string]$SmtpPassword = "",
  [string]$SmtpFrom = "mhendzellab@gmail.com",
  [string]$HostIp = "0.0.0.0",
  [int]$WebPort = 8090,
  [switch]$RebuildWeb
)

$ErrorActionPreference = "Stop"

$scriptRoot = Split-Path -Parent $MyInvocation.MyCommand.Path
$apiScript = Join-Path $scriptRoot "start_api.ps1"
$webScript = Join-Path $scriptRoot "start_tablet_web.ps1"

if (-not (Test-Path $apiScript)) {
  Write-Error "Missing script: $apiScript"
  exit 1
}

if (-not (Test-Path $webScript)) {
  Write-Error "Missing script: $webScript"
  exit 1
}

$apiArgs = @(
  "-NoProfile",
  "-ExecutionPolicy", "Bypass",
  "-File", $apiScript,
  "-DatabaseUrl", $DatabaseUrl,
  "-JwtSecret", $JwtSecret,
  "-HttpAddr", $ApiAddr,
  "-SmtpHost", $SmtpHost,
  "-SmtpPort", $SmtpPort,
  "-SmtpUsername", $SmtpUsername,
  "-SmtpPassword", $SmtpPassword,
  "-SmtpFrom", $SmtpFrom
)

$webArgs = @(
  "-NoProfile",
  "-ExecutionPolicy", "Bypass",
  "-File", $webScript,
  "-HostIp", $HostIp,
  "-Port", $WebPort
)

if ($RebuildWeb) {
  $webArgs += "-Rebuild"
}

Write-Host "Starting API and tablet web host..."
$apiProc = Start-Process -FilePath "powershell" -ArgumentList $apiArgs -PassThru
Start-Sleep -Milliseconds 500
$webProc = Start-Process -FilePath "powershell" -ArgumentList $webArgs -PassThru

Write-Host "API PID: $($apiProc.Id)"
Write-Host "WEB PID: $($webProc.Id)"
Write-Host "Press Ctrl+C to stop both."

Start-Sleep -Seconds 2

$localApiReachable = Test-NetConnection -ComputerName "localhost" -Port 8080 -InformationLevel Quiet
$localWebReachable = Test-NetConnection -ComputerName "localhost" -Port $WebPort -InformationLevel Quiet

if (-not $localApiReachable) {
  Write-Warning "API port 8080 is not reachable on localhost. Tablet sign-in/sync will fail."
}

if (-not $localWebReachable) {
  Write-Warning "Web host port $WebPort is not reachable on localhost. Tablet UI may not load."
}

$lanIps = Get-NetIPAddress -AddressFamily IPv4 -ErrorAction SilentlyContinue |
  Where-Object { $_.IPAddress -match '^10\.|^192\.168\.|^172\.(1[6-9]|2[0-9]|3[0-1])\.' } |
  Select-Object -ExpandProperty IPAddress -Unique

foreach ($lanIp in $lanIps) {
  $apiLanReachable = Test-NetConnection -ComputerName $lanIp -Port 8080 -InformationLevel Quiet
  $webLanReachable = Test-NetConnection -ComputerName $lanIp -Port $WebPort -InformationLevel Quiet

  if ($apiLanReachable -and $webLanReachable) {
    Write-Host "Tablet-ready on $lanIp : API 8080 and Web $WebPort reachable." -ForegroundColor Green
  }
  else {
    Write-Warning "LAN check for $lanIp failed (API:$apiLanReachable WEB:$webLanReachable). Check firewall/router rules for ports 8080 and $WebPort."
  }
}

try {
  while ($true) {
    Start-Sleep -Seconds 1

    if ($apiProc.HasExited) {
      Write-Host "API process exited with code $($apiProc.ExitCode)."
      break
    }

    if ($webProc.HasExited) {
      Write-Host "Web process exited with code $($webProc.ExitCode)."
      break
    }
  }
}
finally {
  if ($apiProc -and -not $apiProc.HasExited) {
    Stop-Process -Id $apiProc.Id -Force
  }
  if ($webProc -and -not $webProc.HasExited) {
    Stop-Process -Id $webProc.Id -Force
  }
}
