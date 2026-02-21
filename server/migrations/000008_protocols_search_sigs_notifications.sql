-- 000008_protocols_search_sigs_notifications.sql
-- Adds: protocols/SOPs, tags, digital signatures, notifications,
--        data extracts, chart configs, attachment previews, templates,
--        full-text search indexes.

-- ============================================================
-- 1. Protocols / SOPs
-- ============================================================
CREATE TABLE protocols (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    owner_user_id   UUID        NOT NULL REFERENCES users(id),
    title           TEXT        NOT NULL,
    description     TEXT        NOT NULL DEFAULT '',
    status          TEXT        NOT NULL DEFAULT 'draft'
                    CHECK (status IN ('draft','published','archived')),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_protocols_owner ON protocols(owner_user_id);
CREATE INDEX idx_protocols_status ON protocols(status);

CREATE TABLE protocol_versions (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    protocol_id     UUID        NOT NULL REFERENCES protocols(id) ON DELETE RESTRICT,
    version_number  INTEGER     NOT NULL,
    body            TEXT        NOT NULL,
    change_summary  TEXT        NOT NULL DEFAULT '',
    author_user_id  UUID        NOT NULL REFERENCES users(id),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(protocol_id, version_number)
);

CREATE INDEX idx_protocol_versions_protocol ON protocol_versions(protocol_id, version_number);

-- Link experiments to protocols with deviation tracking
CREATE TABLE experiment_protocols (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    experiment_id       UUID        NOT NULL REFERENCES experiments(id) ON DELETE RESTRICT,
    protocol_id         UUID        NOT NULL REFERENCES protocols(id) ON DELETE RESTRICT,
    protocol_version_id UUID        NOT NULL REFERENCES protocol_versions(id) ON DELETE RESTRICT,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(experiment_id)
);

CREATE TABLE protocol_deviations (
    id                      UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    experiment_id           UUID        NOT NULL REFERENCES experiments(id) ON DELETE RESTRICT,
    experiment_entry_id     UUID        NOT NULL REFERENCES experiment_entries(id) ON DELETE RESTRICT,
    deviation_type          TEXT        NOT NULL
                            CHECK (deviation_type IN ('planned','unplanned','observation')),
    rationale               TEXT        NOT NULL,
    created_at              TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_protocol_deviations_experiment ON protocol_deviations(experiment_id, created_at);

-- Immutability triggers for protocol tables
CREATE TRIGGER trg_protocol_versions_reject_update
  BEFORE UPDATE ON protocol_versions FOR EACH ROW EXECUTE FUNCTION reject_mutation();
CREATE TRIGGER trg_protocol_versions_reject_delete
  BEFORE DELETE ON protocol_versions FOR EACH ROW EXECUTE FUNCTION reject_mutation();

CREATE TRIGGER trg_protocol_deviations_reject_update
  BEFORE UPDATE ON protocol_deviations FOR EACH ROW EXECUTE FUNCTION reject_mutation();
CREATE TRIGGER trg_protocol_deviations_reject_delete
  BEFORE DELETE ON protocol_deviations FOR EACH ROW EXECUTE FUNCTION reject_mutation();

CREATE TRIGGER trg_experiment_protocols_reject_update
  BEFORE UPDATE ON experiment_protocols FOR EACH ROW EXECUTE FUNCTION reject_mutation();
CREATE TRIGGER trg_experiment_protocols_reject_delete
  BEFORE DELETE ON experiment_protocols FOR EACH ROW EXECUTE FUNCTION reject_mutation();

-- ============================================================
-- 2. Tags
-- ============================================================
CREATE TABLE tags (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name        TEXT NOT NULL,
    owner_user_id UUID NOT NULL REFERENCES users(id),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(owner_user_id, name)
);

CREATE TABLE experiment_tags (
    experiment_id UUID NOT NULL REFERENCES experiments(id) ON DELETE RESTRICT,
    tag_id        UUID NOT NULL REFERENCES tags(id) ON DELETE CASCADE,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (experiment_id, tag_id)
);

CREATE INDEX idx_experiment_tags_tag ON experiment_tags(tag_id);

-- ============================================================
-- 3. Digital Signatures & Witnessing
-- ============================================================
CREATE TABLE experiment_signatures (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    experiment_id   UUID        NOT NULL REFERENCES experiments(id) ON DELETE RESTRICT,
    signer_user_id  UUID        NOT NULL REFERENCES users(id),
    signature_type  TEXT        NOT NULL
                    CHECK (signature_type IN ('author','witness')),
    content_hash    TEXT        NOT NULL,   -- SHA-256 of effective body at signing time
    signed_at       TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_experiment_signatures_experiment
  ON experiment_signatures(experiment_id, signed_at);

CREATE TRIGGER trg_experiment_signatures_reject_update
  BEFORE UPDATE ON experiment_signatures FOR EACH ROW EXECUTE FUNCTION reject_mutation();
CREATE TRIGGER trg_experiment_signatures_reject_delete
  BEFORE DELETE ON experiment_signatures FOR EACH ROW EXECUTE FUNCTION reject_mutation();

-- ============================================================
-- 4. Notifications & Activity Feed
-- ============================================================
CREATE TABLE notifications (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id         UUID        NOT NULL REFERENCES users(id),
    event_type      TEXT        NOT NULL,
    title           TEXT        NOT NULL,
    body            TEXT        NOT NULL DEFAULT '',
    reference_type  TEXT        NOT NULL DEFAULT '',
    reference_id    UUID,
    read_at         TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_notifications_user_unread
  ON notifications(user_id, created_at DESC) WHERE read_at IS NULL;
CREATE INDEX idx_notifications_user_all
  ON notifications(user_id, created_at DESC);

-- ============================================================
-- 5. Data Extracts & Chart Configs (for CSV/Excel visualization)
-- ============================================================
CREATE TABLE data_extracts (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    attachment_id   UUID        NOT NULL REFERENCES attachments(id) ON DELETE RESTRICT,
    experiment_id   UUID        NOT NULL REFERENCES experiments(id) ON DELETE RESTRICT,
    column_headers  JSONB       NOT NULL DEFAULT '[]',
    row_count       INTEGER     NOT NULL DEFAULT 0,
    sample_rows     JSONB       NOT NULL DEFAULT '[]',
    parsed_at       TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_data_extracts_attachment ON data_extracts(attachment_id);
CREATE INDEX idx_data_extracts_experiment ON data_extracts(experiment_id);

CREATE TABLE chart_configs (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    experiment_id   UUID        NOT NULL REFERENCES experiments(id) ON DELETE RESTRICT,
    data_extract_id UUID        NOT NULL REFERENCES data_extracts(id) ON DELETE RESTRICT,
    creator_user_id UUID        NOT NULL REFERENCES users(id),
    chart_type      TEXT        NOT NULL
                    CHECK (chart_type IN ('line','scatter','bar','histogram','area')),
    title           TEXT        NOT NULL DEFAULT '',
    x_column        TEXT        NOT NULL,
    y_columns       JSONB       NOT NULL DEFAULT '[]',
    options         JSONB       NOT NULL DEFAULT '{}',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_chart_configs_experiment ON chart_configs(experiment_id);

CREATE TRIGGER trg_chart_configs_reject_update
  BEFORE UPDATE ON chart_configs FOR EACH ROW EXECUTE FUNCTION reject_mutation();
CREATE TRIGGER trg_chart_configs_reject_delete
  BEFORE DELETE ON chart_configs FOR EACH ROW EXECUTE FUNCTION reject_mutation();

-- ============================================================
-- 6. Attachment Previews / Thumbnails
-- ============================================================
CREATE TABLE attachment_previews (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    attachment_id   UUID        NOT NULL REFERENCES attachments(id) ON DELETE RESTRICT,
    preview_type    TEXT        NOT NULL DEFAULT 'thumbnail',
    mime_type       TEXT        NOT NULL DEFAULT 'image/png',
    width           INTEGER     NOT NULL DEFAULT 0,
    height          INTEGER     NOT NULL DEFAULT 0,
    data            BYTEA       NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(attachment_id, preview_type)
);

CREATE TRIGGER trg_attachment_previews_reject_update
  BEFORE UPDATE ON attachment_previews FOR EACH ROW EXECUTE FUNCTION reject_mutation();
CREATE TRIGGER trg_attachment_previews_reject_delete
  BEFORE DELETE ON attachment_previews FOR EACH ROW EXECUTE FUNCTION reject_mutation();

-- ============================================================
-- 7. Experiment Templates (clone / reuse)
-- ============================================================
CREATE TABLE experiment_templates (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    owner_user_id   UUID        NOT NULL REFERENCES users(id),
    title           TEXT        NOT NULL,
    description     TEXT        NOT NULL DEFAULT '',
    body_template   TEXT        NOT NULL,
    sections        JSONB       NOT NULL DEFAULT '[]',
    protocol_id     UUID        REFERENCES protocols(id),
    tags            JSONB       NOT NULL DEFAULT '[]',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_experiment_templates_owner ON experiment_templates(owner_user_id);

-- ============================================================
-- 8. Full-Text Search indexes
-- ============================================================
ALTER TABLE experiments ADD COLUMN IF NOT EXISTS search_vector tsvector;

CREATE INDEX idx_experiments_fts ON experiments USING GIN(search_vector);

-- Trigger to auto-update search vector on experiments
CREATE OR REPLACE FUNCTION experiments_search_update() RETURNS trigger AS $$
BEGIN
  NEW.search_vector := to_tsvector('english', coalesce(NEW.title, ''));
  RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_experiments_search_update
  BEFORE INSERT OR UPDATE OF title ON experiments
  FOR EACH ROW EXECUTE FUNCTION experiments_search_update();

-- Backfill existing rows
UPDATE experiments SET search_vector = to_tsvector('english', coalesce(title, ''));

-- FTS on experiment entries (body content)
ALTER TABLE experiment_entries ADD COLUMN IF NOT EXISTS search_vector tsvector;

CREATE INDEX idx_experiment_entries_fts ON experiment_entries USING GIN(search_vector);

CREATE OR REPLACE FUNCTION experiment_entries_search_update() RETURNS trigger AS $$
BEGIN
  NEW.search_vector := to_tsvector('english', coalesce(NEW.body, ''));
  RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_experiment_entries_search_update
  BEFORE INSERT ON experiment_entries
  FOR EACH ROW EXECUTE FUNCTION experiment_entries_search_update();

-- Backfill (temporarily disable immutability trigger)
ALTER TABLE experiment_entries DISABLE TRIGGER trg_experiment_entries_reject_update;
UPDATE experiment_entries SET search_vector = to_tsvector('english', coalesce(body, ''));
ALTER TABLE experiment_entries ENABLE TRIGGER trg_experiment_entries_reject_update;

-- FTS on protocols
ALTER TABLE protocols ADD COLUMN IF NOT EXISTS search_vector tsvector;

CREATE INDEX idx_protocols_fts ON protocols USING GIN(search_vector);

CREATE OR REPLACE FUNCTION protocols_search_update() RETURNS trigger AS $$
BEGIN
  NEW.search_vector :=
    setweight(to_tsvector('english', coalesce(NEW.title, '')), 'A') ||
    setweight(to_tsvector('english', coalesce(NEW.description, '')), 'B');
  RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_protocols_search_update
  BEFORE INSERT OR UPDATE OF title, description ON protocols
  FOR EACH ROW EXECUTE FUNCTION protocols_search_update();

-- ============================================================
-- 9. Protocol update rules (allow only status + description changes)
-- ============================================================
CREATE OR REPLACE FUNCTION enforce_protocol_update_rules() RETURNS trigger AS $$
BEGIN
  IF NEW.owner_user_id <> OLD.owner_user_id THEN
    RAISE EXCEPTION 'cannot change protocol owner';
  END IF;
  IF NEW.title <> OLD.title THEN
    RAISE EXCEPTION 'cannot change protocol title after creation';
  END IF;
  IF OLD.status = 'archived' AND NEW.status <> 'archived' THEN
    RAISE EXCEPTION 'cannot un-archive a protocol';
  END IF;
  RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_protocols_update_rules
  BEFORE UPDATE ON protocols FOR EACH ROW EXECUTE FUNCTION enforce_protocol_update_rules();

CREATE TRIGGER trg_protocols_reject_delete
  BEFORE DELETE ON protocols FOR EACH ROW EXECUTE FUNCTION reject_delete();
