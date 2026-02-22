-- Add lot number + expiry date tracking for reagent inventory entities

ALTER TABLE reagent_antibody
  ADD COLUMN IF NOT EXISTS lot_number TEXT,
  ADD COLUMN IF NOT EXISTS expiry_date DATE;

ALTER TABLE reagent_cell_line
  ADD COLUMN IF NOT EXISTS lot_number TEXT,
  ADD COLUMN IF NOT EXISTS expiry_date DATE;

ALTER TABLE reagent_virus
  ADD COLUMN IF NOT EXISTS lot_number TEXT,
  ADD COLUMN IF NOT EXISTS expiry_date DATE;

ALTER TABLE reagent_dna
  ADD COLUMN IF NOT EXISTS lot_number TEXT,
  ADD COLUMN IF NOT EXISTS expiry_date DATE;

ALTER TABLE reagent_oligo
  ADD COLUMN IF NOT EXISTS lot_number TEXT,
  ADD COLUMN IF NOT EXISTS expiry_date DATE;

ALTER TABLE reagent_chemical
  ADD COLUMN IF NOT EXISTS lot_number TEXT,
  ADD COLUMN IF NOT EXISTS expiry_date DATE;

ALTER TABLE reagent_molecular
  ADD COLUMN IF NOT EXISTS lot_number TEXT,
  ADD COLUMN IF NOT EXISTS expiry_date DATE;

CREATE INDEX IF NOT EXISTS idx_reagent_antibody_expiry_date ON reagent_antibody(expiry_date);
CREATE INDEX IF NOT EXISTS idx_reagent_cell_line_expiry_date ON reagent_cell_line(expiry_date);
CREATE INDEX IF NOT EXISTS idx_reagent_virus_expiry_date ON reagent_virus(expiry_date);
CREATE INDEX IF NOT EXISTS idx_reagent_dna_expiry_date ON reagent_dna(expiry_date);
CREATE INDEX IF NOT EXISTS idx_reagent_oligo_expiry_date ON reagent_oligo(expiry_date);
CREATE INDEX IF NOT EXISTS idx_reagent_chemical_expiry_date ON reagent_chemical(expiry_date);
CREATE INDEX IF NOT EXISTS idx_reagent_molecular_expiry_date ON reagent_molecular(expiry_date);
