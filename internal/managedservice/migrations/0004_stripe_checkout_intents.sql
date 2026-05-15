-- +goose Up
ALTER TABLE stripe_checkout_sessions
  ADD COLUMN IF NOT EXISTS checkout_intent_id TEXT,
  ADD COLUMN IF NOT EXISTS stripe_session_id TEXT;

UPDATE stripe_checkout_sessions
SET checkout_intent_id = id,
    stripe_session_id = id
WHERE checkout_intent_id IS NULL;

ALTER TABLE stripe_checkout_sessions
  ALTER COLUMN checkout_intent_id SET NOT NULL;

CREATE UNIQUE INDEX IF NOT EXISTS idx_stripe_checkout_sessions_intent
  ON stripe_checkout_sessions (checkout_intent_id);

CREATE UNIQUE INDEX IF NOT EXISTS idx_stripe_checkout_sessions_stripe_session
  ON stripe_checkout_sessions (stripe_session_id)
  WHERE stripe_session_id IS NOT NULL;

-- +goose Down
DROP INDEX IF EXISTS idx_stripe_checkout_sessions_stripe_session;
DROP INDEX IF EXISTS idx_stripe_checkout_sessions_intent;

ALTER TABLE stripe_checkout_sessions
  DROP COLUMN IF EXISTS stripe_session_id,
  DROP COLUMN IF EXISTS checkout_intent_id;
