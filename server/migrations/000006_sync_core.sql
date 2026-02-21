CREATE TABLE IF NOT EXISTS sync_events (
    cursor BIGSERIAL PRIMARY KEY,
    owner_user_id UUID NOT NULL REFERENCES users(id),
    actor_user_id UUID REFERENCES users(id),
    device_id UUID REFERENCES devices(id),
    event_type TEXT NOT NULL,
    aggregate_type TEXT NOT NULL,
    aggregate_id UUID,
    payload JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_sync_events_owner_cursor
    ON sync_events (owner_user_id, cursor);

CREATE INDEX IF NOT EXISTS idx_sync_events_actor_cursor
    ON sync_events (actor_user_id, cursor)
    WHERE actor_user_id IS NOT NULL;

CREATE TABLE IF NOT EXISTS conflict_artifacts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    owner_user_id UUID NOT NULL REFERENCES users(id),
    actor_user_id UUID REFERENCES users(id),
    device_id UUID REFERENCES devices(id),
    experiment_id UUID NOT NULL REFERENCES experiments(id),
    action_type TEXT NOT NULL,
    client_base_entry_id UUID REFERENCES experiment_entries(id),
    server_latest_entry_id UUID REFERENCES experiment_entries(id),
    payload JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_conflict_artifacts_owner_created
    ON conflict_artifacts (owner_user_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_conflict_artifacts_experiment_created
    ON conflict_artifacts (experiment_id, created_at DESC);

DROP TRIGGER IF EXISTS trg_sync_events_reject_update ON sync_events;
CREATE TRIGGER trg_sync_events_reject_update
BEFORE UPDATE ON sync_events
FOR EACH ROW EXECUTE FUNCTION reject_mutation();

DROP TRIGGER IF EXISTS trg_sync_events_reject_delete ON sync_events;
CREATE TRIGGER trg_sync_events_reject_delete
BEFORE DELETE ON sync_events
FOR EACH ROW EXECUTE FUNCTION reject_mutation();

DROP TRIGGER IF EXISTS trg_conflict_artifacts_reject_update ON conflict_artifacts;
CREATE TRIGGER trg_conflict_artifacts_reject_update
BEFORE UPDATE ON conflict_artifacts
FOR EACH ROW EXECUTE FUNCTION reject_mutation();

DROP TRIGGER IF EXISTS trg_conflict_artifacts_reject_delete ON conflict_artifacts;
CREATE TRIGGER trg_conflict_artifacts_reject_delete
BEFORE DELETE ON conflict_artifacts
FOR EACH ROW EXECUTE FUNCTION reject_mutation();
