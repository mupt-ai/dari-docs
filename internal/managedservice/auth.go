package managedservice

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

type user struct {
	ID        string
	Email     string
	TokenHash string
}

func (s *Server) withAuth(next func(http.ResponseWriter, *http.Request, user)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		u, err := s.authenticate(r.Context(), r.Header.Get("Authorization"))
		if err != nil {
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		next(w, r, u)
	}
}

func (s *Server) authenticate(ctx context.Context, header string) (user, error) {
	token, err := bearerToken(header)
	if err != nil {
		return user{}, err
	}
	hash := sha256Hex(token)
	var u user
	err = s.db.QueryRow(ctx, `
SELECT users.id, users.email, api_tokens.token_hash
FROM api_tokens
JOIN users ON users.id = api_tokens.user_id
WHERE api_tokens.token_hash=$1 AND (api_tokens.expires_at IS NULL OR api_tokens.expires_at > now()) AND api_tokens.revoked_at IS NULL
`, hash).Scan(&u.ID, &u.Email, &u.TokenHash)
	return u, err
}

func bearerToken(header string) (string, error) {
	const prefix = "Bearer "
	if !strings.HasPrefix(header, prefix) {
		return "", errors.New("missing bearer token")
	}
	token := strings.TrimSpace(strings.TrimPrefix(header, prefix))
	if token == "" {
		return "", errors.New("missing bearer token")
	}
	return token, nil
}

func (s *Server) handleDariAuthExchange(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	accessToken, err := bearerToken(r.Header.Get("Authorization"))
	if err != nil {
		writeError(w, http.StatusUnauthorized, "missing Dari user session")
		return
	}
	info, err := s.fetchDariUserInfo(r.Context(), accessToken)
	if err != nil {
		var httpErr *upstreamHTTPError
		if errors.As(err, &httpErr) && (httpErr.Status == http.StatusUnauthorized || httpErr.Status == http.StatusForbidden) {
			writeError(w, http.StatusUnauthorized, "invalid Dari user session")
			return
		}
		writeLoggedError(w, http.StatusBadGateway, "could not verify Dari user session", err)
		return
	}
	tx, err := s.db.Begin(r.Context())
	if err != nil {
		writeLoggedError(w, http.StatusInternalServerError, "could not complete login", err)
		return
	}
	defer tx.Rollback(r.Context())
	userID, err := upsertUserForDariIdentity(r.Context(), tx, info)
	if err != nil {
		writeLoggedError(w, http.StatusInternalServerError, "could not complete login", err)
		return
	}
	if err := s.ensureFreeGrant(r.Context(), tx, userID); err != nil {
		writeLoggedError(w, http.StatusInternalServerError, "could not complete login", err)
		return
	}
	token := randomToken(32)
	_, err = tx.Exec(r.Context(), `
INSERT INTO api_tokens (id, user_id, token_hash, expires_at)
VALUES ($1, $2, $3, NULL)
	`, "tok_"+randomToken(18), userID, sha256Hex(token))
	if err != nil {
		writeLoggedError(w, http.StatusInternalServerError, "could not complete login", err)
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		writeLoggedError(w, http.StatusInternalServerError, "could not complete login", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"token": token, "email": info.Email, "display_name": info.DisplayName})
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request, u user) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	_, err := s.db.Exec(r.Context(), `
UPDATE api_tokens SET revoked_at=coalesce(revoked_at, now())
WHERE token_hash=$1 AND user_id=$2
`, u.TokenHash, u.ID)
	if err != nil {
		writeLoggedError(w, http.StatusInternalServerError, "could not log out", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"revoked": true})
}

type dariUserInfo struct {
	AuthSubject string `json:"auth_subject"`
	Email       string `json:"email"`
	DisplayName string `json:"display_name"`
}

type upstreamHTTPError struct {
	Status int
	Body   string
}

func (e *upstreamHTTPError) Error() string {
	return fmt.Sprintf("Dari userinfo http %d: %s", e.Status, e.Body)
}

func (s *Server) fetchDariUserInfo(ctx context.Context, accessToken string) (dariUserInfo, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(s.cfg.DariAPIBaseURL, "/")+"/v1/auth/userinfo", nil)
	if err != nil {
		return dariUserInfo{}, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/json")
	resp, err := s.outboundHTTPClient().Do(req)
	if err != nil {
		return dariUserInfo{}, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return dariUserInfo{}, &upstreamHTTPError{Status: resp.StatusCode, Body: strings.TrimSpace(string(body))}
	}
	var info dariUserInfo
	if err := json.Unmarshal(body, &info); err != nil {
		return dariUserInfo{}, err
	}
	info.AuthSubject = strings.TrimSpace(info.AuthSubject)
	info.Email = strings.TrimSpace(info.Email)
	info.DisplayName = strings.TrimSpace(info.DisplayName)
	if info.AuthSubject == "" || info.Email == "" {
		return dariUserInfo{}, errors.New("Dari userinfo response missing auth_subject or email")
	}
	return info, nil
}

func upsertUserForDariIdentity(ctx context.Context, tx pgx.Tx, info dariUserInfo) (string, error) {
	userID := "usr_" + randomToken(18)
	err := tx.QueryRow(ctx, `
WITH by_subject AS (
  UPDATE users
  SET email=$2, display_name=$3
  WHERE auth_subject=$1
  RETURNING id
), by_email AS (
  UPDATE users
  SET auth_subject=$1, display_name=$3
  WHERE email=$2 AND auth_subject IS NULL AND NOT EXISTS (SELECT 1 FROM by_subject)
  RETURNING id
), inserted AS (
  INSERT INTO users (id, auth_subject, email, display_name)
  SELECT $4, $1, $2, $3
  WHERE NOT EXISTS (SELECT 1 FROM by_subject) AND NOT EXISTS (SELECT 1 FROM by_email)
  ON CONFLICT (email) DO UPDATE
  SET auth_subject=EXCLUDED.auth_subject, display_name=EXCLUDED.display_name
  WHERE users.auth_subject IS NULL OR users.auth_subject=EXCLUDED.auth_subject
  RETURNING id
)
SELECT id FROM by_subject
UNION ALL SELECT id FROM by_email
UNION ALL SELECT id FROM inserted
LIMIT 1
`, info.AuthSubject, info.Email, nullableString(info.DisplayName), userID).Scan(&userID)
	return userID, err
}

func nullableString(v string) any {
	if strings.TrimSpace(v) == "" {
		return nil
	}
	return v
}

func (s *Server) ensureFreeGrant(ctx context.Context, tx pgx.Tx, userID string) error {
	var grantedAt *time.Time
	if err := tx.QueryRow(ctx, `SELECT free_credit_granted_at FROM users WHERE id=$1 FOR UPDATE`, userID).Scan(&grantedAt); err != nil {
		return err
	}
	if grantedAt != nil || s.cfg.FreeGrantCents <= 0 {
		return nil
	}
	if _, err := tx.Exec(ctx, `
INSERT INTO credit_ledger (id, user_id, amount_cents, kind, source_id)
VALUES ($1, $2, $3, 'free_grant', $4)
`, "led_"+randomToken(18), userID, s.cfg.FreeGrantCents, "free_grant:"+userID); err != nil {
		return err
	}
	_, err := tx.Exec(ctx, `UPDATE users SET free_credit_granted_at=now() WHERE id=$1`, userID)
	return err
}

func (s *Server) handleMe(w http.ResponseWriter, r *http.Request, u user) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	bal, err := s.balanceCents(r.Context(), u.ID)
	if err != nil {
		writeLoggedError(w, http.StatusInternalServerError, "could not load account balance", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"email": u.Email, "balance_cents": bal})
}
