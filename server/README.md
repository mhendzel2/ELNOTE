# ELNOTE Server

Go API service for immutable experiment records.

## Quick Start

1. Start Postgres (example in `../infra/docker-compose.yml`).
2. Create password hashes:

```bash
go run ./cmd/hashpass -password 'owner-password'
go run ./cmd/hashpass -password 'admin-password'
```

3. Insert users (example):

```sql
INSERT INTO users (email, password_hash, role)
VALUES
  ('owner@example.com', '<owner hash>', 'owner'),
  ('admin@example.com', '<admin hash>', 'admin');
```

4. Export environment variables.
5. Run the service from this directory:

```bash
go run ./cmd/api
```

## Environment Variables

- `DATABASE_URL` (required)
- `JWT_SECRET` (required)
- `HTTP_ADDR` (default `:8080`)
- `JWT_ISSUER` (default `elnote-api`)
- `ACCESS_TOKEN_TTL` (default `15m`)
- `REFRESH_TOKEN_TTL` (default `720h`)
- `MIGRATIONS_DIR` (default `./migrations`)
- `AUTO_MIGRATE` (default `true`)
- `REQUIRE_TLS` (default `false`; when `true`, all routes except `/healthz` require HTTPS or `X-Forwarded-Proto: https`)
- `OBJECT_STORE_PUBLIC_BASE_URL` (default `http://localhost:9000`)
- `OBJECT_STORE_BUCKET` (default `elnote`)
- `OBJECT_STORE_SIGN_SECRET` (default falls back to `JWT_SECRET`)
- `OBJECT_STORE_INVENTORY_URL` (optional; JSON inventory endpoint used for orphan-object drift checks)
- `OBJECT_STORE_PROBE_TIMEOUT` (default `10s`)
- `ATTACHMENT_UPLOAD_URL_TTL` (default `15m`)
- `ATTACHMENT_DOWNLOAD_URL_TTL` (default `15m`)
- `RECONCILE_STALE_AFTER` (default `24h`)
- `RECONCILE_SCAN_LIMIT` (default `500`)
- `RECONCILE_SCHEDULE_ENABLED` (default `true`)
- `RECONCILE_SCHEDULE_INTERVAL` (default `24h`)
- `RECONCILE_SCHEDULE_RUN_ON_STARTUP` (default `false`)
- `RECONCILE_SCHEDULE_ACTOR_EMAIL` (default `labadmin`)

## Implemented API Scope

1. Auth: `POST /v1/auth/login`, `POST /v1/auth/refresh`, `POST /v1/auth/logout`.
2. Immutable experiments:
   - `POST /v1/experiments`
   - `POST /v1/experiments/{id}/addendums` (supports `baseEntryId` stale-write detection)
   - `POST /v1/experiments/{id}/complete`
   - `GET /v1/experiments/{id}`
   - `GET /v1/experiments/{id}/history`
3. Admin comment/proposal domain:
   - `POST /v1/experiments/{id}/comments`
   - `GET /v1/experiments/{id}/comments`
   - `POST /v1/proposals`
   - `GET /v1/proposals?sourceExperimentId=<uuid>`
4. Sync v1:
   - `GET /v1/sync/pull?cursor=<n>&limit=<n>`
   - `GET /v1/sync/conflicts`
   - `GET /v1/sync/ws` (WebSocket)
5. Attachment metadata + signed URL broker:
   - `POST /v1/attachments/initiate`
   - `POST /v1/attachments/{id}/complete`
   - `GET /v1/attachments/{id}/download`
6. Ops/security/forensic endpoints (admin):
   - `GET /v1/ops/dashboard`
   - `GET /v1/ops/audit/verify`
   - `POST /v1/ops/attachments/reconcile`
   - `GET /v1/ops/forensic/export?experimentId=<uuid>`

## Automated Restore Drill

Run a logical backup/restore drill and write timestamped evidence:

```bash
DATABASE_URL='postgres://elnote:elnote@localhost:5432/elnote?sslmode=disable' \
  ../scripts/run_pitr_drill.sh
```

Artifacts are written to `docs/drills/pitr/` by default.

## Automated Object-Storage Drill

Run the object-storage validation drill and write timestamped evidence:

```bash
DATABASE_URL='postgres://elnote:elnote@localhost:5432/elnote?sslmode=disable' \
OBJECT_DRILL_ADMIN_PASSWORD='<admin password>' \
  ../scripts/run_object_storage_drill.sh
```

Artifacts are written to `docs/drills/object-storage/` by default.
