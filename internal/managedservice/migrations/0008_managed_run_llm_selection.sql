-- +goose Up
ALTER TABLE runs
  ADD COLUMN IF NOT EXISTS tester_llm_ids JSONB,
  ADD COLUMN IF NOT EXISTS editor_llm_id TEXT;

-- Historical backfill only. Runtime defaults live in Go code under internal/llmoptions,
-- and every new run inserts explicit tester_llm_ids/editor_llm_id values.
UPDATE runs
SET tester_llm_ids = COALESCE(
  (
    SELECT jsonb_agg(llm_id ORDER BY llm_id)
    FROM (
      SELECT DISTINCT NULLIF(llm_id, '') AS llm_id
      FROM run_sessions
      WHERE run_sessions.run_id = runs.id
        AND kind = 'tester'
        AND NULLIF(llm_id, '') IS NOT NULL
    ) tester_llms
  ),
  CASE
    WHEN EXISTS (
      SELECT 1 FROM run_sessions
      WHERE run_sessions.run_id = runs.id
        AND kind = 'tester'
    ) THEN '["medium-claude"]'::jsonb
    ELSE '["dumb-claude","medium-claude","smart-claude"]'::jsonb
  END
)
WHERE tester_llm_ids IS NULL;

UPDATE runs
SET editor_llm_id = COALESCE(
  (
    SELECT NULLIF(llm_id, '')
    FROM run_sessions
    WHERE run_sessions.run_id = runs.id
      AND kind = 'editor'
      AND NULLIF(llm_id, '') IS NOT NULL
    ORDER BY created_at DESC
    LIMIT 1
  ),
  'medium-claude'
)
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
