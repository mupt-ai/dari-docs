package managedservice

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/mupt-ai/dari-docs/internal/bundle"
	"github.com/mupt-ai/dari-docs/internal/dari"
	"github.com/mupt-ai/dari-docs/internal/runner"
)

var (
	errBundleTooLarge      = errors.New("bundle too large")
	errBundleStageInternal = errors.New("bundle staging failed")
	errRunFeedbackLoad     = errors.New("run feedback unavailable")
)

type activeRunLimitError struct {
	Limit int
}

func (e *activeRunLimitError) Error() string {
	noun := "runs"
	if e.Limit == 1 {
		noun = "run"
	}
	return fmt.Sprintf("you already have %d active managed %s; wait for one to finish before starting another run", e.Limit, noun)
}

type insufficientCreditsError struct {
	Need    int64
	Balance int64
}

func (e *insufficientCreditsError) Error() string {
	return fmt.Sprintf("insufficient credits: need %s, balance %s", formatCents(e.Need), formatCents(e.Balance))
}

func (s *Server) handleRuns(w http.ResponseWriter, r *http.Request, u user) {
	switch r.Method {
	case http.MethodGet:
		s.handleListRuns(w, r, u)
		return
	case http.MethodPost:
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	maxMultipartBytes := s.cfg.MaxBundleBytes + 1<<20
	if r.ContentLength > maxMultipartBytes {
		writeError(w, http.StatusRequestEntityTooLarge, "bundle exceeds managed size limit")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxMultipartBytes)
	mr, err := r.MultipartReader()
	if err != nil {
		writeLoggedError(w, http.StatusBadRequest, "invalid multipart form", err)
		return
	}

	var (
		mode               string
		tasks              []string
		liveVerify         bool
		runtimeSecretJSON  string
		runtimeSecretNames []string
		runtimeNonce       []byte
		runtimeCiphertext  []byte
		testerLLMIDs       []string
		editorLLMID        string
		tmpPath            string
		b                  bundle.Result
		bundleName         string
		reserve            int64
	)
	for {
		part, err := mr.NextPart()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			if isRequestBodyTooLarge(err) {
				writeError(w, http.StatusRequestEntityTooLarge, "bundle exceeds managed size limit")
				return
			}
			writeLoggedError(w, http.StatusBadRequest, "invalid multipart form", err)
			return
		}
		switch part.FormName() {
		case "mode":
			v, err := readTextPart(part, 64)
			if err != nil {
				writeError(w, http.StatusBadRequest, "mode field is too large")
				return
			}
			mode = strings.TrimSpace(v)
			if mode != "check" && mode != "optimize" {
				writeError(w, http.StatusBadRequest, "mode must be check or optimize")
				return
			}
			if !requireScope(w, u, runModeScope(mode)) {
				return
			}
		case "tasks_json":
			v, err := readTextPart(part, maxTasksJSONFieldBytes(s.cfg.MaxTasksPerRun, s.cfg.MaxTaskBytes))
			if err != nil {
				writeError(w, http.StatusBadRequest, "tasks_json field is too large")
				return
			}
			tasks, err = parseManagedTasksJSON(v, s.cfg.MaxTasksPerRun, s.cfg.MaxTaskBytes)
			if err != nil {
				writeError(w, http.StatusBadRequest, err.Error())
				return
			}
		case "live_verify":
			v, err := readTextPart(part, 16)
			if err != nil {
				writeError(w, http.StatusBadRequest, "live_verify field is too large")
				return
			}
			liveVerify = strings.TrimSpace(v) == "true"
		case "runtime_secrets_json":
			v, err := readTextPart(part, managedMaxRuntimeSecretsBytes)
			if err != nil {
				writeError(w, http.StatusBadRequest, "runtime_secrets_json field is too large")
				return
			}
			runtimeSecretJSON = v
			runtimeSecretNames, err = runtimeSecretNamesFromJSON(v)
			if err != nil {
				writeError(w, http.StatusBadRequest, err.Error())
				return
			}
		case "feedback_llm_ids_json":
			v, err := readTextPart(part, 1024)
			if err != nil {
				writeError(w, http.StatusBadRequest, "feedback_llm_ids_json field is too large")
				return
			}
			testerLLMIDs, err = parseManagedLLMIDsJSON(v)
			if err != nil {
				writeError(w, http.StatusBadRequest, err.Error())
				return
			}
		case "editor_llm_id":
			v, err := readTextPart(part, 128)
			if err != nil {
				writeError(w, http.StatusBadRequest, "editor_llm_id field is too large")
				return
			}
			editorLLMID, err = normalizeManagedLLMID(v)
			if err != nil {
				writeError(w, http.StatusBadRequest, err.Error())
				return
			}
		case "bundle":
			if mode == "" || tasks == nil {
				writeError(w, http.StatusBadRequest, "mode and tasks_json must be sent before bundle")
				return
			}
			if runtimeSecretJSON != "" && !liveVerify {
				writeError(w, http.StatusBadRequest, "runtime secrets require live_verify=true")
				return
			}
			if runtimeSecretJSON != "" {
				runtimeNonce, runtimeCiphertext, err = s.encryptRuntimeSecrets([]byte(runtimeSecretJSON))
				if err != nil {
					writeLoggedError(w, http.StatusInternalServerError, "could not encrypt runtime secrets", err)
					return
				}
			}
			if len(testerLLMIDs) == 0 {
				testerLLMIDs = defaultManagedTesterLLMIDs()
			}
			if editorLLMID == "" {
				editorLLMID = managedDefaultLLMID
			}
			reserve = reserveCentsForRun(mode, len(tasks), len(testerLLMIDs), s.cfg)
			if err := s.preflightRun(r.Context(), u.ID, reserve); err != nil {
				var activeErr *activeRunLimitError
				if errors.As(err, &activeErr) {
					writeError(w, http.StatusConflict, err.Error())
					return
				}
				var creditErr *insufficientCreditsError
				if errors.As(err, &creditErr) {
					writeError(w, http.StatusPaymentRequired, creditErr.Error())
					return
				}
				writeLoggedError(w, http.StatusInternalServerError, "could not reserve managed run", err)
				return
			}
			tmpPath, b, err = s.stageManagedBundle(part)
			if err != nil {
				if errors.Is(err, errBundleTooLarge) {
					writeError(w, http.StatusRequestEntityTooLarge, "bundle exceeds managed size limit")
					return
				}
				if isRequestBodyTooLarge(err) {
					writeError(w, http.StatusRequestEntityTooLarge, "bundle exceeds managed size limit")
					return
				}
				if errors.Is(err, errBundleStageInternal) {
					writeLoggedError(w, http.StatusInternalServerError, "could not stage bundle", err)
					return
				}
				writeError(w, http.StatusBadRequest, err.Error())
				return
			}
			defer os.Remove(tmpPath)
			bundleName = filepath.Base(part.FileName())
			if bundleName == "." || bundleName == string(filepath.Separator) || bundleName == "" {
				bundleName = "input-docs-bundle.tar.gz"
			}
		default:
			writeError(w, http.StatusBadRequest, "unexpected multipart field")
			return
		}
		if tmpPath != "" {
			break
		}
	}
	if mode == "" {
		writeError(w, http.StatusBadRequest, "mode must be check or optimize")
		return
	}
	if tasks == nil {
		writeError(w, http.StatusBadRequest, "tasks_json must be a JSON string array")
		return
	}
	if tmpPath == "" {
		writeError(w, http.StatusBadRequest, "bundle file is required")
		return
	}
	runID := "run_" + randomToken(18)
	taskJSON, err := json.Marshal(tasks)
	if err != nil {
		writeLoggedError(w, http.StatusInternalServerError, "could not encode managed run tasks", err)
		return
	}
	testerLLMIDsJSON, err := json.Marshal(testerLLMIDs)
	if err != nil {
		writeLoggedError(w, http.StatusInternalServerError, "could not encode managed run LLM IDs", err)
		return
	}
	secretNamesJSON, err := json.Marshal(runtimeSecretNames)
	if err != nil {
		writeLoggedError(w, http.StatusInternalServerError, "could not encode runtime secret names", err)
		return
	}
	if err := s.reserveRun(r.Context(), u.ID, runID, mode, taskJSON, testerLLMIDsJSON, editorLLMID, b, reserve, liveVerify, secretNamesJSON, runtimeNonce, runtimeCiphertext); err != nil {
		if errors.Is(err, errNoActiveManagedAgentRelease) {
			writeError(w, http.StatusServiceUnavailable, "managed agents are not configured")
			return
		}
		var activeErr *activeRunLimitError
		if errors.As(err, &activeErr) {
			writeError(w, http.StatusConflict, err.Error())
			return
		}
		var creditErr *insufficientCreditsError
		if errors.As(err, &creditErr) {
			writeError(w, http.StatusPaymentRequired, creditErr.Error())
			return
		}
		writeLoggedError(w, http.StatusInternalServerError, "could not reserve managed run", err)
		return
	}
	f, err := os.Open(tmpPath)
	if err != nil {
		s.failBeforeQueue(r.Context(), runID, reserve, persistedErrBundleStageFailed)
		writeLoggedError(w, http.StatusInternalServerError, "could not read staged bundle", err)
		return
	}
	up, err := s.dari.UploadReader(r.Context(), bundleName, f)
	_ = f.Close()
	if err != nil {
		s.failBeforeQueue(r.Context(), runID, reserve, persistedErrBundleUploadFailed)
		writeLoggedError(w, http.StatusBadGateway, "could not upload bundle to Dari", err)
		return
	}
	_, err = s.db.Exec(r.Context(), `
UPDATE runs SET status=$2, bundle_file_id=$3, updated_at=now() WHERE id=$1
`, runID, statusQueued, up.ID)
	if err != nil {
		s.failBeforeQueue(r.Context(), runID, reserve, persistedErrRunQueueFailed)
		writeLoggedError(w, http.StatusInternalServerError, "could not queue managed run", err)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]string{"run_id": runID, "status": statusQueued})
}

type runListResponse struct {
	Runs       []runListItem `json:"runs"`
	NextCursor string        `json:"next_cursor,omitempty"`
}

type runListItem struct {
	ID                   string          `json:"id"`
	Mode                 string          `json:"mode"`
	Status               string          `json:"status"`
	Tasks                []string        `json:"tasks"`
	TaskCount            int             `json:"task_count"`
	CreatedAt            time.Time       `json:"created_at"`
	CompletedAt          *time.Time      `json:"completed_at,omitempty"`
	ReservedCents        int64           `json:"reserved_cents"`
	ChargedCents         int64           `json:"charged_cents"`
	Estimated            bool            `json:"estimated"`
	Error                string          `json:"error,omitempty"`
	LLMs                 []runLLMSummary `json:"llms"`
	UpdatedDocsAvailable bool            `json:"updated_docs_available"`
}

type runLLMSummary struct {
	Role  string `json:"role"`
	LLMID string `json:"llm_id"`
	Count int    `json:"count"`
}

type runSessionSummary struct {
	Kind        string     `json:"kind"`
	TaskIndex   int        `json:"task_index"`
	Status      string     `json:"status"`
	LLMID       string     `json:"llm_id"`
	CreatedAt   time.Time  `json:"created_at"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
}

type runListParams struct {
	Limit     int
	Offset    int
	Sort      string
	Direction string
}

func (s *Server) handleListRuns(w http.ResponseWriter, r *http.Request, u user) {
	if !requireScope(w, u, scopeManagedRead) {
		return
	}
	params, err := parseRunListParams(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	order, ok := runListOrderExpr(params.Sort)
	if !ok {
		writeError(w, http.StatusBadRequest, "unsupported sort")
		return
	}
	direction := "DESC"
	if params.Direction == "asc" {
		direction = "ASC"
	}
	query := fmt.Sprintf(`
SELECT id, mode, status, tasks, created_at, completed_at, reserved_cents, charged_cents,
       coalesce(cost_status,''), coalesce(error,''), coalesce(editor_session_id,'')
FROM runs
WHERE user_id=$1
ORDER BY %s %s, id %s
LIMIT $2 OFFSET $3
`, order.Expr, direction, direction)
	args := []any{u.ID, params.Limit + 1, params.Offset}
	args = append(args, order.Args...)
	rows, err := s.db.Query(r.Context(), query, args...)
	if err != nil {
		writeLoggedError(w, http.StatusInternalServerError, "could not list runs", err)
		return
	}
	defer rows.Close()
	runs := make([]runListItem, 0, params.Limit)
	runIDs := make([]string, 0, params.Limit)
	for rows.Next() {
		var item runListItem
		var tasksJSON []byte
		var costStatus, editorSessionID string
		if err := rows.Scan(&item.ID, &item.Mode, &item.Status, &tasksJSON, &item.CreatedAt, &item.CompletedAt, &item.ReservedCents, &item.ChargedCents, &costStatus, &item.Error, &editorSessionID); err != nil {
			writeLoggedError(w, http.StatusInternalServerError, "could not list runs", err)
			return
		}
		_ = json.Unmarshal(tasksJSON, &item.Tasks)
		item.TaskCount = len(item.Tasks)
		item.Estimated = costStatus == "estimated"
		item.UpdatedDocsAvailable = item.Status == statusCompleted && editorSessionID != ""
		runs = append(runs, item)
		runIDs = append(runIDs, item.ID)
	}
	if rows.Err() != nil {
		writeLoggedError(w, http.StatusInternalServerError, "could not list runs", rows.Err())
		return
	}
	nextCursor := ""
	if len(runs) > params.Limit {
		runs = runs[:params.Limit]
		runIDs = runIDs[:params.Limit]
		nextCursor = encodeRunListCursor(params.Offset + params.Limit)
	}
	llms, err := s.loadRunLLMSummaries(r.Context(), u.ID, runIDs)
	if err != nil {
		writeLoggedError(w, http.StatusInternalServerError, "could not list run llms", err)
		return
	}
	for i := range runs {
		runs[i].LLMs = llms[runs[i].ID]
		if runs[i].LLMs == nil {
			runs[i].LLMs = []runLLMSummary{}
		}
	}
	writeJSON(w, http.StatusOK, runListResponse{Runs: runs, NextCursor: nextCursor})
}

func parseRunListParams(r *http.Request) (runListParams, error) {
	q := r.URL.Query()
	limit := 50
	if raw := strings.TrimSpace(q.Get("limit")); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n <= 0 {
			return runListParams{}, errors.New("limit must be a positive integer")
		}
		limit = n
	}
	if limit > 100 {
		limit = 100
	}
	offset := 0
	if raw := strings.TrimSpace(q.Get("cursor")); raw != "" {
		n, err := decodeRunListCursor(raw)
		if err != nil {
			return runListParams{}, errors.New("invalid cursor")
		}
		offset = n
	}
	sort := strings.TrimSpace(q.Get("sort"))
	if sort == "" {
		sort = "created_at"
	}
	direction := strings.ToLower(strings.TrimSpace(q.Get("direction")))
	if direction == "" {
		direction = "desc"
	}
	if direction != "asc" && direction != "desc" {
		return runListParams{}, errors.New("direction must be asc or desc")
	}
	return runListParams{Limit: limit, Offset: offset, Sort: sort, Direction: direction}, nil
}

type runListOrder struct {
	Expr string
	Args []any
}

func runListOrderExpr(sort string) (runListOrder, bool) {
	switch sort {
	case "status":
		return runListOrder{Expr: "status"}, true
	case "type", "mode":
		return runListOrder{Expr: "mode"}, true
	case "task":
		return runListOrder{Expr: "tasks::text"}, true
	case "cost", "charged_cents":
		return runListOrder{Expr: "charged_cents"}, true
	case "created", "created_at":
		return runListOrder{Expr: "created_at"}, true
	case "completed", "completed_at":
		return runListOrder{Expr: "coalesce(completed_at, '0001-01-01'::timestamptz)"}, true
	case "llms":
		return runListOrder{
			Expr: `(array_to_string(ARRAY(SELECT jsonb_array_elements_text(runs.tester_llm_ids)), ',') || ',' || runs.editor_llm_id)`,
		}, true
	default:
		return runListOrder{}, false
	}
}

func encodeRunListCursor(offset int) string {
	return base64.RawURLEncoding.EncodeToString([]byte(strconv.Itoa(offset)))
}

func decodeRunListCursor(raw string) (int, error) {
	data, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return 0, err
	}
	offset, err := strconv.Atoi(string(data))
	if err != nil || offset < 0 {
		return 0, errors.New("invalid offset")
	}
	return offset, nil
}

func (s *Server) loadRunLLMSummaries(ctx context.Context, userID string, runIDs []string) (map[string][]runLLMSummary, error) {
	out := make(map[string][]runLLMSummary, len(runIDs))
	if len(runIDs) == 0 {
		return out, nil
	}
	plannedRows, err := s.db.Query(ctx, `
SELECT id, mode, jsonb_array_length(tasks), tester_llm_ids, editor_llm_id
FROM runs
WHERE user_id=$1 AND id = ANY($2)
`, userID, runIDs)
	if err != nil {
		return nil, err
	}
	for plannedRows.Next() {
		var runID, mode, editorLLMID string
		var taskCount int
		var testerLLMIDsJSON []byte
		if err := plannedRows.Scan(&runID, &mode, &taskCount, &testerLLMIDsJSON, &editorLLMID); err != nil {
			plannedRows.Close()
			return nil, err
		}
		var testerLLMIDs []string
		if err := json.Unmarshal(testerLLMIDsJSON, &testerLLMIDs); err != nil {
			plannedRows.Close()
			return nil, err
		}
		testerLLMIDs, err = normalizeManagedLLMIDs(testerLLMIDs)
		if err != nil {
			plannedRows.Close()
			return nil, err
		}
		for _, llmID := range testerLLMIDs {
			out[runID] = append(out[runID], runLLMSummary{Role: "tester", LLMID: llmID, Count: taskCount})
		}
		if mode == "optimize" {
			editorLLMID, err = normalizeManagedLLMID(editorLLMID)
			if err != nil {
				plannedRows.Close()
				return nil, err
			}
			out[runID] = append(out[runID], runLLMSummary{Role: "editor", LLMID: editorLLMID, Count: 1})
		}
	}
	if err := plannedRows.Err(); err != nil {
		plannedRows.Close()
		return nil, err
	}
	plannedRows.Close()

	rows, err := s.db.Query(ctx, `
SELECT rs.run_id, rs.kind, coalesce(nullif(rs.llm_id,''), $3) AS llm_id, count(*)
FROM run_sessions rs
JOIN runs r ON r.id = rs.run_id
WHERE r.user_id=$1 AND rs.run_id = ANY($2)
GROUP BY rs.run_id, rs.kind, coalesce(nullif(rs.llm_id,''), $3)
ORDER BY rs.run_id, CASE rs.kind WHEN 'tester' THEN 1 WHEN 'editor' THEN 2 ELSE 3 END, llm_id
`, userID, runIDs, managedDefaultLLMID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	actual := make(map[string][]runLLMSummary, len(runIDs))
	for rows.Next() {
		var runID string
		var item runLLMSummary
		if err := rows.Scan(&runID, &item.Role, &item.LLMID, &item.Count); err != nil {
			return nil, err
		}
		actual[runID] = append(actual[runID], item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for runID, llms := range actual {
		if len(llms) > 0 {
			out[runID] = llms
		}
	}
	return out, nil
}

func (s *Server) stageManagedBundle(part *multipart.Part) (string, bundle.Result, error) {
	tmp, err := os.CreateTemp("", "dari-docs-bundle-*.tar.gz")
	if err != nil {
		return "", bundle.Result{}, fmt.Errorf("%w: create temporary bundle file: %v", errBundleStageInternal, err)
	}
	tmpPath := tmp.Name()
	keep := false
	defer func() {
		if !keep {
			_ = os.Remove(tmpPath)
		}
	}()

	written, err := io.Copy(tmp, io.LimitReader(part, s.cfg.MaxBundleBytes+1))
	closeErr := tmp.Close()
	if err != nil {
		return "", bundle.Result{}, err
	}
	if closeErr != nil {
		return "", bundle.Result{}, fmt.Errorf("%w: finish writing bundle file: %v", errBundleStageInternal, closeErr)
	}
	if written > s.cfg.MaxBundleBytes {
		return "", bundle.Result{}, errBundleTooLarge
	}
	b, err := bundle.ReadWithOptions(tmpPath, bundle.ReadOptions{
		MaxUncompressedBytes: s.cfg.BundleMaxUncompressedBytes,
		MaxFileBytes:         s.cfg.BundleMaxFileBytes,
	})
	if err != nil {
		return "", bundle.Result{}, err
	}
	keep = true
	return tmpPath, b, nil
}

func (s *Server) preflightRun(ctx context.Context, userID string, reserve int64) error {
	var active int
	if err := s.db.QueryRow(ctx, `SELECT count(*) FROM runs WHERE user_id=$1 AND status IN ($2,$3,$4,$5)`, userID, statusUploading, statusQueued, statusStarting, statusRunning).Scan(&active); err != nil {
		return err
	}
	limit := s.maxActiveRunsPerUser()
	if active >= limit {
		return &activeRunLimitError{Limit: limit}
	}
	balance, err := s.balanceCents(ctx, userID)
	if err != nil {
		return err
	}
	if balance < reserve {
		return &insufficientCreditsError{Need: reserve, Balance: balance}
	}
	return nil
}

func parseManagedTasksJSON(raw string, maxTasks int, maxTaskBytes int64) ([]string, error) {
	var tasks []string
	if err := json.Unmarshal([]byte(raw), &tasks); err != nil {
		return nil, errors.New("tasks_json must be a JSON string array")
	}
	tasks = compactTasks(tasks)
	if len(tasks) == 0 {
		return nil, errors.New("at least one task is required")
	}
	if len(tasks) > maxTasks {
		return nil, fmt.Errorf("at most %d tasks are allowed per managed run", maxTasks)
	}
	if err := validateManagedTasks(tasks, maxTaskBytes); err != nil {
		return nil, err
	}
	return tasks, nil
}

func reserveCentsForRun(mode string, taskCount int, testerLLMCount int, cfg Config) int64 {
	if testerLLMCount <= 0 {
		testerLLMCount = 1
	}
	reserve := int64(taskCount*testerLLMCount) * cfg.TesterReserveCents
	if mode == "optimize" {
		reserve += cfg.EditorReserveCents
	}
	return reserve
}

func runModeScope(mode string) string {
	if mode == "optimize" {
		return scopeManagedOptimize
	}
	return scopeManagedCheck
}

func (s *Server) maxActiveRunsPerUser() int {
	if s.cfg.MaxActiveRunsPerUser > 0 {
		return s.cfg.MaxActiveRunsPerUser
	}
	return int(managedMaxActiveRunsPerUser)
}

func (s *Server) reserveRun(ctx context.Context, userID, runID, mode string, taskJSON, testerLLMIDsJSON []byte, editorLLMID string, b bundle.Result, reserve int64, liveVerify bool, secretNamesJSON, runtimeNonce, runtimeCiphertext []byte) error {
	release, err := s.loadManagedAgentRelease(ctx)
	if err != nil {
		return err
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `SELECT id FROM users WHERE id=$1 FOR UPDATE`, userID); err != nil {
		return err
	}
	var active int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM runs WHERE user_id=$1 AND status IN ($2,$3,$4,$5)`, userID, statusUploading, statusQueued, statusStarting, statusRunning).Scan(&active); err != nil {
		return err
	}
	limit := s.maxActiveRunsPerUser()
	if active >= limit {
		return &activeRunLimitError{Limit: limit}
	}
	balance, err := balanceCentsTx(ctx, tx, userID)
	if err != nil {
		return err
	}
	if balance < reserve {
		return &insufficientCreditsError{Need: reserve, Balance: balance}
	}
	if editorLLMID == "" {
		editorLLMID = managedDefaultLLMID
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO runs (id, user_id, mode, status, tasks, tester_llm_ids, editor_llm_id, tester_agent_id, tester_version_id, editor_agent_id, editor_version_id, bundle_sha256, bundle_files, reserved_cents, live_verify, runtime_secret_names, runtime_secrets_nonce, runtime_secrets_ciphertext)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18)
		`, runID, userID, mode, statusUploading, taskJSON, testerLLMIDsJSON, editorLLMID, release.TesterAgentID, release.TesterVersionID, release.EditorAgentID, release.EditorVersionID, b.SHA256, len(b.Manifest.Files), reserve, liveVerify, secretNamesJSON, runtimeNonce, runtimeCiphertext); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
INSERT INTO credit_ledger (id, user_id, amount_cents, kind, source_id, run_id)
VALUES ($1, $2, $3, 'run_reservation', $4, $5)
`, "led_"+randomToken(18), userID, -reserve, "reservation:"+runID, runID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Server) failBeforeQueue(_ context.Context, runID string, reserve int64, code persistedErrorCode) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	_, _ = s.db.Exec(ctx, `
UPDATE runs SET status=$2, error=$3, updated_at=now(), completed_at=now() WHERE id=$1
`, runID, statusFailed, persistedErrorString(code))
	s.clearRuntimeSecrets(ctx, runID)
	s.releaseReservation(ctx, runID, reserve)
}

func (s *Server) releaseReservation(ctx context.Context, runID string, reserve int64) {
	if reserve <= 0 {
		return
	}
	_, _ = s.db.Exec(ctx, `
	INSERT INTO credit_ledger (id, user_id, amount_cents, kind, source_id, run_id)
	SELECT $1, user_id, $2, 'run_reservation_release', $3, id FROM runs WHERE id=$4
ON CONFLICT (source_id) DO NOTHING
`, "led_"+randomToken(18), reserve, "release:"+runID, runID)
}

func (s *Server) handleRunByID(w http.ResponseWriter, r *http.Request, u user) {
	rest := strings.TrimPrefix(r.URL.Path, "/v1/runs/")
	if strings.HasSuffix(rest, "/updated-docs.zip") {
		runID := strings.TrimSuffix(rest, "/updated-docs.zip")
		s.handleUpdatedZip(w, r, u, strings.Trim(runID, "/"))
		return
	}
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if !requireScope(w, u, scopeManagedRead) {
		return
	}
	runID := strings.Trim(rest, "/")
	rs, err := s.loadRunStatus(r.Context(), u.ID, runID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "run not found")
		} else if errors.Is(err, errRunFeedbackLoad) {
			writeLoggedError(w, http.StatusBadGateway, "could not load run feedback", err)
		} else {
			writeLoggedError(w, http.StatusInternalServerError, "could not load run status", err)
		}
		return
	}
	writeJSON(w, http.StatusOK, rs)
}

func (s *Server) handleUpdatedZip(w http.ResponseWriter, r *http.Request, u user, runID string) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if !requireScope(w, u, scopeManagedOptimize) {
		return
	}
	var editorSessionID string
	err := s.db.QueryRow(r.Context(), `SELECT coalesce(editor_session_id,'') FROM runs WHERE id=$1 AND user_id=$2 AND status=$3`, runID, u.ID, statusCompleted).Scan(&editorSessionID)
	if err != nil || editorSessionID == "" {
		writeError(w, http.StatusNotFound, "updated docs are not available")
		return
	}
	var buf bytes.Buffer
	if err := s.dari.WriteWorkspaceZipWithLimit(r.Context(), editorSessionID, []string{"updated-docs"}, &buf, managedMaxUpdatedZipBytes); err != nil {
		writeError(w, http.StatusConflict, "updated docs are unavailable; rerun optimize")
		log.Printf("download updated docs for run %s: %v", runID, err)
		return
	}
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", `attachment; filename="updated-docs-workspace.zip"`)
	_, _ = w.Write(buf.Bytes())
}

type runStatusResponse struct {
	ID                   string              `json:"id"`
	Mode                 string              `json:"mode"`
	Status               string              `json:"status"`
	Error                string              `json:"error,omitempty"`
	Tasks                []string            `json:"tasks,omitempty"`
	TaskCount            int                 `json:"task_count"`
	CreatedAt            time.Time           `json:"created_at"`
	CompletedAt          *time.Time          `json:"completed_at,omitempty"`
	LLMs                 []runLLMSummary     `json:"llms"`
	Sessions             []runSessionSummary `json:"sessions"`
	FeedbackReports      []string            `json:"feedback_reports,omitempty"`
	AggregateFeedback    string              `json:"aggregate_feedback,omitempty"`
	UpdatedDocsAvailable bool                `json:"updated_docs_available"`
	ReservedCents        int64               `json:"reserved_cents"`
	ChargedCents         int64               `json:"charged_cents"`
	Estimated            bool                `json:"estimated"`
}

func (s *Server) loadRunStatus(ctx context.Context, userID, runID string) (runStatusResponse, error) {
	var rs runStatusResponse
	var editorSessionID string
	var tasksJSON []byte
	var costStatus string
	err := s.db.QueryRow(ctx, `
SELECT id, mode, status, tasks, coalesce(error,''), coalesce(editor_session_id,''), reserved_cents, charged_cents,
       coalesce(cost_status,''), created_at, completed_at
FROM runs WHERE id=$1 AND user_id=$2
`, runID, userID).Scan(&rs.ID, &rs.Mode, &rs.Status, &tasksJSON, &rs.Error, &editorSessionID, &rs.ReservedCents, &rs.ChargedCents, &costStatus, &rs.CreatedAt, &rs.CompletedAt)
	if err != nil {
		return rs, err
	}
	_ = json.Unmarshal(tasksJSON, &rs.Tasks)
	rs.TaskCount = len(rs.Tasks)
	rs.UpdatedDocsAvailable = rs.Status == statusCompleted && editorSessionID != ""
	rs.Estimated = costStatus == "estimated"
	llms, err := s.loadRunLLMSummaries(ctx, userID, []string{runID})
	if err != nil {
		return rs, err
	}
	rs.LLMs = llms[runID]
	if rs.LLMs == nil {
		rs.LLMs = []runLLMSummary{}
	}
	sessions, err := s.loadRunSessionSummaries(ctx, userID, runID)
	if err != nil {
		return rs, err
	}
	rs.Sessions = sessions
	if rs.Sessions == nil {
		rs.Sessions = []runSessionSummary{}
	}
	if rs.Status == statusCompleted || rs.Status == statusFailed {
		reports, err := s.completedTesterReports(ctx, runID)
		if err != nil {
			return rs, err
		}
		rs.FeedbackReports = reports
		if len(reports) > 0 {
			rs.AggregateFeedback = runner.AggregateFeedback(reports)
		}
	}
	return rs, nil
}

func (s *Server) loadRunSessionSummaries(ctx context.Context, userID, runID string) ([]runSessionSummary, error) {
	rows, err := s.db.Query(ctx, `
SELECT rs.kind,
       rs.task_index,
       rs.status,
       coalesce(nullif(rs.llm_id,''), $3),
       rs.created_at,
       rs.completed_at
FROM run_sessions rs
JOIN runs r ON r.id = rs.run_id
WHERE r.id=$1 AND r.user_id=$2
ORDER BY CASE rs.kind WHEN 'tester' THEN 1 WHEN 'editor' THEN 2 ELSE 3 END, rs.task_index, rs.llm_id, rs.created_at
`, runID, userID, managedDefaultLLMID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	sessions := []runSessionSummary{}
	for rows.Next() {
		var session runSessionSummary
		if err := rows.Scan(
			&session.Kind,
			&session.TaskIndex,
			&session.Status,
			&session.LLMID,
			&session.CreatedAt,
			&session.CompletedAt,
		); err != nil {
			return nil, err
		}
		sessions = append(sessions, session)
	}
	return sessions, rows.Err()
}

func (s *Server) completedTesterReports(ctx context.Context, runID string) ([]string, error) {
	rows, err := s.db.Query(ctx, `
SELECT session_id,
       task_index,
       coalesce(nullif(llm_id,''), $3)
FROM run_sessions
WHERE run_id=$1 AND kind='tester' AND status=$2
ORDER BY task_index, llm_id, created_at
`, runID, statusCompleted, managedDefaultLLMID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	type testerSession struct {
		id        string
		taskIndex int
		llmID     string
	}
	var sessions []testerSession
	for rows.Next() {
		var session testerSession
		if err := rows.Scan(&session.id, &session.taskIndex, &session.llmID); err != nil {
			return nil, err
		}
		sessions = append(sessions, session)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	reports := make([]string, 0, len(sessions))
	for _, session := range sessions {
		tr, err := s.dari.GetTranscript(ctx, session.id)
		if err != nil {
			return nil, fmt.Errorf("%w: get transcript %s: %v", errRunFeedbackLoad, session.id, err)
		}
		reports = append(reports, formatManagedFeedbackReport(session.taskIndex, session.llmID, dari.FinalAssistantText(tr)))
	}
	return reports, nil
}

func readUploadedFormFile(r *http.Request, name string, maxBytes int64) ([]byte, error) {
	file, _, err := r.FormFile(name)
	if err != nil {
		return nil, fmt.Errorf("missing %s", name)
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, maxBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", name, err)
	}
	if int64(len(data)) > maxBytes {
		return nil, fmt.Errorf("%s exceeds managed size limit", name)
	}
	return data, nil
}

func compactTasks(in []string) []string {
	out := make([]string, 0, len(in))
	for _, t := range in {
		t = strings.TrimSpace(t)
		if t != "" {
			out = append(out, t)
		}
	}
	return out
}

func readTextPart(part *multipart.Part, maxBytes int64) (string, error) {
	data, err := io.ReadAll(io.LimitReader(part, maxBytes+1))
	if err != nil {
		return "", err
	}
	if int64(len(data)) > maxBytes {
		return "", fmt.Errorf("multipart field exceeds %d bytes", maxBytes)
	}
	return string(data), nil
}

func maxTasksJSONFieldBytes(maxTasks int, maxTaskBytes int64) int64 {
	limit := int64(maxTasks)*maxTaskBytes + 4096
	if limit < 4096 {
		return 4096
	}
	if limit > 1<<20 {
		return 1 << 20
	}
	return limit
}

func validateManagedTasks(tasks []string, maxTaskBytes int64) error {
	if maxTaskBytes <= 0 {
		maxTaskBytes = 10000
	}
	for i, task := range tasks {
		if int64(len(task)) > maxTaskBytes {
			return fmt.Errorf("task %d exceeds managed task text limit of %d bytes", i+1, maxTaskBytes)
		}
	}
	return nil
}
