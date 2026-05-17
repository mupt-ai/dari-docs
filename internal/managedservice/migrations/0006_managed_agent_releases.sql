-- +goose Up
CREATE TABLE managed_agent_releases (
  id TEXT PRIMARY KEY,
  tester_agent_id TEXT NOT NULL,
  tester_version_id TEXT NOT NULL,
  editor_agent_id TEXT NOT NULL,
  editor_version_id TEXT NOT NULL,
  active BOOLEAN NOT NULL DEFAULT true,
  source TEXT,
  git_sha TEXT,
  github_run_id TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX idx_managed_agent_releases_active
  ON managed_agent_releases (active)
  WHERE active;

-- +goose Down
DROP TABLE IF EXISTS managed_agent_releases;
