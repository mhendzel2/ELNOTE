param(
  [int[]]$Ports = @(8080, 8090)
)

$ErrorActionPreference = "Stop"

function Stop-ByPort {
  param(
    [int]$Port
  )

  $connections = Get-NetTCPConnection -LocalPort $Port -State Listen -ErrorAction SilentlyContinue
  if (-not $connections) {
    Write-Host "No listening process found on port $Port"
    return
  }

  $pids = $connections | Select-Object -ExpandProperty OwningProcess -Unique
  foreach ($pid in $pids) {
    try {
      $proc = Get-Process -Id $pid -ErrorAction Stop
      Stop-Process -Id $pid -Force -ErrorAction Stop
      Write-Host "Stopped PID $pid ($($proc.ProcessName)) on port $Port"
    }
    catch {
      Write-Host "Failed to stop PID $pid on port $Port: $($_.Exception.Message)"
    }
  }
}

foreach ($port in $Ports) {
  Stop-ByPort -Port $port
}
