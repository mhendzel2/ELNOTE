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

  $processIds = $connections | Select-Object -ExpandProperty OwningProcess -Unique
  foreach ($procId in $processIds) {
    try {
      $proc = Get-Process -Id $procId -ErrorAction Stop
      Stop-Process -Id $procId -Force -ErrorAction Stop
      Write-Host "Stopped PID $procId ($($proc.ProcessName)) on port $Port"
    }
    catch {
      Write-Host "Failed to stop PID $procId on port ${Port}: $($_.Exception.Message)"
    }
  }
}

foreach ($port in $Ports) {
  Stop-ByPort -Port $port
}
