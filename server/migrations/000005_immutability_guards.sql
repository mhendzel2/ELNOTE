CREATE OR REPLACE FUNCTION reject_mutation()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
BEGIN
    RAISE EXCEPTION '% on % is not allowed', TG_OP, TG_TABLE_NAME USING ERRCODE = '55000';
END;
$$;

CREATE OR REPLACE FUNCTION reject_delete()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
BEGIN
    RAISE EXCEPTION 'DELETE on % is not allowed', TG_TABLE_NAME USING ERRCODE = '55000';
END;
$$;

CREATE OR REPLACE FUNCTION enforce_experiment_entry_rules()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
DECLARE
    supersedes_experiment_id UUID;
BEGIN
    IF NEW.entry_type = 'original' THEN
        IF NEW.supersedes_entry_id IS NOT NULL THEN
            RAISE EXCEPTION 'original entry cannot supersede another entry' USING ERRCODE = '55000';
        END IF;
    ELSIF NEW.entry_type = 'addendum' THEN
        IF NEW.supersedes_entry_id IS NULL THEN
            RAISE EXCEPTION 'addendum entry must supersede a prior entry' USING ERRCODE = '55000';
        END IF;

        SELECT experiment_id
        INTO supersedes_experiment_id
        FROM experiment_entries
        WHERE id = NEW.supersedes_entry_id;

        IF supersedes_experiment_id IS NULL THEN
            RAISE EXCEPTION 'supersedes_entry_id does not exist' USING ERRCODE = '55000';
        END IF;

        IF supersedes_experiment_id <> NEW.experiment_id THEN
            RAISE EXCEPTION 'addendum must supersede an entry in the same experiment' USING ERRCODE = '55000';
        END IF;
    END IF;

    RETURN NEW;
END;
$$;

CREATE OR REPLACE FUNCTION enforce_experiment_update_rules()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
BEGIN
    IF NEW.owner_user_id <> OLD.owner_user_id THEN
        RAISE EXCEPTION 'owner_user_id is immutable' USING ERRCODE = '55000';
    END IF;

    IF NEW.title <> OLD.title THEN
        RAISE EXCEPTION 'title is immutable after creation' USING ERRCODE = '55000';
    END IF;

    IF OLD.status = 'completed' AND NEW.status <> 'completed' THEN
        RAISE EXCEPTION 'completed experiments cannot move back to draft' USING ERRCODE = '55000';
    END IF;

    IF NEW.status = 'completed' AND NEW.completed_at IS NULL THEN
        RAISE EXCEPTION 'completed experiments must have completed_at' USING ERRCODE = '55000';
    END IF;

    RETURN NEW;
END;
$$;

DROP TRIGGER IF EXISTS trg_experiment_entries_reject_update ON experiment_entries;
CREATE TRIGGER trg_experiment_entries_reject_update
BEFORE UPDATE ON experiment_entries
FOR EACH ROW EXECUTE FUNCTION reject_mutation();

DROP TRIGGER IF EXISTS trg_experiment_entries_reject_delete ON experiment_entries;
CREATE TRIGGER trg_experiment_entries_reject_delete
BEFORE DELETE ON experiment_entries
FOR EACH ROW EXECUTE FUNCTION reject_mutation();

DROP TRIGGER IF EXISTS trg_experiments_reject_delete ON experiments;
CREATE TRIGGER trg_experiments_reject_delete
BEFORE DELETE ON experiments
FOR EACH ROW EXECUTE FUNCTION reject_delete();

DROP TRIGGER IF EXISTS trg_audit_log_reject_update ON audit_log;
CREATE TRIGGER trg_audit_log_reject_update
BEFORE UPDATE ON audit_log
FOR EACH ROW EXECUTE FUNCTION reject_mutation();

DROP TRIGGER IF EXISTS trg_audit_log_reject_delete ON audit_log;
CREATE TRIGGER trg_audit_log_reject_delete
BEFORE DELETE ON audit_log
FOR EACH ROW EXECUTE FUNCTION reject_mutation();

DROP TRIGGER IF EXISTS trg_record_comments_reject_update ON record_comments;
CREATE TRIGGER trg_record_comments_reject_update
BEFORE UPDATE ON record_comments
FOR EACH ROW EXECUTE FUNCTION reject_mutation();

DROP TRIGGER IF EXISTS trg_record_comments_reject_delete ON record_comments;
CREATE TRIGGER trg_record_comments_reject_delete
BEFORE DELETE ON record_comments
FOR EACH ROW EXECUTE FUNCTION reject_mutation();

DROP TRIGGER IF EXISTS trg_experiment_proposals_reject_update ON experiment_proposals;
CREATE TRIGGER trg_experiment_proposals_reject_update
BEFORE UPDATE ON experiment_proposals
FOR EACH ROW EXECUTE FUNCTION reject_mutation();

DROP TRIGGER IF EXISTS trg_experiment_proposals_reject_delete ON experiment_proposals;
CREATE TRIGGER trg_experiment_proposals_reject_delete
BEFORE DELETE ON experiment_proposals
FOR EACH ROW EXECUTE FUNCTION reject_mutation();

DROP TRIGGER IF EXISTS trg_experiment_entries_insert_rules ON experiment_entries;
CREATE TRIGGER trg_experiment_entries_insert_rules
BEFORE INSERT ON experiment_entries
FOR EACH ROW EXECUTE FUNCTION enforce_experiment_entry_rules();

DROP TRIGGER IF EXISTS trg_experiments_update_rules ON experiments;
CREATE TRIGGER trg_experiments_update_rules
BEFORE UPDATE ON experiments
FOR EACH ROW EXECUTE FUNCTION enforce_experiment_update_rules();
