-- +goose Up
ALTER TABLE runs
  DROP COLUMN IF EXISTS agent_set_id,
  ALTER COLUMN tester_agent_id SET NOT NULL,
  ALTER COLUMN tester_version_id SET NOT NULL,
  ALTER COLUMN editor_agent_id SET NOT NULL,
  ALTER COLUMN editor_version_id SET NOT NULL;

DROP TABLE IF EXISTS agent_set_deploys;
DROP SEQUENCE IF EXISTS agent_set_deploy_sequence;
DROP TABLE IF EXISTS agent_sets;

-- +goose Down
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

ALTER TABLE runs
  ADD COLUMN IF NOT EXISTS agent_set_id TEXT REFERENCES agent_sets(id),
  ALTER COLUMN tester_agent_id DROP NOT NULL,
  ALTER COLUMN tester_version_id DROP NOT NULL,
  ALTER COLUMN editor_agent_id DROP NOT NULL,
  ALTER COLUMN editor_version_id DROP NOT NULL;

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

CREATE INDEX idx_agent_sets_user_updated ON agent_sets(user_id, updated_at DESC);
CREATE INDEX idx_agent_set_deploys_status_sequence ON agent_set_deploys(status, sequence);
CREATE INDEX idx_agent_set_deploys_agent_set_sequence ON agent_set_deploys(agent_set_id, sequence DESC);
CREATE INDEX idx_agent_set_deploys_user_updated ON agent_set_deploys(user_id, updated_at DESC);
