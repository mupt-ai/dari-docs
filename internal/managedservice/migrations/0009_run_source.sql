-- +goose Up
ALTER TABLE runs ADD COLUMN IF NOT EXISTS source TEXT NOT NULL DEFAULT 'cli';

ALTER TABLE runs DROP CONSTRAINT IF EXISTS runs_source_check;
ALTER TABLE runs ADD CONSTRAINT runs_source_check CHECK (source IN ('cli', 'web'));

-- +goose Down
ALTER TABLE runs DROP CONSTRAINT IF EXISTS runs_source_check;
ALTER TABLE runs DROP COLUMN IF EXISTS source;
