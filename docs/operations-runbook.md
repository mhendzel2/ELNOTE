# ELNOTE Operations Runbook (Sprint 9 Baseline)

## 1) Purpose

This runbook defines the minimum operational procedures for incident response, forensic export, backup/restore drills, and key rotation for ELNOTE production environments.

## 2) Daily/Per-Shift Checks

1. Verify service health:
   - `GET /healthz` returns `200`.
2. Review ops dashboard:
   - `GET /v1/ops/dashboard` as admin.
   - Watch for spikes in `syncConflicts24h`, `reconcileFindingsUnresolved`, and auth event anomalies.
3. Verify audit integrity:
   - `GET /v1/ops/audit/verify` as admin.
   - Escalate immediately if response is `409` or `valid=false`.
4. Run attachment reconcile (or confirm scheduled run):
   - `POST /v1/ops/attachments/reconcile` with `{}` (defaults) or scoped parameters.
   - Investigate unresolved findings.

## 3) Incident Response

1. Declare incident and assign roles:
   - Incident Commander
   - Communications Lead
   - Technical Lead
2. Preserve evidence:
   - Capture timestamp window, request IDs, and relevant logs.
   - Export forensic bundles for impacted experiments using:
     - `GET /v1/ops/forensic/export?experimentId=<uuid>`
3. Containment:
   - Restrict external access if compromise is suspected.
   - Force secret rotation if token/signing key exposure is possible.
4. Eradication and recovery:
   - Patch root cause.
   - Restore from known-good snapshot/PITR target if data integrity is in doubt.
5. Closure:
   - Run `GET /v1/ops/audit/verify` and attachment reconcile after recovery.
   - Publish incident summary and corrective actions.

## 4) Postgres Backup and PITR Drill (Monthly)

1. Confirm base backup and WAL archiving are healthy.
2. Choose a recent timestamp target and perform restore to a staging environment.
3. Validate restored instance:
   - Core tables present and queryable.
   - `audit_log` hash-chain verification passes.
4. Record drill evidence:
   - Start/end timestamps
   - Recovery point target
   - Verification results
   - Gaps and action items

## 5) Object Storage Backup Drill (Monthly)

1. Confirm NAS/object-store snapshot or versioning jobs completed successfully.
2. Restore a sample attachment object in staging.
3. Validate metadata/object consistency:
   - Attachment metadata exists in Postgres.
   - Object is retrievable with signed URL flow.

## 6) Key Rotation (Quarterly or on Incident)

1. Rotate `JWT_SECRET`:
   - Deploy new secret through secret manager.
   - Restart services.
   - Re-authenticate active sessions as needed.
2. Rotate `OBJECT_STORE_SIGN_SECRET`:
   - Deploy new secret.
   - Restart API service.
   - Validate attachment initiate/download URL generation.
3. Document rotation:
   - Ticket ID
   - Rotation timestamp
   - Operator
   - Verification outcomes

## 7) Pilot/UAT Readiness Checklist

1. Representative owner/admin workflows complete without policy violations.
2. Audit verification endpoint passes continuously.
3. Reconcile findings are triaged and resolved.
4. Backup restore drill evidence is current.
5. Key rotation procedure validated in non-production.
