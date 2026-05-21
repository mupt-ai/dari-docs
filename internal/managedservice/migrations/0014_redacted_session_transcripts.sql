ALTER TABLE run_sessions
ADD COLUMN IF NOT EXISTS redacted_transcript JSONB;
