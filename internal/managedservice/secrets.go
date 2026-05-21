package managedservice

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/mupt-ai/dari-docs/internal/runtimeenv"
)

func decodeRuntimeSecretsKey(raw string) ([]byte, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, errors.New("DARI_DOCS_SECRET_ENCRYPTION_KEY is required")
	}
	key, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		return nil, errors.New("DARI_DOCS_SECRET_ENCRYPTION_KEY must be base64-encoded")
	}
	if len(key) != 32 {
		return nil, errors.New("DARI_DOCS_SECRET_ENCRYPTION_KEY must decode to 32 bytes")
	}
	return key, nil
}

func (s *Server) encryptRuntimeSecrets(plaintext []byte) ([]byte, []byte, error) {
	block, err := aes.NewCipher(s.cfg.RuntimeSecretsKey)
	if err != nil {
		return nil, nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, nil, err
	}
	return nonce, gcm.Seal(nil, nonce, plaintext, nil), nil
}

func (s *Server) runtimeSecrets(ctx context.Context, runID string) (map[string]string, error) {
	var nonce, ciphertext []byte
	err := s.db.QueryRow(ctx, `
SELECT runtime_secrets_nonce, runtime_secrets_ciphertext FROM runs WHERE id=$1
`, runID).Scan(&nonce, &ciphertext)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if len(nonce) == 0 || len(ciphertext) == 0 {
		return nil, nil
	}
	block, err := aes.NewCipher(s.cfg.RuntimeSecretsKey)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, err
	}
	secrets, _, err := runtimeenv.ParseJSON(string(plaintext))
	if err != nil {
		return nil, err
	}
	return secrets, nil
}

func (s *Server) clearRuntimeSecrets(ctx context.Context, runID string) {
	_, _ = s.db.Exec(ctx, `
UPDATE runs
SET runtime_secrets_nonce=NULL,
    runtime_secrets_ciphertext=NULL,
    runtime_secrets_cleared_at=coalesce(runtime_secrets_cleared_at, now())
WHERE id=$1 AND runtime_secrets_ciphertext IS NOT NULL
`, runID)
}

func secretNameMap(names []string) map[string]string {
	if len(names) == 0 {
		return nil
	}
	out := make(map[string]string, len(names))
	for _, name := range names {
		out[name] = ""
	}
	return out
}

func shouldAttachRuntimeSecrets(run queuedRun, next nextSession) bool {
	return run.LiveVerify && (next.Kind == "tester" || next.Kind == "editor")
}

func isFinalSecretBearingSession(run queuedRun, next nextSession) bool {
	if run.Mode == "optimize" {
		return next.Kind == "editor"
	}
	return next.Kind == "tester" && next.TaskIndex == len(run.Tasks)
}
