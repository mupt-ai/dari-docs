-- +goose Up
ALTER TABLE runs
  ADD COLUMN run_request_id TEXT;

CREATE UNIQUE INDEX idx_runs_user_request_id
  ON runs (user_id, run_request_id)
  WHERE run_request_id IS NOT NULL;

-- +goose Down
DROP INDEX IF EXISTS idx_runs_user_request_id;

ALTER TABLE runs
  DROP COLUMN IF EXISTS run_request_id;
