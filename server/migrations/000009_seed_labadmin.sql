-- 000009: Seed default LabAdmin account + expand user roles
-- The LabAdmin is seeded at app startup via Go code so that the password
-- hash is generated fresh each time.  This migration adds an
-- 'is_default_admin' column and expands the role check constraint.

-- Expand role constraint to include author and viewer
ALTER TABLE users DROP CONSTRAINT IF EXISTS users_role_check;
ALTER TABLE users ADD CONSTRAINT users_role_check CHECK (role IN ('owner', 'admin', 'author', 'viewer'));

-- Add default-admin flag
ALTER TABLE users ADD COLUMN IF NOT EXISTS is_default_admin BOOLEAN NOT NULL DEFAULT FALSE;
