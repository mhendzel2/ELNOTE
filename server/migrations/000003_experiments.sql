CREATE TABLE IF NOT EXISTS experiments (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    owner_user_id UUID NOT NULL REFERENCES users(id),
    title TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'draft' CHECK (status IN ('draft', 'completed')),
    completed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_experiments_owner_user_id ON experiments(owner_user_id);
CREATE INDEX IF NOT EXISTS idx_experiments_status ON experiments(status);

CREATE TABLE IF NOT EXISTS experiment_entries (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    experiment_id UUID NOT NULL REFERENCES experiments(id) ON DELETE RESTRICT,
    author_user_id UUID NOT NULL REFERENCES users(id),
    entry_type TEXT NOT NULL CHECK (entry_type IN ('original', 'addendum')),
    supersedes_entry_id UUID REFERENCES experiment_entries(id),
    body TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_experiment_single_original_entry
    ON experiment_entries (experiment_id)
    WHERE entry_type = 'original';

CREATE INDEX IF NOT EXISTS idx_experiment_entries_experiment_created
    ON experiment_entries (experiment_id, created_at);
