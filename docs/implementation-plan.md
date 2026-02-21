# ELNOTE Implementation Plan (Approval Baseline)

## 1) Approved Policy Constraints (Locked)

1. `Owner-only write control`: Only the record owner can create records or addendums for that record.
2. `No deletion or destructive edits`: Once a record is saved, it cannot be deleted or overwritten.
3. `Corrections via addendum only`: Corrections are new addendum entries that can change displayed content while preserving full original content for forensic review.
4. `Admin read/comment/propose only`: Admin cannot create, update, or delete record content.
5. `Admin read scope`: Admin can access completed experiments.
6. `Admin comment scope`: Admin can add comments.
7. `Admin proposal scope`: Admin can propose new experiments linked to prior results.
8. `Large-file policy`: Server stores metadata only; file bytes flow directly between client and NAS object storage.

## 2) Target Architecture

1. Client apps (PC/tablet): Flutter + local SQLite (offline-first).
2. Server API/sync: Go + WebSocket/REST.
3. System database: PostgreSQL for metadata, permissions, changelog, audit, comments, proposals.
4. Large-object storage: NAS-hosted S3-compatible object store (direct upload/download via presigned URLs).

## 3) Permission and Data Governance Model

## Roles

1. Owner can create experiment records.
2. Owner can add addendums to own records.
3. Owner can mark experiments completed.
4. Owner can upload attachments (metadata + object link).
5. Admin can read completed experiments.
6. Admin can add comments.
7. Admin can create experiment proposals.
8. Admin cannot create, update, or delete record content.

## Enforcement

1. API authorization checks for every write path.
2. PostgreSQL role/row policies to block admin writes to record-content tables.
3. DB triggers that reject `UPDATE` and `DELETE` on immutable record tables.
4. Append-only audit events for every content, comment, and proposal action.

## 4) Data Model (Planned)

1. `experiments`: experiment header (owner, status, timestamps).
2. `experiment_entries`: immutable content entries.
3. `entry_type`: `original` or `addendum`.
4. `supersedes_entry_id`: nullable reference for addendum chains.
5. `display_priority`: computed by server for effective view (latest valid addendum wins).
6. `record_comments`: comments on completed experiments (admin and optionally owner).
7. `experiment_proposals`: admin proposals linked to source experiment(s).
8. `attachments`: metadata only (`object_key`, checksum, size, mime, uploader, timestamps).
9. `audit_log`: append-only event table with tamper-evident hash chain (`prev_hash`, `event_hash`).

## 5) API Contract Changes (Compared to generic CRUD)

1. No `DELETE /records/{id}` endpoint.
2. No direct `PATCH` to immutable entry body.
3. `POST /v1/experiments` creates experiment + original entry.
4. `POST /v1/experiments/{id}/addendums` adds correction entry.
5. `GET /v1/experiments/{id}` returns effective displayed content.
6. `GET /v1/experiments/{id}/history` returns original + full addendum chain.
7. `POST /v1/experiments/{id}/comments` for admin comments.
8. `POST /v1/proposals` for admin proposed experiments.
9. Attachment endpoints remain metadata/broker only:
10. `POST /v1/attachments/initiate` (presigned PUT).
11. `POST /v1/attachments/{id}/complete` (checksum/size validation).
12. `GET /v1/attachments/{id}/download` (presigned GET).

## 6) Sprint Roadmap (Execution Sequence)

## Sprint 1 (Week 1-2): Architecture and Contracts Freeze

1. Finalize role matrix and immutable-record rules in architecture docs.
2. Publish OpenAPI with addendum-only correction model.
3. Define audit event schema and retention policy.
4. Exit gate: Architecture sign-off and endpoint contract sign-off.

## Sprint 2 (Week 3-4): Foundation and Auth

1. Repo scaffolding, CI pipeline, base Go service, Postgres migration framework.
2. Auth flows (login/refresh/logout), Argon2id hashing, JWT access tokens.
3. Device registration for sync sessions.
4. Exit gate: Auth tests and service baseline passing in CI.

## Sprint 3 (Week 5-6): Immutable Record Core

1. Implement experiment/original-entry creation flow.
2. Implement addendum creation flow with supersedence links.
3. Block all update/delete paths at API + DB trigger level.
4. Implement effective-content resolver and full history retrieval.
5. Exit gate: Immutable constraints verified by integration tests.

## Sprint 4 (Week 7-8): Admin Comment and Proposal Domain

1. Implement admin-only comment endpoints.
2. Implement admin proposal endpoints linked to completed experiments.
3. Enforce completed-experiment read scope for admin view endpoints.
4. Exit gate: Permission matrix tests pass (admin cannot alter record content).

## Sprint 5 (Week 9-10): Offline Client MVP

1. Flutter UI for experiments, entry history, addendum creation, comments, proposals.
2. Local SQLite schema for offline reads/writes and queueing.
3. UX rule: all corrections are addendums; no edit-in-place affordance.
4. Exit gate: Offline create/addendum flows work locally with replay support.

## Sprint 6 (Week 11-12): Sync v1 (Owner Multi-Device)

1. Outbox + cursor-based sync over WebSocket.
2. Concurrency checks for same owner on multiple devices.
3. Conflict artifacts for stale submissions (never silent overwrite).
4. Exit gate: deterministic sync behavior and conflict visibility tests pass.

## Sprint 7 (Week 13-14): NAS Attachment Integration

1. Presigned URL issue/complete/download workflow.
2. Metadata-only server handling and checksum verification.
3. Reconcile job for metadata/object drift detection.
4. Exit gate: large files transferred client<->NAS without server byte proxy.

## Sprint 8 (Week 15-16): Audit, Reporting, and Security Hardening

1. Full audit trail coverage for content, comments, proposals, auth events.
2. TLS enforcement, permission hardening, backup/PITR drills.
3. Operational dashboards for sync errors, storage reconcile failures, auth events.
4. Exit gate: security and recovery checklist completion.

## Sprint 9 (Week 17-18): Pilot and Production Cutover

1. UAT with representative experiment workflows.
2. Data retention and forensic export validation.
3. Runbook finalization (incident response, restore, key rotation).
4. Exit gate: pilot acceptance + go-live decision.

## 7) Mandatory Acceptance Tests

1. `Immutability`: update/delete SQL and API attempts fail for original records and addendums.
2. `Addendum supersedence`: effective view shows latest addendum; history endpoint returns complete immutable chain.
3. `Role enforcement`: admin cannot create or alter record entries; admin can comment and propose.
4. `Completed-only admin access`: admin can access completed experiments per policy.
5. `Owner-only write`: non-owner users cannot add addendums or attachments to someone else's record.
6. `Sync safety`: same-owner concurrent device writes produce explicit conflict artifacts when stale.
7. `Attachment pipeline`: server never streams file bytes in normal flow; only metadata and signed URL handling.
8. `Forensic audit`: every write-like action has immutable audit event with hash-chain continuity checks.

## 8) Deployment and Operations

1. Docker Compose baseline for server + Postgres (+ optional MinIO test environment).
2. NAS object store deployed separately; credentials stored server-side only.
3. Postgres backups use base backup + WAL archiving (PITR-ready).
4. NAS backups use snapshot/versioning per appliance capability.
5. Restore drill cadence is monthly.

## 9) Immediate Next Build Steps (Now)

1. Create monorepo skeleton (`server`, `client_flutter`, `docs`, `infra`).
2. Draft `docs/api-openapi.yaml` with immutable/addendum endpoints.
3. Implement Sprint 2 baseline auth and migration scaffolding.
4. Implement Sprint 3 immutable record core before any advanced sync features.

### Execution Status (February 21, 2026)

1. [x] Monorepo skeleton created (`server`, `client_flutter`, `docs`, `infra`).
2. [x] OpenAPI contract drafted at `docs/api-openapi.yaml`.
3. [x] Sprint 2 baseline implemented in `server/`:
   - Go service bootstrap, config loader, Postgres connection, migration runner.
   - Auth endpoints (`login`, `refresh`, `logout`) with Argon2id password verification and JWT access tokens.
4. [x] Sprint 3 immutable core implemented in `server/`:
   - Experiment creation (`original` entry), addendum creation (`supersedes_entry_id` chain), complete action.
   - Effective-view and full-history endpoints.
   - DB immutability triggers for non-destructive record policy and append-only audit log protections.
5. [x] Sprint 4 domains (`comments`, `proposals`) are implemented.

## 10) Sprint 4-6 Execution Status (February 21, 2026)

1. [x] Sprint 4 backend domain implemented:
   - Admin-only comment create/list endpoints.
   - Admin-only proposal create/list endpoints linked to source experiments.
   - Completed-experiment policy enforcement for admin read/comment/propose scope.
2. [x] Sprint 6 backend sync core implemented:
   - Cursor-based sync event table and pull endpoint.
   - WebSocket sync endpoint for event/heartbeat streaming.
   - Stale addendum conflict artifact generation with explicit `409` response payload.
3. [x] Sprint 5 Flutter offline MVP scaffold implemented:
   - Local SQLite schema for experiments/entries/comments/proposals/outbox/conflicts/cursor.
   - Outbox replay flow and sync pull/WebSocket client integration.
   - UI for experiments, immutable history, addendum creation, comments, proposals, and conflict visibility.
4. [x] Sprint 7 attachment workflow implemented:
   - Metadata-first attachment lifecycle (`initiate`, `complete`, `download`) with signed URL broker service.
   - Owner-only attachment write enforcement and completed-only download semantics for admin review scope.
   - Attachment reconcile run/findings model for stale initiated records and completed-missing-checksum detection.

## 11) Sprint 7-9 Execution Status (February 21, 2026)

1. [x] Sprint 7 NAS attachment integration implemented in backend:
   - API endpoints wired: `POST /v1/attachments/initiate`, `POST /v1/attachments/{id}/complete`, `GET /v1/attachments/{id}/download`.
   - Storage flow remains metadata-only in API server; file bytes are handled via signed URLs against object storage.
   - DB hardening migration added for attachment immutability and reconcile tracking tables.
2. [x] Sprint 8 audit/reporting/security hardening implemented in backend:
   - TLS gate support (`REQUIRE_TLS`) added at HTTP entrypoint.
   - Ops endpoints added for dashboard counters, audit hash-chain verification, and attachment reconcile runs.
   - Audit trail expanded for attachment lifecycle and forensic export operations.
3. [x] Sprint 9 pilot/cutover readiness baseline implemented:
   - Forensic export endpoint implemented for completed experiments with immutable-chain evidence payload.
   - API contract updated in `docs/api-openapi.yaml` for attachments and ops endpoints.
   - Server operator config/docs updated (`server/.env.example`, `server/README.md`) for Sprint 7-9 runtime controls.
   - Runbook baseline added at `docs/operations-runbook.md` (incident response, PITR drill, key rotation).
