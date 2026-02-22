param(
  [string]$DatabaseUrl = "postgres://elnote:elnote@localhost:5432/elnote?sslmode=disable",
  [string]$JwtSecret = "dev-secret-dev-secret-dev-secret-123",
  [string]$HttpAddr = ":8080"
)

$ErrorActionPreference = "Stop"

Write-Host "Starting ELNOTE API on $HttpAddr ..."

wsl -u root sh -lc "if [ ! -x /tmp/elnote-api ]; then cd /mnt/c/Users/mjhen/Github/ELNOTE/server && go build -o /tmp/elnote-api ./cmd/api; fi; cd /mnt/c/Users/mjhen/Github/ELNOTE/server; env DATABASE_URL='$DatabaseUrl' JWT_SECRET='$JwtSecret' HTTP_ADDR='$HttpAddr' AUTO_MIGRATE=true MIGRATIONS_DIR='/mnt/c/Users/mjhen/Github/ELNOTE/server/migrations' /tmp/elnote-api"
