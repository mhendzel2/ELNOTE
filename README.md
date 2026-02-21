# ELNOTE

Implementation scaffold for the immutable experiment record platform described in `docs/implementation-plan.md`.

## Repository Layout

- `server/`: Go API service (auth, immutable experiment core, migrations).
- `client_flutter/`: Flutter offline-first MVP (Sprint 5 scope).
- `docs/`: Architecture, API contract, and operations runbook docs.
- `infra/`: Local infrastructure definitions (Postgres compose).

## Current Build Scope

Implemented from the immediate plan steps:

1. Monorepo skeleton.
2. OpenAPI draft (`docs/api-openapi.yaml`).
3. Sprint 2 baseline auth + migration scaffolding.
4. Sprint 3 immutable experiment core endpoints.

Extended implementation:

5. Sprint 4 admin comments/proposals endpoints and completed-experiment scope checks.
6. Sprint 5 Flutter offline MVP scaffold with SQLite outbox and replay.
7. Sprint 6 sync foundations (cursor pull, WebSocket updates, stale-write conflict artifacts).
8. Sprint 7 attachment metadata/signed URL workflow + reconcile tracking.
9. Sprint 8 ops/security hardening (TLS gate, dashboard, audit hash-chain verification).
10. Sprint 9 forensic export path and operations runbook baseline.
