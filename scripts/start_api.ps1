param(
  [string]$DatabaseUrl = "postgres://elnote:elnote@localhost:5432/elnote?sslmode=disable",
  [string]$JwtSecret = "dev-secret-dev-secret-dev-secret-123",
  [string]$HttpAddr = ":8080",
  [string]$InitialAdminPassword = "",
  [switch]$AllowLocalAdminReset = $true,
  [string]$SmtpHost = "smtp.gmail.com",
  [int]$SmtpPort = 587,
  [string]$SmtpUsername = "mhendzellab",
  [string]$SmtpPassword = "",
  [string]$SmtpFrom = "mhendzellab@gmail.com"
)

$ErrorActionPreference = "Stop"

Write-Host "Starting ELNOTE API on $HttpAddr ..."
Write-Host "On first startup, record the LabAdmin bootstrap password shown in server logs."

$allowResetValue = if ($AllowLocalAdminReset) { "true" } else { "false" }

wsl -u root sh -lc "cd /mnt/c/Users/mjhen/Github/ELNOTE/server && go build -o /tmp/elnote-api ./cmd/api; cd /mnt/c/Users/mjhen/Github/ELNOTE/server; env DATABASE_URL='$DatabaseUrl' JWT_SECRET='$JwtSecret' HTTP_ADDR='$HttpAddr' AUTO_MIGRATE=true MIGRATIONS_DIR='/mnt/c/Users/mjhen/Github/ELNOTE/server/migrations' INITIAL_ADMIN_PASSWORD='$InitialAdminPassword' ALLOW_LOCAL_ADMIN_RESET='$allowResetValue' SMTP_HOST='$SmtpHost' SMTP_PORT='$SmtpPort' SMTP_USERNAME='$SmtpUsername' SMTP_PASSWORD='$SmtpPassword' SMTP_FROM='$SmtpFrom' /tmp/elnote-api"
