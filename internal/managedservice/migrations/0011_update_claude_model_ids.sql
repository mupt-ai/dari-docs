-- +goose Up
UPDATE run_sessions
SET llm_id = CASE llm_id
  WHEN 'claude-sonnet-4-7' THEN 'claude-sonnet-4-6'
  WHEN 'claude-opus-4-6' THEN 'claude-opus-4-7'
  ELSE llm_id
END
WHERE llm_id IN ('claude-sonnet-4-7', 'claude-opus-4-6');

UPDATE runs
SET editor_llm_id = CASE editor_llm_id
  WHEN 'claude-sonnet-4-7' THEN 'claude-sonnet-4-6'
  WHEN 'claude-opus-4-6' THEN 'claude-opus-4-7'
  ELSE editor_llm_id
END
WHERE editor_llm_id IN ('claude-sonnet-4-7', 'claude-opus-4-6');

UPDATE runs
SET tester_llm_ids = (
  SELECT jsonb_agg(
    CASE llm_id
      WHEN 'claude-sonnet-4-7' THEN 'claude-sonnet-4-6'
      WHEN 'claude-opus-4-6' THEN 'claude-opus-4-7'
      ELSE llm_id
    END
    ORDER BY ord
  )
  FROM jsonb_array_elements_text(tester_llm_ids) WITH ORDINALITY AS ids(llm_id, ord)
)
WHERE tester_llm_ids ?| ARRAY['claude-sonnet-4-7', 'claude-opus-4-6'];

-- +goose Down
UPDATE run_sessions
SET llm_id = CASE llm_id
  WHEN 'claude-sonnet-4-6' THEN 'claude-sonnet-4-7'
  WHEN 'claude-opus-4-7' THEN 'claude-opus-4-6'
  ELSE llm_id
END
WHERE llm_id IN ('claude-sonnet-4-6', 'claude-opus-4-7');

UPDATE runs
SET editor_llm_id = CASE editor_llm_id
  WHEN 'claude-sonnet-4-6' THEN 'claude-sonnet-4-7'
  WHEN 'claude-opus-4-7' THEN 'claude-opus-4-6'
  ELSE editor_llm_id
END
WHERE editor_llm_id IN ('claude-sonnet-4-6', 'claude-opus-4-7');

UPDATE runs
SET tester_llm_ids = (
  SELECT jsonb_agg(
    CASE llm_id
      WHEN 'claude-sonnet-4-6' THEN 'claude-sonnet-4-7'
      WHEN 'claude-opus-4-7' THEN 'claude-opus-4-6'
      ELSE llm_id
    END
    ORDER BY ord
  )
  FROM jsonb_array_elements_text(tester_llm_ids) WITH ORDINALITY AS ids(llm_id, ord)
)
WHERE tester_llm_ids ?| ARRAY['claude-sonnet-4-6', 'claude-opus-4-7'];
