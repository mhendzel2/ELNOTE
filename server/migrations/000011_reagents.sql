-- ============================================================
-- Reagent Database Migration (from MHLabmanagement2023N.mdb)
-- MUTABLE tables: all authenticated users may INSERT/UPDATE/DELETE
-- ============================================================

-- Storage locations master table
CREATE TABLE IF NOT EXISTS reagent_storage (
    id            SERIAL PRIMARY KEY,
    name          TEXT NOT NULL,
    location_type TEXT,
    description   TEXT,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_by    UUID REFERENCES users(id)
);

-- Box/container registry
CREATE TABLE IF NOT EXISTS reagent_box (
    id          SERIAL PRIMARY KEY,
    box_no      TEXT NOT NULL UNIQUE,
    box_type    TEXT,
    owner       TEXT,
    label       TEXT,
    location    TEXT,
    drawer      TEXT,
    position    TEXT,
    storage_id  INT REFERENCES reagent_storage(id),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_by  UUID REFERENCES users(id)
);

-- Antibodies
CREATE TABLE IF NOT EXISTS reagent_antibody (
    id             SERIAL PRIMARY KEY,
    antibody_name  TEXT NOT NULL,
    catalog_no     TEXT,
    company        TEXT,
    class          TEXT,
    antigen        TEXT,
    host           TEXT,
    investigator   TEXT,
    exp_id         TEXT,
    notes          TEXT,
    box_id         INT REFERENCES reagent_box(id),
    location       TEXT,
    quantity       TEXT,
    is_depleted    BOOLEAN NOT NULL DEFAULT FALSE,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_by     UUID REFERENCES users(id)
);

-- Cell lines
CREATE TABLE IF NOT EXISTS reagent_cell_line (
    id              SERIAL PRIMARY KEY,
    cell_line_name  TEXT NOT NULL,
    selection       TEXT,
    species         TEXT,
    parental_cell   TEXT,
    medium          TEXT,
    obtain_from     TEXT,
    cell_type       TEXT,
    box_id          INT REFERENCES reagent_box(id),
    location        TEXT,
    owner           TEXT,
    label           TEXT,
    notes           TEXT,
    is_depleted     BOOLEAN NOT NULL DEFAULT FALSE,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_by      UUID REFERENCES users(id)
);

-- Viruses
CREATE TABLE IF NOT EXISTS reagent_virus (
    id           SERIAL PRIMARY KEY,
    virus_name   TEXT NOT NULL,
    virus_type   TEXT,
    box_id       INT REFERENCES reagent_box(id),
    location     TEXT,
    owner        TEXT,
    label        TEXT,
    notes        TEXT,
    is_depleted  BOOLEAN NOT NULL DEFAULT FALSE,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_by   UUID REFERENCES users(id)
);

-- DNA constructs
CREATE TABLE IF NOT EXISTS reagent_dna (
    id          SERIAL PRIMARY KEY,
    dna_name    TEXT NOT NULL,
    dna_type    TEXT,
    box_id      INT REFERENCES reagent_box(id),
    location    TEXT,
    owner       TEXT,
    label       TEXT,
    notes       TEXT,
    is_depleted BOOLEAN NOT NULL DEFAULT FALSE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_by  UUID REFERENCES users(id)
);

-- Oligonucleotides
CREATE TABLE IF NOT EXISTS reagent_oligo (
    id           SERIAL PRIMARY KEY,
    oligo_name   TEXT NOT NULL,
    sequence     TEXT,
    oligo_type   TEXT,
    box_id       INT REFERENCES reagent_box(id),
    location     TEXT,
    owner        TEXT,
    label        TEXT,
    notes        TEXT,
    is_depleted  BOOLEAN NOT NULL DEFAULT FALSE,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_by   UUID REFERENCES users(id)
);

-- Chemicals
CREATE TABLE IF NOT EXISTS reagent_chemical (
    id            SERIAL PRIMARY KEY,
    chemical_name TEXT NOT NULL,
    catalog_no    TEXT,
    company       TEXT,
    chem_type     TEXT,
    box_id        INT REFERENCES reagent_box(id),
    location      TEXT,
    owner         TEXT,
    label         TEXT,
    notes         TEXT,
    is_depleted   BOOLEAN NOT NULL DEFAULT FALSE,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_by    UUID REFERENCES users(id)
);

-- Molecular reagents (MR / MItems)
CREATE TABLE IF NOT EXISTS reagent_molecular (
    id            SERIAL PRIMARY KEY,
    mr_name       TEXT NOT NULL,
    mr_type       TEXT,
    box_id        INT REFERENCES reagent_box(id),
    location      TEXT,
    position      TEXT,
    owner         TEXT,
    label         TEXT,
    notes         TEXT,
    is_depleted   BOOLEAN NOT NULL DEFAULT FALSE,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_by    UUID REFERENCES users(id)
);

-- Reagent change audit log (append-only, separate from immutable experiment audit)
CREATE TABLE IF NOT EXISTS reagent_audit_log (
    id           BIGSERIAL PRIMARY KEY,
    table_name   TEXT NOT NULL,
    record_id    INT NOT NULL,
    action       TEXT NOT NULL CHECK (action IN ('INSERT','UPDATE','DELETE')),
    changed_by   UUID REFERENCES users(id),
    changed_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    old_data     JSONB,
    new_data     JSONB
);

-- Full-text search indexes on reagent name fields
CREATE INDEX IF NOT EXISTS idx_reagent_antibody_name ON reagent_antibody USING gin(to_tsvector('english', antibody_name));
CREATE INDEX IF NOT EXISTS idx_reagent_cell_line_name ON reagent_cell_line USING gin(to_tsvector('english', cell_line_name));
CREATE INDEX IF NOT EXISTS idx_reagent_virus_name ON reagent_virus USING gin(to_tsvector('english', virus_name));
CREATE INDEX IF NOT EXISTS idx_reagent_dna_name ON reagent_dna USING gin(to_tsvector('english', dna_name));
CREATE INDEX IF NOT EXISTS idx_reagent_oligo_name ON reagent_oligo USING gin(to_tsvector('english', oligo_name));
CREATE INDEX IF NOT EXISTS idx_reagent_chemical_name ON reagent_chemical USING gin(to_tsvector('english', chemical_name));
CREATE INDEX IF NOT EXISTS idx_reagent_molecular_name ON reagent_molecular USING gin(to_tsvector('english', mr_name));

-- Trigger function for updated_at
CREATE OR REPLACE FUNCTION reagent_set_updated_at()
RETURNS TRIGGER LANGUAGE plpgsql AS $$
BEGIN
  NEW.updated_at = NOW();
  RETURN NEW;
END;
$$;

-- Apply updated_at trigger to all reagent tables
DO $$
DECLARE tbl TEXT;
BEGIN
  FOREACH tbl IN ARRAY ARRAY[
    'reagent_storage','reagent_box','reagent_antibody','reagent_cell_line',
    'reagent_virus','reagent_dna','reagent_oligo','reagent_chemical','reagent_molecular'
  ] LOOP
    EXECUTE format(
      'CREATE TRIGGER trg_%s_updated_at BEFORE UPDATE ON %s
       FOR EACH ROW EXECUTE FUNCTION reagent_set_updated_at()', tbl, tbl);
  END LOOP;
END;
$$;

-- Audit trigger function
CREATE OR REPLACE FUNCTION reagent_audit_trigger()
RETURNS TRIGGER LANGUAGE plpgsql AS $$
BEGIN
  INSERT INTO reagent_audit_log(table_name, record_id, action, changed_by, old_data, new_data)
  VALUES (
    TG_TABLE_NAME,
    COALESCE(NEW.id, OLD.id),
    TG_OP,
    COALESCE(NEW.updated_by, OLD.updated_by),
    CASE WHEN TG_OP != 'INSERT' THEN row_to_json(OLD)::jsonb END,
    CASE WHEN TG_OP != 'DELETE' THEN row_to_json(NEW)::jsonb END
  );
  RETURN COALESCE(NEW, OLD);
END;
$$;

-- Apply audit triggers
DO $$
DECLARE tbl TEXT;
BEGIN
  FOREACH tbl IN ARRAY ARRAY[
    'reagent_storage','reagent_box','reagent_antibody','reagent_cell_line',
    'reagent_virus','reagent_dna','reagent_oligo','reagent_chemical','reagent_molecular'
  ] LOOP
    EXECUTE format(
      'CREATE TRIGGER trg_%s_audit AFTER INSERT OR UPDATE OR DELETE ON %s
       FOR EACH ROW EXECUTE FUNCTION reagent_audit_trigger()', tbl, tbl);
  END LOOP;
END;
$$;
