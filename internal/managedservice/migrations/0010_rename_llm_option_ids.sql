-- +goose Up
UPDATE run_sessions
SET llm_id = CASE llm_id
  WHEN 'dumb-claude' THEN 'claude-haiku-4-5'
  WHEN 'medium-claude' THEN 'claude-sonnet-4-7'
  WHEN 'smart-claude' THEN 'claude-opus-4-6'
  WHEN 'dumb-gpt' THEN 'gpt-5-mini'
  WHEN 'medium-gpt' THEN 'gpt-5.1'
  WHEN 'smart-gpt' THEN 'gpt-5.5'
  ELSE llm_id
END
WHERE llm_id IN ('dumb-claude', 'medium-claude', 'smart-claude', 'dumb-gpt', 'medium-gpt', 'smart-gpt');

UPDATE runs
SET editor_llm_id = CASE editor_llm_id
  WHEN 'dumb-claude' THEN 'claude-haiku-4-5'
  WHEN 'medium-claude' THEN 'claude-sonnet-4-7'
  WHEN 'smart-claude' THEN 'claude-opus-4-6'
  WHEN 'dumb-gpt' THEN 'gpt-5-mini'
  WHEN 'medium-gpt' THEN 'gpt-5.1'
  WHEN 'smart-gpt' THEN 'gpt-5.5'
  ELSE editor_llm_id
END
WHERE editor_llm_id IN ('dumb-claude', 'medium-claude', 'smart-claude', 'dumb-gpt', 'medium-gpt', 'smart-gpt');

UPDATE runs
SET tester_llm_ids = (
  SELECT jsonb_agg(
    CASE llm_id
      WHEN 'dumb-claude' THEN 'claude-haiku-4-5'
      WHEN 'medium-claude' THEN 'claude-sonnet-4-7'
      WHEN 'smart-claude' THEN 'claude-opus-4-6'
      WHEN 'dumb-gpt' THEN 'gpt-5-mini'
      WHEN 'medium-gpt' THEN 'gpt-5.1'
      WHEN 'smart-gpt' THEN 'gpt-5.5'
      ELSE llm_id
    END
    ORDER BY ord
  )
  FROM jsonb_array_elements_text(tester_llm_ids) WITH ORDINALITY AS ids(llm_id, ord)
)
WHERE tester_llm_ids ?| ARRAY['dumb-claude', 'medium-claude', 'smart-claude', 'dumb-gpt', 'medium-gpt', 'smart-gpt'];

-- +goose Down
UPDATE run_sessions
SET llm_id = CASE llm_id
  WHEN 'claude-haiku-4-5' THEN 'dumb-claude'
  WHEN 'claude-sonnet-4-7' THEN 'medium-claude'
  WHEN 'claude-opus-4-6' THEN 'smart-claude'
  WHEN 'gpt-5-mini' THEN 'dumb-gpt'
  WHEN 'gpt-5.1' THEN 'medium-gpt'
  WHEN 'gpt-5.5' THEN 'smart-gpt'
  ELSE llm_id
END
WHERE llm_id IN ('claude-haiku-4-5', 'claude-sonnet-4-7', 'claude-opus-4-6', 'gpt-5-mini', 'gpt-5.1', 'gpt-5.5');

UPDATE runs
SET editor_llm_id = CASE editor_llm_id
  WHEN 'claude-haiku-4-5' THEN 'dumb-claude'
  WHEN 'claude-sonnet-4-7' THEN 'medium-claude'
  WHEN 'claude-opus-4-6' THEN 'smart-claude'
  WHEN 'gpt-5-mini' THEN 'dumb-gpt'
  WHEN 'gpt-5.1' THEN 'medium-gpt'
  WHEN 'gpt-5.5' THEN 'smart-gpt'
  ELSE editor_llm_id
END
WHERE editor_llm_id IN ('claude-haiku-4-5', 'claude-sonnet-4-7', 'claude-opus-4-6', 'gpt-5-mini', 'gpt-5.1', 'gpt-5.5');

UPDATE runs
SET tester_llm_ids = (
  SELECT jsonb_agg(
    CASE llm_id
      WHEN 'claude-haiku-4-5' THEN 'dumb-claude'
      WHEN 'claude-sonnet-4-7' THEN 'medium-claude'
      WHEN 'claude-opus-4-6' THEN 'smart-claude'
      WHEN 'gpt-5-mini' THEN 'dumb-gpt'
      WHEN 'gpt-5.1' THEN 'medium-gpt'
      WHEN 'gpt-5.5' THEN 'smart-gpt'
      ELSE llm_id
    END
    ORDER BY ord
  )
  FROM jsonb_array_elements_text(tester_llm_ids) WITH ORDINALITY AS ids(llm_id, ord)
)
WHERE tester_llm_ids ?| ARRAY['claude-haiku-4-5', 'claude-sonnet-4-7', 'claude-opus-4-6', 'gpt-5-mini', 'gpt-5.1', 'gpt-5.5'];
