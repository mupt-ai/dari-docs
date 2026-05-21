package managedservice

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/mupt-ai/dari-docs/internal/dari"
	"github.com/mupt-ai/dari-docs/internal/redact"
)

func (s *Server) redactedTranscriptForSession(ctx context.Context, runID string, sessionID string) ([]byte, error) {
	secrets, err := s.runtimeSecrets(ctx, runID)
	if err != nil {
		return nil, fmt.Errorf("load runtime secrets: %w", err)
	}
	if len(secrets) == 0 {
		return nil, nil
	}
	raw, err := s.dari.GetTranscriptRaw(ctx, sessionID)
	if err != nil {
		return nil, fmt.Errorf("load session transcript for redaction: %w", err)
	}
	redacted := redact.NewSecrets(secrets).Bytes(raw)
	if !json.Valid(redacted) {
		return nil, fmt.Errorf("redacted session transcript is not valid JSON")
	}
	return redacted, nil
}

func (s *Server) redactedTranscriptRaw(ctx context.Context, sessionID string) ([]byte, bool, error) {
	var raw []byte
	err := s.db.QueryRow(ctx, `
SELECT redacted_transcript
FROM run_sessions
WHERE session_id=$1 AND redacted_transcript IS NOT NULL
`, sessionID).Scan(&raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return raw, true, nil
}

func (s *Server) sessionAssistantText(ctx context.Context, sessionID string, redactor redact.Redactor) (string, error) {
	if raw, ok, err := s.redactedTranscriptRaw(ctx, sessionID); err != nil {
		return "", err
	} else if ok {
		var tr dari.Transcript
		if err := json.Unmarshal(raw, &tr); err != nil {
			return "", err
		}
		return dari.FinalAssistantText(tr), nil
	}
	tr, err := s.dari.GetTranscript(ctx, sessionID)
	if err != nil {
		return "", err
	}
	return redactor.String(dari.FinalAssistantText(tr)), nil
}
