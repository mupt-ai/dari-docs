-- +goose Up
ALTER TABLE run_sessions
  ADD COLUMN IF NOT EXISTS llm_id TEXT;

-- +goose Down
ALTER TABLE run_sessions
  DROP COLUMN IF EXISTS llm_id;
