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
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/mupt-ai/dari-docs/internal/platformauth"
)

type user struct {
	ID          string
	Email       string
	TokenID     string
	TokenHash   string
	TokenKind   string
	TokenName   string
	TokenPrefix string
	TokenScopes []string
}

const (
	tokenKindInteractive    = "interactive"
	tokenKindAutomation     = "automation"
	tokenKindBrowserSession = "browser_session"

	scopeManagedRead         = "managed:read"
	scopeManagedCheck        = "managed:check"
	scopeManagedOptimize     = "managed:optimize"
	scopeManagedAgentsDeploy = "managed:agents:deploy"
	scopeManagedBilling      = "managed:billing"
	scopeManagedTokens       = "managed:tokens"
)

var (
	allManagedScopes = []string{
		scopeManagedRead,
		scopeManagedCheck,
		scopeManagedOptimize,
		scopeManagedAgentsDeploy,
		scopeManagedBilling,
		scopeManagedTokens,
	}
	defaultAutomationScopes = []string{
		scopeManagedRead,
		scopeManagedCheck,
		scopeManagedOptimize,
	}
)

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

func (s *Server) withUserAuth(next func(http.ResponseWriter, *http.Request, user)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		u, err := s.authenticateUser(r.Context(), r.Header.Get("Authorization"))
		if err != nil {
			var upstreamErr *upstreamHTTPError
			if errors.As(err, &upstreamErr) && upstreamErr.Status != http.StatusUnauthorized && upstreamErr.Status != http.StatusForbidden {
				writeLoggedError(w, http.StatusBadGateway, "could not verify Dari user session", err)
				return
			}
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
	return s.authenticateManagedToken(ctx, token)
}

func (s *Server) authenticateUser(ctx context.Context, header string) (user, error) {
	token, err := bearerToken(header)
	if err != nil {
		return user{}, err
	}
	u, err := s.authenticateManagedToken(ctx, token)
	if err == nil {
		return u, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) || !looksLikeJWT(token) {
		return user{}, err
	}
	return s.authenticateDariSession(ctx, token)
}

func (s *Server) authenticateManagedToken(ctx context.Context, token string) (user, error) {
	hash := sha256Hex(token)
	var u user
	var scopesJSON string
	err := s.db.QueryRow(ctx, `
SELECT users.id, users.email, api_tokens.id, api_tokens.token_hash,
       coalesce(api_tokens.kind, 'interactive'), coalesce(api_tokens.name, ''),
       coalesce(api_tokens.token_prefix, ''), coalesce(api_tokens.scopes, '[]'::jsonb)::text
FROM api_tokens
JOIN users ON users.id = api_tokens.user_id
WHERE api_tokens.token_hash=$1 AND (api_tokens.expires_at IS NULL OR api_tokens.expires_at > now()) AND api_tokens.revoked_at IS NULL
`, hash).Scan(&u.ID, &u.Email, &u.TokenID, &u.TokenHash, &u.TokenKind, &u.TokenName, &u.TokenPrefix, &scopesJSON)
	if err != nil {
		return user{}, err
	}
	u.TokenScopes = parseScopesJSON(scopesJSON)
	_, _ = s.db.Exec(ctx, `
UPDATE api_tokens SET last_used_at=now()
WHERE id=$1 AND (last_used_at IS NULL OR last_used_at < now() - interval '10 minutes')
`, u.TokenID)
	return u, nil
}

func (s *Server) authenticateDariSession(ctx context.Context, accessToken string) (user, error) {
	info, err := s.fetchDariUserInfo(ctx, accessToken)
	if err != nil {
		return user{}, err
	}
	u, needsGrant, found, err := s.lookupDariSessionUser(ctx, info)
	if err != nil {
		return user{}, err
	}
	if found && !needsGrant {
		return u, nil
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return user{}, err
	}
	defer tx.Rollback(ctx)
	userID := u.ID
	if !found {
		userID, err = upsertUserForDariIdentity(ctx, tx, info)
		if err != nil {
			return user{}, err
		}
	}
	if err := s.ensureFreeGrant(ctx, tx, userID); err != nil {
		return user{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return user{}, err
	}
	return user{
		ID:        userID,
		Email:     info.Email,
		TokenKind: tokenKindBrowserSession,
	}, nil
}

func (s *Server) lookupDariSessionUser(ctx context.Context, info dariUserInfo) (user, bool, bool, error) {
	var u user
	var grantedAt *time.Time
	err := s.db.QueryRow(ctx, `
SELECT id, free_credit_granted_at
FROM users
WHERE auth_subject=$1
`, info.AuthSubject).Scan(&u.ID, &grantedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return user{}, false, false, nil
	}
	if err != nil {
		return user{}, false, false, err
	}
	u.Email = info.Email
	u.TokenKind = tokenKindBrowserSession
	needsGrant := grantedAt == nil && s.cfg.FreeGrantCents > 0
	return u, needsGrant, true, nil
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

func looksLikeJWT(token string) bool {
	return strings.Count(token, ".") == 2
}

func parseScopesJSON(raw string) []string {
	var scopes []string
	if err := json.Unmarshal([]byte(raw), &scopes); err != nil {
		return nil
	}
	return normalizeScopes(scopes)
}

func normalizeScopes(scopes []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, scope := range scopes {
		scope = strings.TrimSpace(scope)
		if scope == "" || seen[scope] {
			continue
		}
		seen[scope] = true
		out = append(out, scope)
	}
	return out
}

func validScope(scope string) bool {
	for _, allowed := range allManagedScopes {
		if scope == allowed {
			return true
		}
	}
	return false
}

func (u user) hasScope(scope string) bool {
	switch effectiveTokenKind(u.TokenKind) {
	case tokenKindInteractive, tokenKindBrowserSession:
		return true
	}
	for _, s := range u.TokenScopes {
		if s == scope {
			return true
		}
	}
	return false
}

func requireScope(w http.ResponseWriter, u user, scope string) bool {
	if u.hasScope(scope) {
		return true
	}
	writeError(w, http.StatusForbidden, fmt.Sprintf("token missing required scope %s", scope))
	return false
}

func (s *Server) handleAuthConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	cfg, err := platformauth.FetchConfig(r.Context(), s.cfg.DariAPIBaseURL)
	if err != nil {
		writeLoggedError(w, http.StatusBadGateway, "could not load auth config", err)
		return
	}
	writeJSON(w, http.StatusOK, cfg)
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
INSERT INTO api_tokens (id, user_id, token_hash, expires_at, kind, scopes, created_by_user_id)
VALUES ($1, $2, $3, NULL, $4, $5, $2)
	`, "tok_"+randomToken(18), userID, sha256Hex(token), tokenKindInteractive, mustJSON(allManagedScopes))
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
UPDATE api_tokens SET revoked_at=coalesce(revoked_at, now()), revoked_by_user_id=coalesce(revoked_by_user_id, $2)
WHERE token_hash=$1 AND user_id=$2
`, u.TokenHash, u.ID)
	if err != nil {
		writeLoggedError(w, http.StatusInternalServerError, "could not log out", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"revoked": true})
}

func (s *Server) handleLogoutAll(w http.ResponseWriter, r *http.Request, u user) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if effectiveTokenKind(u.TokenKind) != tokenKindInteractive && !u.hasScope(scopeManagedTokens) {
		writeError(w, http.StatusForbidden, fmt.Sprintf("token missing required scope %s", scopeManagedTokens))
		return
	}
	kind := strings.TrimSpace(r.URL.Query().Get("kind"))
	if kind != "" && kind != tokenKindInteractive && kind != tokenKindAutomation {
		writeError(w, http.StatusBadRequest, "kind must be interactive or automation")
		return
	}
	query := `
UPDATE api_tokens SET revoked_at=coalesce(revoked_at, now()), revoked_by_user_id=coalesce(revoked_by_user_id, $2)
WHERE user_id=$1 AND revoked_at IS NULL`
	args := []any{u.ID, u.ID}
	if kind != "" {
		query += ` AND kind=$3`
		args = append(args, kind)
	}
	tag, err := s.db.Exec(r.Context(), query, args...)
	if err != nil {
		writeLoggedError(w, http.StatusInternalServerError, "could not log out", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"revoked": true, "tokens_revoked": tag.RowsAffected()})
}

type authTokenResponse struct {
	ID          string     `json:"id"`
	Name        string     `json:"name,omitempty"`
	Kind        string     `json:"kind"`
	TokenPrefix string     `json:"token_prefix,omitempty"`
	Scopes      []string   `json:"scopes"`
	Token       string     `json:"token,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	LastUsedAt  *time.Time `json:"last_used_at,omitempty"`
	ExpiresAt   *time.Time `json:"expires_at,omitempty"`
	RevokedAt   *time.Time `json:"revoked_at,omitempty"`
}

type authTokenListResponse struct {
	Tokens []authTokenResponse `json:"tokens"`
}

func (s *Server) handleAuthTokens(w http.ResponseWriter, r *http.Request, u user) {
	switch r.Method {
	case http.MethodGet:
		s.handleListAuthTokens(w, r, u)
	case http.MethodPost:
		s.handleCreateAuthToken(w, r, u)
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (s *Server) handleAuthTokenByID(w http.ResponseWriter, r *http.Request, u user) {
	rest := strings.Trim(strings.TrimPrefix(r.URL.Path, "/v1/auth/tokens/"), "/")
	tokenID, action, ok := strings.Cut(rest, "/")
	if tokenID == "" || !ok || action != "revoke" {
		writeError(w, http.StatusNotFound, "token not found")
		return
	}
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if !requireScope(w, u, scopeManagedTokens) {
		return
	}
	tag, err := s.db.Exec(r.Context(), `
UPDATE api_tokens SET revoked_at=coalesce(revoked_at, now()), revoked_by_user_id=coalesce(revoked_by_user_id, $3)
WHERE id=$1 AND user_id=$2 AND revoked_at IS NULL
`, tokenID, u.ID, u.ID)
	if err != nil {
		writeLoggedError(w, http.StatusInternalServerError, "could not revoke token", err)
		return
	}
	if tag.RowsAffected() == 0 {
		writeError(w, http.StatusNotFound, "token not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"revoked": true})
}

func (s *Server) handleCreateAuthToken(w http.ResponseWriter, r *http.Request, u user) {
	if !requireScope(w, u, scopeManagedTokens) {
		return
	}
	var req struct {
		Name      string     `json:"name"`
		Scopes    []string   `json:"scopes"`
		ExpiresAt *time.Time `json:"expires_at"`
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	if len(name) > 80 || strings.ContainsAny(name, "\r\n\t") {
		writeError(w, http.StatusBadRequest, "name must be 1-80 printable characters")
		return
	}
	scopes := normalizeScopes(req.Scopes)
	if len(scopes) == 0 {
		scopes = append([]string{}, defaultAutomationScopes...)
	}
	for _, scope := range scopes {
		if !validScope(scope) {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("unknown scope %s", scope))
			return
		}
		if !u.hasScope(scope) {
			writeError(w, http.StatusForbidden, fmt.Sprintf("token cannot grant scope %s", scope))
			return
		}
	}
	var activeNameExists bool
	if err := s.db.QueryRow(r.Context(), `
SELECT EXISTS (
  SELECT 1 FROM api_tokens
  WHERE user_id=$1 AND kind=$2 AND lower(name)=lower($3) AND revoked_at IS NULL
)
`, u.ID, tokenKindAutomation, name).Scan(&activeNameExists); err != nil {
		writeLoggedError(w, http.StatusInternalServerError, "could not create token", err)
		return
	}
	if activeNameExists {
		writeError(w, http.StatusConflict, "an active automation token with this name already exists")
		return
	}
	tokenID := "tok_" + randomToken(12)
	prefix := "mdt_v1_" + tokenID
	rawToken := prefix + "_" + randomToken(32)
	scopesJSON := mustJSON(scopes)
	var createdAt time.Time
	err := s.db.QueryRow(r.Context(), `
INSERT INTO api_tokens (id, user_id, name, kind, token_prefix, token_hash, scopes, expires_at, created_by_user_id)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $2)
RETURNING created_at
`, tokenID, u.ID, name, tokenKindAutomation, prefix, sha256Hex(rawToken), scopesJSON, req.ExpiresAt).Scan(&createdAt)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			writeError(w, http.StatusConflict, "an active automation token with this name already exists")
			return
		}
		writeLoggedError(w, http.StatusInternalServerError, "could not create token", err)
		return
	}
	writeJSON(w, http.StatusCreated, authTokenResponse{
		ID:          tokenID,
		Name:        name,
		Kind:        tokenKindAutomation,
		TokenPrefix: prefix,
		Scopes:      scopes,
		Token:       rawToken,
		CreatedAt:   createdAt,
		ExpiresAt:   req.ExpiresAt,
	})
}

func (s *Server) handleListAuthTokens(w http.ResponseWriter, r *http.Request, u user) {
	if !requireScope(w, u, scopeManagedTokens) {
		return
	}
	rows, err := s.db.Query(r.Context(), `
SELECT id, coalesce(name,''), coalesce(kind,'interactive'), coalesce(token_prefix,''),
       coalesce(scopes, '[]'::jsonb)::text, created_at, last_used_at, expires_at, revoked_at
FROM api_tokens
WHERE user_id=$1 AND revoked_at IS NULL
ORDER BY created_at DESC
`, u.ID)
	if err != nil {
		writeLoggedError(w, http.StatusInternalServerError, "could not list tokens", err)
		return
	}
	defer rows.Close()
	tokens := []authTokenResponse{}
	for rows.Next() {
		var token authTokenResponse
		var scopesJSON string
		if err := rows.Scan(&token.ID, &token.Name, &token.Kind, &token.TokenPrefix, &scopesJSON, &token.CreatedAt, &token.LastUsedAt, &token.ExpiresAt, &token.RevokedAt); err != nil {
			writeLoggedError(w, http.StatusInternalServerError, "could not list tokens", err)
			return
		}
		token.Scopes = parseScopesJSON(scopesJSON)
		if token.Kind == tokenKindInteractive {
			token.Scopes = append([]string{}, allManagedScopes...)
		} else if token.Scopes == nil {
			token.Scopes = []string{}
		}
		tokens = append(tokens, token)
	}
	if rows.Err() != nil {
		writeLoggedError(w, http.StatusInternalServerError, "could not list tokens", rows.Err())
		return
	}
	writeJSON(w, http.StatusOK, authTokenListResponse{Tokens: tokens})
}

func mustJSON(v any) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return b
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
  SET email=CASE
        WHEN EXISTS (
          SELECT 1 FROM users other
          WHERE other.email=$2 AND other.auth_subject IS DISTINCT FROM $1
        ) THEN users.email
        ELSE $2
      END,
      display_name=$3
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
	if !requireScope(w, u, scopeManagedRead) {
		return
	}
	bal, err := s.balanceCents(r.Context(), u.ID)
	if err != nil {
		writeLoggedError(w, http.StatusInternalServerError, "could not load account balance", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"email":         u.Email,
		"balance_cents": bal,
		"token": map[string]any{
			"id":           u.TokenID,
			"name":         u.TokenName,
			"kind":         effectiveTokenKind(u.TokenKind),
			"token_prefix": u.TokenPrefix,
			"scopes":       effectiveScopes(u),
		},
	})
}

func effectiveTokenKind(kind string) string {
	if kind == tokenKindBrowserSession {
		return tokenKindBrowserSession
	}
	if kind == tokenKindAutomation {
		return tokenKindAutomation
	}
	return tokenKindInteractive
}

func effectiveScopes(u user) []string {
	switch effectiveTokenKind(u.TokenKind) {
	case tokenKindInteractive, tokenKindBrowserSession:
		return append([]string{}, allManagedScopes...)
	}
	return append([]string{}, u.TokenScopes...)
}
