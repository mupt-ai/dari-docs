-- +goose Up
ALTER TABLE runs
  ADD COLUMN IF NOT EXISTS tester_llm_ids JSONB,
  ADD COLUMN IF NOT EXISTS editor_llm_id TEXT;

UPDATE runs
SET tester_llm_ids = '["dumb-claude","medium-claude","smart-claude"]'::jsonb
WHERE tester_llm_ids IS NULL;

UPDATE runs
SET editor_llm_id = 'medium-claude'
WHERE editor_llm_id IS NULL;

ALTER TABLE runs
  ALTER COLUMN tester_llm_ids SET NOT NULL,
  ALTER COLUMN tester_llm_ids DROP DEFAULT,
  ALTER COLUMN editor_llm_id SET NOT NULL,
  ALTER COLUMN editor_llm_id DROP DEFAULT;

-- +goose Down
ALTER TABLE runs
  DROP COLUMN IF EXISTS tester_llm_ids,
  DROP COLUMN IF EXISTS editor_llm_id;
