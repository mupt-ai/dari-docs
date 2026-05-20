-- +goose Up
ALTER TABLE users
  ADD COLUMN IF NOT EXISTS max_tasks_per_run_override INTEGER,
  ADD COLUMN IF NOT EXISTS max_active_runs_per_user_override INTEGER;

ALTER TABLE users
  DROP CONSTRAINT IF EXISTS users_max_tasks_per_run_override_nonnegative,
  ADD CONSTRAINT users_max_tasks_per_run_override_nonnegative
    CHECK (max_tasks_per_run_override IS NULL OR max_tasks_per_run_override >= 0);

ALTER TABLE users
  DROP CONSTRAINT IF EXISTS users_max_active_runs_per_user_override_nonnegative,
  ADD CONSTRAINT users_max_active_runs_per_user_override_nonnegative
    CHECK (max_active_runs_per_user_override IS NULL OR max_active_runs_per_user_override >= 0);

-- +goose Down
ALTER TABLE users
  DROP CONSTRAINT IF EXISTS users_max_active_runs_per_user_override_nonnegative,
  DROP CONSTRAINT IF EXISTS users_max_tasks_per_run_override_nonnegative,
  DROP COLUMN IF EXISTS max_active_runs_per_user_override,
  DROP COLUMN IF EXISTS max_tasks_per_run_override;
