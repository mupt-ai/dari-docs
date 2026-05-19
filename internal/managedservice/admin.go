package managedservice

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

type adminUserSearchResponse struct {
	Users []adminUserSummary `json:"users"`
}

type adminUserDetailResponse struct {
	User adminUserSummary  `json:"user"`
	Runs []adminRunSummary `json:"runs"`
}

type adminUserSummary struct {
	ID                 string     `json:"id"`
	Email              string     `json:"email"`
	DisplayName        *string    `json:"display_name"`
	CreatedAt          time.Time  `json:"created_at"`
	LastActiveAt       *time.Time `json:"last_active_at"`
	BalanceCents       int64      `json:"balance_cents"`
	CreditGrantedCents int64      `json:"credit_granted_cents"`
	CreditSpentCents   int64      `json:"credit_spent_cents"`
	RunCount           int64      `json:"run_count"`
	ActiveRunCount     int64      `json:"active_run_count"`
}

type adminRunSummary struct {
	ID            string     `json:"id"`
	Mode          string     `json:"mode"`
	Source        string     `json:"source"`
	Status        string     `json:"status"`
	TesterAgentID *string    `json:"tester_agent_id"`
	EditorAgentID *string    `json:"editor_agent_id"`
	TaskCount     int64      `json:"task_count"`
	ReservedCents int64      `json:"reserved_cents"`
	ChargedCents  int64      `json:"charged_cents"`
	CreatedAt     time.Time  `json:"created_at"`
	CompletedAt   *time.Time `json:"completed_at"`
}

type adminCreditGrantResponse struct {
	ID              string    `json:"id"`
	UserID          string    `json:"user_id"`
	AmountCents     int64     `json:"amount_cents"`
	AmountUSD       string    `json:"amount_usd"`
	Note            *string   `json:"note"`
	CreatedAt       time.Time `json:"created_at"`
	CreatedByUserID string    `json:"created_by_user_id"`
}

const adminUserSummarySelect = `
SELECT
	u.id,
	u.email,
	u.display_name,
	u.created_at,
	runs.last_active_at,
	coalesce(credits.balance_cents, 0),
	coalesce(credits.granted_cents, 0),
	greatest(coalesce(credits.granted_cents, 0) - coalesce(credits.balance_cents, 0), 0),
	coalesce(runs.run_count, 0),
	coalesce(runs.active_run_count, 0)
FROM users u
LEFT JOIN (
	SELECT
		user_id,
		coalesce(sum(amount_cents), 0) AS balance_cents,
		coalesce(sum(amount_cents) FILTER (WHERE amount_cents > 0 AND kind <> 'run_reservation_release'), 0) AS granted_cents
	FROM credit_ledger
	GROUP BY user_id
) credits ON credits.user_id = u.id
LEFT JOIN (
	SELECT
		user_id,
		count(*) AS run_count,
		count(*) FILTER (WHERE status IN ('queued', 'uploading', 'starting', 'running')) AS active_run_count,
		max(created_at) AS last_active_at
	FROM runs
	GROUP BY user_id
) runs ON runs.user_id = u.id
`

var adminEmails = map[string]struct{}{
	"avyay@mupt.ai": {},
	"ben@mupt.ai":   {},
}

func (s *Server) isAdminEmail(email string) bool {
	_, ok := adminEmails[strings.ToLower(strings.TrimSpace(email))]
	return ok
}

func (s *Server) isAdminBrowserSession(u user) bool {
	return s.isAdminEmail(u.Email) && effectiveTokenKind(u.TokenKind) == tokenKindBrowserSession
}

func (s *Server) requireAdmin(w http.ResponseWriter, u user) bool {
	if s.isAdminEmail(u.Email) {
		if effectiveTokenKind(u.TokenKind) != tokenKindBrowserSession {
			writeError(w, http.StatusForbidden, "platform admin browser session is required")
			return false
		}
		return true
	}
	writeError(w, http.StatusForbidden, "platform admin access is required")
	return false
}

func (s *Server) handleAdminUserSearch(w http.ResponseWriter, r *http.Request, u user) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if !s.requireAdmin(w, u) {
		return
	}
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	if len(query) < 2 {
		writeJSON(w, http.StatusOK, adminUserSearchResponse{Users: []adminUserSummary{}})
		return
	}
	limit := adminSearchLimit(r.URL.Query().Get("limit"))
	pattern := "%" + escapeAdminSearchPattern(query) + "%"
	rows, err := s.db.Query(r.Context(), adminUserSummarySelect+`
WHERE u.id ILIKE $1 OR u.email ILIKE $1 OR coalesce(u.display_name, '') ILIKE $1 OR u.auth_subject ILIKE $1
ORDER BY u.created_at DESC
LIMIT $2
`, pattern, limit)
	if err != nil {
		writeLoggedError(w, http.StatusInternalServerError, "could not search users", err)
		return
	}
	defer rows.Close()
	users := []adminUserSummary{}
	for rows.Next() {
		user, err := scanAdminUserSummary(rows)
		if err != nil {
			writeLoggedError(w, http.StatusInternalServerError, "could not search users", err)
			return
		}
		users = append(users, user)
	}
	if rows.Err() != nil {
		writeLoggedError(w, http.StatusInternalServerError, "could not search users", rows.Err())
		return
	}
	writeJSON(w, http.StatusOK, adminUserSearchResponse{Users: users})
}

func (s *Server) handleAdminUserByID(w http.ResponseWriter, r *http.Request, u user) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if !s.requireAdmin(w, u) {
		return
	}
	userID := strings.Trim(strings.TrimPrefix(r.URL.Path, "/v1/admin/users/"), "/")
	if userID == "" || strings.Contains(userID, "/") {
		writeError(w, http.StatusNotFound, "user not found")
		return
	}
	summary, err := s.getAdminUserSummary(r.Context(), userID)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "user not found")
		return
	}
	if err != nil {
		writeLoggedError(w, http.StatusInternalServerError, "could not load user", err)
		return
	}
	runs, err := s.listAdminUserRuns(r.Context(), userID)
	if err != nil {
		writeLoggedError(w, http.StatusInternalServerError, "could not load user runs", err)
		return
	}
	writeJSON(w, http.StatusOK, adminUserDetailResponse{User: summary, Runs: runs})
}

func (s *Server) handleAdminCredits(w http.ResponseWriter, r *http.Request, u user) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if !s.requireAdmin(w, u) {
		return
	}
	var req struct {
		UserID    string  `json:"user_id"`
		AmountUSD string  `json:"amount_usd"`
		Note      *string `json:"note"`
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	userID := strings.TrimSpace(req.UserID)
	if userID == "" {
		writeError(w, http.StatusBadRequest, "user_id is required")
		return
	}
	amountCents, err := parseAdminUSDToCents(req.AmountUSD)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	note, err := normalizeAdminGrantNote(req.Note)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	ledgerID := "led_" + randomToken(18)
	sourceID := "admin_grant:" + ledgerID
	var createdAt time.Time
	err = s.db.QueryRow(r.Context(), `
INSERT INTO credit_ledger (id, user_id, amount_cents, kind, source_id, note, created_by_user_id)
SELECT $1, $2, $3, 'admin_grant', $4, $5, $6
WHERE EXISTS (SELECT 1 FROM users WHERE id=$2)
RETURNING created_at
`, ledgerID, userID, amountCents, sourceID, note, u.ID).Scan(&createdAt)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "user not found")
		return
	}
	if err != nil {
		writeLoggedError(w, http.StatusInternalServerError, "could not grant credits", err)
		return
	}
	writeJSON(w, http.StatusCreated, adminCreditGrantResponse{
		ID:              ledgerID,
		UserID:          userID,
		AmountCents:     amountCents,
		AmountUSD:       usdStringFromCents(amountCents),
		Note:            note,
		CreatedAt:       createdAt,
		CreatedByUserID: u.ID,
	})
}

func (s *Server) getAdminUserSummary(ctx context.Context, userID string) (adminUserSummary, error) {
	return scanAdminUserSummary(s.db.QueryRow(ctx, adminUserSummarySelect+`WHERE u.id=$1`, userID))
}

func (s *Server) listAdminUserRuns(ctx context.Context, userID string) ([]adminRunSummary, error) {
	rows, err := s.db.Query(ctx, `
SELECT id, mode, coalesce(source, 'cli'), status, tester_agent_id, editor_agent_id, jsonb_array_length(tasks),
       reserved_cents, charged_cents, created_at, completed_at
FROM runs
WHERE user_id=$1
ORDER BY created_at DESC
LIMIT 25
`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	runs := []adminRunSummary{}
	for rows.Next() {
		var run adminRunSummary
		if err := rows.Scan(&run.ID, &run.Mode, &run.Source, &run.Status, &run.TesterAgentID, &run.EditorAgentID, &run.TaskCount, &run.ReservedCents, &run.ChargedCents, &run.CreatedAt, &run.CompletedAt); err != nil {
			return nil, err
		}
		runs = append(runs, run)
	}
	return runs, rows.Err()
}

type scanRow interface {
	Scan(dest ...any) error
}

func scanAdminUserSummary(row scanRow) (adminUserSummary, error) {
	var summary adminUserSummary
	var displayName sql.NullString
	err := row.Scan(
		&summary.ID,
		&summary.Email,
		&displayName,
		&summary.CreatedAt,
		&summary.LastActiveAt,
		&summary.BalanceCents,
		&summary.CreditGrantedCents,
		&summary.CreditSpentCents,
		&summary.RunCount,
		&summary.ActiveRunCount,
	)
	if err != nil {
		return adminUserSummary{}, err
	}
	if displayName.Valid {
		summary.DisplayName = &displayName.String
	}
	return summary, nil
}

var adminSearchPatternEscaper = strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)

func escapeAdminSearchPattern(query string) string {
	return adminSearchPatternEscaper.Replace(query)
}

func adminSearchLimit(raw string) int {
	if raw == "" {
		return 20
	}
	limit, err := strconv.Atoi(raw)
	if err != nil {
		return 20
	}
	if limit < 1 {
		return 1
	}
	if limit > 50 {
		return 50
	}
	return limit
}

func normalizeAdminGrantNote(raw *string) (*string, error) {
	if raw == nil {
		return nil, nil
	}
	note := strings.TrimSpace(*raw)
	if note == "" {
		return nil, nil
	}
	if len(note) > 500 {
		return nil, fmt.Errorf("note must be at most 500 characters")
	}
	return &note, nil
}

func parseAdminUSDToCents(raw string) (int64, error) {
	v := strings.TrimSpace(raw)
	if strings.HasPrefix(v, "$") {
		v = strings.TrimSpace(strings.TrimPrefix(v, "$"))
	}
	if v == "" {
		return 0, fmt.Errorf("amount_usd is required")
	}
	if strings.HasPrefix(v, "+") || strings.HasPrefix(v, "-") {
		return 0, fmt.Errorf("amount_usd must be a positive dollar amount")
	}
	whole, frac, hasFrac := strings.Cut(v, ".")
	if whole == "" || !asciiDigits(whole) {
		return 0, fmt.Errorf("amount_usd must be a positive dollar amount")
	}
	if !hasFrac {
		frac = "00"
	} else {
		if frac == "" || len(frac) > 2 || !asciiDigits(frac) {
			return 0, fmt.Errorf("amount_usd can include at most two decimal places")
		}
		for len(frac) < 2 {
			frac += "0"
		}
	}
	dollars, err := strconv.ParseInt(whole, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("amount_usd is too large")
	}
	cents, err := strconv.ParseInt(frac, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("amount_usd must be a positive dollar amount")
	}
	const maxInt64 = int64(9223372036854775807)
	if dollars > (maxInt64-cents)/100 {
		return 0, fmt.Errorf("amount_usd is too large")
	}
	total := dollars*100 + cents
	if total <= 0 {
		return 0, fmt.Errorf("amount_usd must be greater than zero")
	}
	return total, nil
}

func asciiDigits(v string) bool {
	for _, ch := range v {
		if ch < '0' || ch > '9' {
			return false
		}
	}
	return true
}

func usdStringFromCents(cents int64) string {
	return fmt.Sprintf("%d.%02d", cents/100, cents%100)
}
