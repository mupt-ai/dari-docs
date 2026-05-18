-- +goose Up
ALTER TABLE runs
  ADD COLUMN IF NOT EXISTS tester_llm_ids JSONB NOT NULL DEFAULT '["medium-claude"]'::jsonb,
  ADD COLUMN IF NOT EXISTS editor_llm_id TEXT NOT NULL DEFAULT 'medium-claude';

-- +goose Down
ALTER TABLE runs
  DROP COLUMN IF EXISTS tester_llm_ids,
  DROP COLUMN IF EXISTS editor_llm_id;
