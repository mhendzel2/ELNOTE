CREATE TABLE IF NOT EXISTS record_comments (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    experiment_id UUID NOT NULL REFERENCES experiments(id) ON DELETE RESTRICT,
    author_user_id UUID NOT NULL REFERENCES users(id),
    body TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_record_comments_experiment_id
    ON record_comments (experiment_id, created_at);

CREATE TABLE IF NOT EXISTS experiment_proposals (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    source_experiment_id UUID NOT NULL REFERENCES experiments(id) ON DELETE RESTRICT,
    proposer_user_id UUID NOT NULL REFERENCES users(id),
    title TEXT NOT NULL,
    body TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_experiment_proposals_source_experiment_id
    ON experiment_proposals (source_experiment_id, created_at);

CREATE TABLE IF NOT EXISTS attachments (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    experiment_id UUID NOT NULL REFERENCES experiments(id) ON DELETE RESTRICT,
    uploader_user_id UUID NOT NULL REFERENCES users(id),
    object_key TEXT NOT NULL UNIQUE,
    checksum TEXT,
    size_bytes BIGINT NOT NULL,
    mime_type TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'initiated' CHECK (status IN ('initiated', 'completed')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    completed_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_attachments_experiment_id
    ON attachments (experiment_id);

CREATE TABLE IF NOT EXISTS audit_log (
    id BIGSERIAL PRIMARY KEY,
    event_id UUID NOT NULL DEFAULT gen_random_uuid(),
    actor_user_id UUID REFERENCES users(id),
    event_type TEXT NOT NULL,
    entity_type TEXT NOT NULL,
    entity_id UUID,
    payload JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    prev_hash BYTEA,
    event_hash BYTEA NOT NULL
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_audit_log_event_id ON audit_log(event_id);
CREATE INDEX IF NOT EXISTS idx_audit_log_created_at ON audit_log(created_at);
CREATE INDEX IF NOT EXISTS idx_audit_log_event_type ON audit_log(event_type);
