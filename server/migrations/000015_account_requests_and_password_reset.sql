-- 000015_account_requests_and_password_reset.sql
-- Adds:
-- 1) must_change_password flag for first-login password rotation
-- 2) account_requests table for unauthenticated account and recovery requests

ALTER TABLE users
  ADD COLUMN IF NOT EXISTS must_change_password BOOLEAN NOT NULL DEFAULT FALSE;

CREATE TABLE IF NOT EXISTS account_requests (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  request_type TEXT NOT NULL CHECK (request_type IN ('account_create', 'password_recovery')),
  username TEXT NOT NULL,
  email TEXT NOT NULL,
  note TEXT,
  status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'fulfilled', 'dismissed')),
  fulfilled_by_user_id UUID REFERENCES users(id),
  fulfilled_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_account_requests_status_created
  ON account_requests(status, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_account_requests_email_status
  ON account_requests(email, status);