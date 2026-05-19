-- +goose Up
ALTER TABLE credit_ledger ADD COLUMN IF NOT EXISTS note TEXT;
ALTER TABLE credit_ledger ADD COLUMN IF NOT EXISTS created_by_user_id TEXT REFERENCES users(id);

-- +goose Down
ALTER TABLE credit_ledger DROP COLUMN IF EXISTS created_by_user_id;
ALTER TABLE credit_ledger DROP COLUMN IF EXISTS note;
