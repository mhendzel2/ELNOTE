# ELNOTE Infrastructure

Contains local and deployment infrastructure definitions.

## Local Dev Stack

`docker-compose.yml` starts:

- `postgres` (database)
- `minio` + `minio-init` (object storage and bucket bootstrap)
- `mailhog` (SMTP sink + web UI)
- `api` (Go server from repo source)

### Run

```bash
cd infra
docker compose up --build
```

### Access

- API: `http://localhost:8080`
- MinIO S3 API: `http://localhost:9000`
- MinIO Console: `http://localhost:9001` (`minioadmin` / `minioadmin`)
- MailHog SMTP: `localhost:1025`
- MailHog UI: `http://localhost:8025`

### Admin Bootstrap

- Optional: set `INITIAL_ADMIN_PASSWORD` before `docker compose up`.
- If unset, the server generates a LabAdmin bootstrap password and prints it once in API logs.
- Write the password down immediately. Outside local-dev reset mode, lost admin credentials are not recoverable.
