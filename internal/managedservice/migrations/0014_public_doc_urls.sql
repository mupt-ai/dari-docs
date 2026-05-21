-- +goose Up
ALTER TABLE runs
  ADD COLUMN IF NOT EXISTS public_doc_urls JSONB NOT NULL DEFAULT '[]'::jsonb;

-- +goose Down
ALTER TABLE runs
  DROP COLUMN IF EXISTS public_doc_urls;
