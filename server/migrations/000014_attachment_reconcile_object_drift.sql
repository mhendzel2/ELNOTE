-- Allow reconcile findings that are not tied to a metadata row (for orphan object findings).
ALTER TABLE attachment_reconcile_findings
    ALTER COLUMN attachment_id DROP NOT NULL;

-- Recreate update-rule function so nullable attachment_id comparisons remain immutable.
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
    IF NEW.attachment_id IS DISTINCT FROM OLD.attachment_id THEN
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
