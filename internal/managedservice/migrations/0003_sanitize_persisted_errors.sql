-- +goose Up
UPDATE runs
SET error = CASE
  WHEN error = 'bundle upload did not complete' THEN 'bundle_upload_incomplete'
  ELSE 'run_failed'
END
WHERE error IS NOT NULL
  AND error <> ''
  AND error NOT IN (
    'run_failed',
    'bundle_stage_failed',
    'bundle_upload_failed',
    'bundle_upload_incomplete',
    'run_queue_failed',
    'runtime_secrets_load_failed',
    'session_create_failed',
    'session_message_failed',
    'session_failed',
    'session_poll_failed',
    'session_poll_stale',
    'session_stale'
  );

UPDATE run_sessions
SET last_poll_error = 'session_poll_failed'
WHERE last_poll_error IS NOT NULL
  AND last_poll_error <> ''
  AND last_poll_error NOT IN (
    'runtime_secrets_load_failed',
    'session_create_failed',
    'session_message_failed',
    'session_failed',
    'session_poll_failed',
    'session_poll_stale',
    'session_stale'
  );

UPDATE agent_set_deploys
SET error = 'agent_deploy_failed'
WHERE error IS NOT NULL
  AND error <> ''
  AND error NOT IN (
    'agent_deploy_failed',
    'agent_deploy_stale',
    'agent_deploy_publish_tester_failed',
    'agent_deploy_publish_editor_failed',
    'agent_deploy_update_failed',
    'agent_deploy_apply_failed'
  );

-- +goose Down
-- Irreversible sanitization of legacy free-form error strings.
SELECT 1;
