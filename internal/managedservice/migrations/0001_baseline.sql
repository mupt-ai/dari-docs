-- +goose Up
CREATE TABLE users (
  id TEXT PRIMARY KEY,
  auth_subject TEXT NOT NULL UNIQUE,
  email TEXT NOT NULL UNIQUE,
  display_name TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  free_credit_granted_at TIMESTAMPTZ
);

CREATE TABLE api_tokens (
  id TEXT PRIMARY KEY,
  user_id TEXT NOT NULL REFERENCES users(id),
  token_hash TEXT NOT NULL UNIQUE,
  expires_at TIMESTAMPTZ,
  revoked_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE agent_sets (
  id TEXT PRIMARY KEY,
  user_id TEXT NOT NULL REFERENCES users(id),
  tester_agent_id TEXT NOT NULL,
  editor_agent_id TEXT NOT NULL,
  tester_version_id TEXT,
  editor_version_id TEXT,
  tester_sha256 TEXT NOT NULL,
  editor_sha256 TEXT NOT NULL,
  applied_deploy_id TEXT,
  applied_deploy_sequence BIGINT NOT NULL DEFAULT 0,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE credit_ledger (
  id TEXT PRIMARY KEY,
  user_id TEXT NOT NULL REFERENCES users(id),
  amount_cents BIGINT NOT NULL,
  kind TEXT NOT NULL,
  source_id TEXT UNIQUE,
  run_id TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE stripe_checkout_sessions (
  id TEXT PRIMARY KEY,
  user_id TEXT NOT NULL REFERENCES users(id),
  amount_cents BIGINT NOT NULL,
  currency TEXT NOT NULL DEFAULT 'usd',
  status TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  completed_at TIMESTAMPTZ,
  credited_at TIMESTAMPTZ
);

CREATE TABLE runs (
  id TEXT PRIMARY KEY,
  user_id TEXT NOT NULL REFERENCES users(id),
  mode TEXT NOT NULL,
  status TEXT NOT NULL,
  tasks JSONB NOT NULL,
  agent_set_id TEXT REFERENCES agent_sets(id),
  tester_agent_id TEXT,
  tester_version_id TEXT,
  editor_agent_id TEXT,
  editor_version_id TEXT,
  bundle_file_id TEXT,
  bundle_sha256 TEXT NOT NULL,
  bundle_files INTEGER NOT NULL,
  live_verify BOOLEAN NOT NULL DEFAULT false,
  runtime_secret_names JSONB NOT NULL DEFAULT '[]'::jsonb,
  runtime_secrets_nonce BYTEA,
  runtime_secrets_ciphertext BYTEA,
  runtime_secrets_cleared_at TIMESTAMPTZ,
  editor_session_id TEXT,
  error TEXT,
  reserved_cents BIGINT NOT NULL DEFAULT 0,
  charged_cents BIGINT NOT NULL DEFAULT 0,
  cost_status TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  completed_at TIMESTAMPTZ
);

CREATE TABLE run_sessions (
  session_id TEXT PRIMARY KEY,
  run_id TEXT NOT NULL REFERENCES runs(id),
  kind TEXT NOT NULL,
  task_index INTEGER NOT NULL DEFAULT 0,
  status TEXT NOT NULL,
  version_id TEXT,
  cost_cents BIGINT,
  charge_cents BIGINT,
  last_polled_at TIMESTAMPTZ,
  last_poll_error_at TIMESTAMPTZ,
  last_poll_error TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  completed_at TIMESTAMPTZ
);

CREATE SEQUENCE agent_set_deploy_sequence;

CREATE TABLE agent_set_deploys (
  id TEXT PRIMARY KEY,
  user_id TEXT NOT NULL REFERENCES users(id),
  deploy_request_id TEXT NOT NULL,
  agent_set_id TEXT NOT NULL,
  target_tester_agent_id TEXT,
  target_editor_agent_id TEXT,
  tester_source_snapshot_id TEXT NOT NULL,
  editor_source_snapshot_id TEXT NOT NULL,
  tester_sha256 TEXT NOT NULL,
  editor_sha256 TEXT NOT NULL,
  tester_agent_id TEXT,
  tester_version_id TEXT,
  editor_agent_id TEXT,
  editor_version_id TEXT,
  sequence BIGINT NOT NULL DEFAULT nextval('agent_set_deploy_sequence'),
  status TEXT NOT NULL,
  step TEXT,
  applied BOOLEAN NOT NULL DEFAULT false,
  error TEXT,
  heartbeat_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  completed_at TIMESTAMPTZ,
  UNIQUE(user_id, deploy_request_id)
);

CREATE INDEX idx_runs_status_created ON runs(status, created_at);
CREATE INDEX idx_runs_user_status ON runs(user_id, status);
CREATE INDEX idx_run_sessions_status_created ON run_sessions(status, created_at);
CREATE INDEX idx_run_sessions_run_status ON run_sessions(run_id, status);
CREATE INDEX idx_agent_sets_user_updated ON agent_sets(user_id, updated_at DESC);
CREATE INDEX idx_stripe_checkout_sessions_user_created ON stripe_checkout_sessions(user_id, created_at DESC);
CREATE INDEX idx_agent_set_deploys_status_sequence ON agent_set_deploys(status, sequence);
CREATE INDEX idx_agent_set_deploys_agent_set_sequence ON agent_set_deploys(agent_set_id, sequence DESC);
CREATE INDEX idx_agent_set_deploys_user_updated ON agent_set_deploys(user_id, updated_at DESC);

-- +goose Down
DROP TABLE IF EXISTS agent_set_deploys;
DROP SEQUENCE IF EXISTS agent_set_deploy_sequence;
DROP TABLE IF EXISTS run_sessions;
DROP TABLE IF EXISTS runs;
DROP TABLE IF EXISTS stripe_checkout_sessions;
DROP TABLE IF EXISTS credit_ledger;
DROP TABLE IF EXISTS agent_sets;
DROP TABLE IF EXISTS api_tokens;
DROP TABLE IF EXISTS users;
