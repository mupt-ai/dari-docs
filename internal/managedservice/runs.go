package managedservice

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
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
	if r.Method != http.MethodPost {
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
			reserve = reserveCentsForRun(mode, len(tasks), s.cfg)
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
	secretNamesJSON, err := json.Marshal(runtimeSecretNames)
	if err != nil {
		writeLoggedError(w, http.StatusInternalServerError, "could not encode runtime secret names", err)
		return
	}
	if err := s.reserveRun(r.Context(), u.ID, runID, mode, taskJSON, b, reserve, liveVerify, secretNamesJSON, runtimeNonce, runtimeCiphertext); err != nil {
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

func reserveCentsForRun(mode string, taskCount int, cfg Config) int64 {
	reserve := int64(taskCount) * cfg.TesterReserveCents
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

func (s *Server) reserveRun(ctx context.Context, userID, runID, mode string, taskJSON []byte, b bundle.Result, reserve int64, liveVerify bool, secretNamesJSON, runtimeNonce, runtimeCiphertext []byte) error {
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
	if _, err := tx.Exec(ctx, `
		INSERT INTO runs (id, user_id, mode, status, tasks, tester_agent_id, tester_version_id, editor_agent_id, editor_version_id, bundle_sha256, bundle_files, reserved_cents, live_verify, runtime_secret_names, runtime_secrets_nonce, runtime_secrets_ciphertext)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16)
		`, runID, userID, mode, statusUploading, taskJSON, s.cfg.ManagedTesterAgentID, s.cfg.ManagedTesterVersionID, s.cfg.ManagedEditorAgentID, s.cfg.ManagedEditorVersionID, b.SHA256, len(b.Manifest.Files), reserve, liveVerify, secretNamesJSON, runtimeNonce, runtimeCiphertext); err != nil {
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
	ID                   string   `json:"id"`
	Mode                 string   `json:"mode"`
	Status               string   `json:"status"`
	Error                string   `json:"error,omitempty"`
	FeedbackReports      []string `json:"feedback_reports,omitempty"`
	AggregateFeedback    string   `json:"aggregate_feedback,omitempty"`
	UpdatedDocsAvailable bool     `json:"updated_docs_available"`
	ReservedCents        int64    `json:"reserved_cents"`
	ChargedCents         int64    `json:"charged_cents"`
}

func (s *Server) loadRunStatus(ctx context.Context, userID, runID string) (runStatusResponse, error) {
	var rs runStatusResponse
	var editorSessionID string
	err := s.db.QueryRow(ctx, `
SELECT id, mode, status, coalesce(error,''), coalesce(editor_session_id,''), reserved_cents, charged_cents
FROM runs WHERE id=$1 AND user_id=$2
`, runID, userID).Scan(&rs.ID, &rs.Mode, &rs.Status, &rs.Error, &editorSessionID, &rs.ReservedCents, &rs.ChargedCents)
	if err != nil {
		return rs, err
	}
	rs.UpdatedDocsAvailable = rs.Status == statusCompleted && editorSessionID != ""
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

func (s *Server) completedTesterReports(ctx context.Context, runID string) ([]string, error) {
	rows, err := s.db.Query(ctx, `
SELECT session_id
FROM run_sessions
WHERE run_id=$1 AND kind='tester' AND status=$2
ORDER BY task_index
`, runID, statusCompleted)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var sessionIDs []string
	for rows.Next() {
		var sessionID string
		if err := rows.Scan(&sessionID); err != nil {
			return nil, err
		}
		sessionIDs = append(sessionIDs, sessionID)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	reports := make([]string, 0, len(sessionIDs))
	for _, sessionID := range sessionIDs {
		tr, err := s.dari.GetTranscript(ctx, sessionID)
		if err != nil {
			return nil, fmt.Errorf("%w: get transcript %s: %v", errRunFeedbackLoad, sessionID, err)
		}
		reports = append(reports, dari.FinalAssistantText(tr))
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
