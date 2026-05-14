-- +goose Up
ALTER TABLE api_tokens
  ADD COLUMN name TEXT,
  ADD COLUMN kind TEXT NOT NULL DEFAULT 'interactive',
  ADD COLUMN token_prefix TEXT,
  ADD COLUMN scopes JSONB NOT NULL DEFAULT '[]'::jsonb,
  ADD COLUMN last_used_at TIMESTAMPTZ,
  ADD COLUMN created_by_user_id TEXT REFERENCES users(id),
  ADD COLUMN revoked_by_user_id TEXT REFERENCES users(id);

CREATE UNIQUE INDEX api_tokens_active_automation_name_key
  ON api_tokens (user_id, lower(name))
  WHERE kind = 'automation' AND name IS NOT NULL AND revoked_at IS NULL;

CREATE INDEX idx_api_tokens_user_kind_created
  ON api_tokens (user_id, kind, created_at DESC);

-- +goose Down
DROP INDEX IF EXISTS idx_api_tokens_user_kind_created;
DROP INDEX IF EXISTS api_tokens_active_automation_name_key;

ALTER TABLE api_tokens
  DROP COLUMN IF EXISTS revoked_by_user_id,
  DROP COLUMN IF EXISTS created_by_user_id,
  DROP COLUMN IF EXISTS last_used_at,
  DROP COLUMN IF EXISTS scopes,
  DROP COLUMN IF EXISTS token_prefix,
  DROP COLUMN IF EXISTS kind,
  DROP COLUMN IF EXISTS name;
