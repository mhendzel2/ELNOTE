CREATE TABLE IF NOT EXISTS attachment_reconcile_runs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    triggered_by_user_id UUID NOT NULL REFERENCES users(id),
    started_at TIMESTAMPTZ NOT NULL,
    finished_at TIMESTAMPTZ,
    stale_after_seconds BIGINT NOT NULL,
    scan_limit INTEGER NOT NULL,
    stale_initiated_count INTEGER NOT NULL DEFAULT 0,
    missing_checksum_count INTEGER NOT NULL DEFAULT 0,
    total_findings INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_attachment_reconcile_runs_started
    ON attachment_reconcile_runs (started_at DESC);

CREATE TABLE IF NOT EXISTS attachment_reconcile_findings (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    run_id UUID NOT NULL REFERENCES attachment_reconcile_runs(id) ON DELETE RESTRICT,
    attachment_id UUID NOT NULL REFERENCES attachments(id) ON DELETE RESTRICT,
    finding_type TEXT NOT NULL,
    details JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    resolved_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_attachment_reconcile_findings_run
    ON attachment_reconcile_findings (run_id, created_at);

CREATE INDEX IF NOT EXISTS idx_attachment_reconcile_findings_resolved
    ON attachment_reconcile_findings (resolved_at);

CREATE OR REPLACE FUNCTION enforce_attachment_update_rules()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
BEGIN
    IF NEW.experiment_id <> OLD.experiment_id THEN
        RAISE EXCEPTION 'experiment_id is immutable for attachments' USING ERRCODE = '55000';
    END IF;

    IF NEW.uploader_user_id <> OLD.uploader_user_id THEN
        RAISE EXCEPTION 'uploader_user_id is immutable for attachments' USING ERRCODE = '55000';
    END IF;

    IF NEW.object_key <> OLD.object_key THEN
        RAISE EXCEPTION 'object_key is immutable for attachments' USING ERRCODE = '55000';
    END IF;

    IF NEW.size_bytes <> OLD.size_bytes THEN
        RAISE EXCEPTION 'size_bytes is immutable for attachments' USING ERRCODE = '55000';
    END IF;

    IF NEW.mime_type <> OLD.mime_type THEN
        RAISE EXCEPTION 'mime_type is immutable for attachments' USING ERRCODE = '55000';
    END IF;

    IF OLD.status = 'completed' THEN
        IF NEW.status <> 'completed' THEN
            RAISE EXCEPTION 'completed attachments are immutable' USING ERRCODE = '55000';
        END IF;

        IF COALESCE(NEW.checksum, '') <> COALESCE(OLD.checksum, '') THEN
            RAISE EXCEPTION 'checksum cannot be changed after completion' USING ERRCODE = '55000';
        END IF;

        IF NEW.completed_at IS DISTINCT FROM OLD.completed_at THEN
            RAISE EXCEPTION 'completed_at cannot be changed after completion' USING ERRCODE = '55000';
        END IF;

        RETURN NEW;
    END IF;

    IF NEW.status NOT IN ('initiated', 'completed') THEN
        RAISE EXCEPTION 'invalid attachment status transition' USING ERRCODE = '55000';
    END IF;

    IF NEW.status = 'initiated' THEN
        IF COALESCE(BTRIM(NEW.checksum), '') <> '' THEN
            RAISE EXCEPTION 'initiated attachments must not include checksum' USING ERRCODE = '55000';
        END IF;
        IF NEW.completed_at IS NOT NULL THEN
            RAISE EXCEPTION 'initiated attachments must not include completed_at' USING ERRCODE = '55000';
        END IF;
    END IF;

    IF NEW.status = 'completed' THEN
        IF NEW.completed_at IS NULL THEN
            RAISE EXCEPTION 'completed attachments must set completed_at' USING ERRCODE = '55000';
        END IF;
        IF COALESCE(BTRIM(NEW.checksum), '') = '' THEN
            RAISE EXCEPTION 'completed attachments must include checksum' USING ERRCODE = '55000';
        END IF;
    END IF;

    RETURN NEW;
END;
$$;

CREATE OR REPLACE FUNCTION enforce_attachment_reconcile_run_update_rules()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
BEGIN
    IF NEW.id <> OLD.id THEN
        RAISE EXCEPTION 'reconcile run id is immutable' USING ERRCODE = '55000';
    END IF;
    IF NEW.triggered_by_user_id <> OLD.triggered_by_user_id THEN
        RAISE EXCEPTION 'triggered_by_user_id is immutable' USING ERRCODE = '55000';
    END IF;
    IF NEW.started_at <> OLD.started_at THEN
        RAISE EXCEPTION 'started_at is immutable' USING ERRCODE = '55000';
    END IF;
    IF NEW.stale_after_seconds <> OLD.stale_after_seconds THEN
        RAISE EXCEPTION 'stale_after_seconds is immutable' USING ERRCODE = '55000';
    END IF;
    IF NEW.scan_limit <> OLD.scan_limit THEN
        RAISE EXCEPTION 'scan_limit is immutable' USING ERRCODE = '55000';
    END IF;
    IF NEW.created_at <> OLD.created_at THEN
        RAISE EXCEPTION 'created_at is immutable' USING ERRCODE = '55000';
    END IF;

    RETURN NEW;
END;
$$;

CREATE OR REPLACE FUNCTION enforce_attachment_reconcile_finding_update_rules()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
BEGIN
    IF NEW.id <> OLD.id THEN
        RAISE EXCEPTION 'finding id is immutable' USING ERRCODE = '55000';
    END IF;
    IF NEW.run_id <> OLD.run_id THEN
        RAISE EXCEPTION 'run_id is immutable' USING ERRCODE = '55000';
    END IF;
    IF NEW.attachment_id <> OLD.attachment_id THEN
        RAISE EXCEPTION 'attachment_id is immutable' USING ERRCODE = '55000';
    END IF;
    IF NEW.finding_type <> OLD.finding_type THEN
        RAISE EXCEPTION 'finding_type is immutable' USING ERRCODE = '55000';
    END IF;
    IF NEW.details <> OLD.details THEN
        RAISE EXCEPTION 'details are immutable' USING ERRCODE = '55000';
    END IF;
    IF NEW.created_at <> OLD.created_at THEN
        RAISE EXCEPTION 'created_at is immutable' USING ERRCODE = '55000';
    END IF;

    IF OLD.resolved_at IS NOT NULL AND NEW.resolved_at <> OLD.resolved_at THEN
        RAISE EXCEPTION 'resolved_at cannot be changed after being set' USING ERRCODE = '55000';
    END IF;

    RETURN NEW;
END;
$$;

DROP TRIGGER IF EXISTS trg_attachments_update_rules ON attachments;
CREATE TRIGGER trg_attachments_update_rules
BEFORE UPDATE ON attachments
FOR EACH ROW EXECUTE FUNCTION enforce_attachment_update_rules();

DROP TRIGGER IF EXISTS trg_attachments_reject_delete ON attachments;
CREATE TRIGGER trg_attachments_reject_delete
BEFORE DELETE ON attachments
FOR EACH ROW EXECUTE FUNCTION reject_mutation();

DROP TRIGGER IF EXISTS trg_attachment_reconcile_runs_reject_update ON attachment_reconcile_runs;
CREATE TRIGGER trg_attachment_reconcile_runs_reject_update
BEFORE UPDATE ON attachment_reconcile_runs
FOR EACH ROW EXECUTE FUNCTION enforce_attachment_reconcile_run_update_rules();

DROP TRIGGER IF EXISTS trg_attachment_reconcile_runs_reject_delete ON attachment_reconcile_runs;
CREATE TRIGGER trg_attachment_reconcile_runs_reject_delete
BEFORE DELETE ON attachment_reconcile_runs
FOR EACH ROW EXECUTE FUNCTION reject_mutation();

DROP TRIGGER IF EXISTS trg_attachment_reconcile_findings_reject_update ON attachment_reconcile_findings;
CREATE TRIGGER trg_attachment_reconcile_findings_reject_update
BEFORE UPDATE ON attachment_reconcile_findings
FOR EACH ROW EXECUTE FUNCTION enforce_attachment_reconcile_finding_update_rules();

DROP TRIGGER IF EXISTS trg_attachment_reconcile_findings_reject_delete ON attachment_reconcile_findings;
CREATE TRIGGER trg_attachment_reconcile_findings_reject_delete
BEFORE DELETE ON attachment_reconcile_findings
FOR EACH ROW EXECUTE FUNCTION reject_mutation();
