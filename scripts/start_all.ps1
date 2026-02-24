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
