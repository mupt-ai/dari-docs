package managedservice

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/mupt-ai/dari-docs/internal/bundle"
	"github.com/mupt-ai/dari-docs/internal/dari"
	"github.com/mupt-ai/dari-docs/internal/runner"
)

type queuedRun struct {
	ID              string
	UserID          string
	Mode            string
	Tasks           []string
	TesterAgentID   string
	TesterVersionID string
	EditorAgentID   string
	EditorVersionID string
	BundleFileID    string
	BundleSHA256    string
	BundleFiles     int
	LiveVerify      bool
	SecretNames     []string
	ReservedCents   int64
}

type runSessionRecord struct {
	ID              string
	RunID           string
	Kind            string
	TaskIndex       int
	Status          string
	CreatedAt       time.Time
	LastPollErrorAt *time.Time
	LastPollError   string
}

type nextSession struct {
	Kind      string
	TaskIndex int
	AgentID   string
	VersionID string
	Prompt    string
}

func (s *Server) sessionStarterLoop(ctx context.Context) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		if err := s.recoverStaleUploadingRuns(ctx); err != nil {
			log.Printf("recover stale uploading runs: %v", err)
		}
		if err := s.recoverStaleStartingSessions(ctx); err != nil {
			log.Printf("recover stale starting sessions: %v", err)
		}
		if err := s.recoverStaleStartingRuns(ctx); err != nil {
			log.Printf("recover stale starting runs: %v", err)
		}
		if err := s.startAvailableSessions(ctx); err != nil {
			log.Printf("start sessions: %v", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (s *Server) sessionReconcilerLoop(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		if err := s.reconcileRunningSessions(ctx); err != nil {
			log.Printf("reconcile running sessions: %v", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (s *Server) settlementLoop(ctx context.Context) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	for {
		if err := s.settleUnsettledRuns(ctx); err != nil {
			log.Printf("settle unsettled runs: %v", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (s *Server) startAvailableSessions(ctx context.Context) error {
	for {
		run, ok, err := s.claimStartableRun(ctx)
		if err != nil {
			return err
		}
		if !ok {
			return nil
		}
		if err := s.startNextSession(ctx, run); err != nil {
			log.Printf("start run %s: %v", run.ID, err)
		}
	}
}

func (s *Server) claimStartableRun(ctx context.Context) (queuedRun, bool, error) {
	var run queuedRun
	var tasksJSON, secretNamesJSON []byte
	err := s.db.QueryRow(ctx, `
UPDATE runs r SET status=$1, updated_at=now()
WHERE r.id = (
  SELECT id FROM runs WHERE status=$2 ORDER BY created_at LIMIT 1 FOR UPDATE SKIP LOCKED
	)
	RETURNING r.id, r.user_id, r.mode, r.tasks,
	  r.tester_agent_id,
	  r.tester_version_id,
	  r.editor_agent_id,
	  r.editor_version_id,
	  coalesce(r.bundle_file_id,''), r.bundle_sha256, r.bundle_files, r.live_verify, r.runtime_secret_names, r.reserved_cents
	`, statusStarting, statusQueued).Scan(
		&run.ID, &run.UserID, &run.Mode, &tasksJSON,
		&run.TesterAgentID, &run.TesterVersionID, &run.EditorAgentID, &run.EditorVersionID,
		&run.BundleFileID, &run.BundleSHA256, &run.BundleFiles, &run.LiveVerify, &secretNamesJSON, &run.ReservedCents,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return queuedRun{}, false, nil
	}
	if err != nil {
		return queuedRun{}, false, err
	}
	if err := json.Unmarshal(tasksJSON, &run.Tasks); err != nil {
		return queuedRun{}, false, err
	}
	_ = json.Unmarshal(secretNamesJSON, &run.SecretNames)
	return run, true, nil
}

func (s *Server) startNextSession(ctx context.Context, run queuedRun) error {
	sessions, err := s.loadRunSessions(ctx, run.ID)
	if err != nil {
		return err
	}
	next, ok, err := s.nextSession(ctx, run, sessions)
	if err != nil {
		return s.failStartedRun(ctx, run, persistedErrSessionFailed, err)
	}
	if !ok {
		return s.reconcileRunProgress(ctx, run.ID)
	}
	sessionReq := dari.CreateSessionRequest{VersionID: next.VersionID}
	if shouldAttachRuntimeSecrets(run, next) {
		secretJSON, err := s.runtimeSecretsJSON(ctx, run.ID)
		if err != nil {
			return s.failStartedRun(ctx, run, persistedErrRuntimeSecretsLoadFailed, fmt.Errorf("load runtime secrets: %w", err))
		}
		if secretJSON != "" {
			sessionReq.Secrets = map[string]string{managedRuntimeSecretsName: secretJSON}
		}
	}
	session, err := s.dari.CreateSession(ctx, next.AgentID, sessionReq)
	if err != nil {
		return s.failStartedRun(ctx, run, persistedErrSessionCreateFailed, fmt.Errorf("create %s session: %w", next.Kind, err))
	}
	_, err = s.db.Exec(ctx, `
INSERT INTO run_sessions (session_id, run_id, kind, task_index, status, version_id)
VALUES ($1, $2, $3, $4, $5, $6)
`, session.ID, run.ID, next.Kind, next.TaskIndex, statusStarting, next.VersionID)
	if err != nil {
		return err
	}
	if _, err := s.dari.SendUserMessage(ctx, session.ID, []dari.ContentBlock{dari.TextBlock(next.Prompt), dari.FileBlock(run.BundleFileID)}); err != nil {
		_, _ = s.db.Exec(ctx, `
UPDATE run_sessions
SET status=$2, completed_at=now(), last_poll_error=$3
WHERE session_id=$1 AND status=$4
`, session.ID, statusFailed, persistedErrorString(persistedErrSessionMessageFailed), statusStarting)
		return s.failStartedRun(ctx, run, persistedErrSessionMessageFailed, fmt.Errorf("send %s session %s message: %w", next.Kind, session.ID, err))
	}
	tag, err := s.db.Exec(ctx, `
UPDATE run_sessions
SET status=$2, last_polled_at=NULL, last_poll_error_at=NULL, last_poll_error=NULL
WHERE session_id=$1 AND status=$3
`, session.ID, statusRunning, statusStarting)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return s.reconcileRunProgress(ctx, run.ID)
	}
	if isFinalSecretBearingSession(run, next) {
		s.clearRuntimeSecrets(ctx, run.ID)
	}
	_, err = s.db.Exec(ctx, `UPDATE runs SET status=$2, updated_at=now() WHERE id=$1 AND status=$3`, run.ID, statusRunning, statusStarting)
	return err
}

func (s *Server) nextSession(ctx context.Context, run queuedRun, sessions []runSessionRecord) (nextSession, bool, error) {
	for _, session := range sessions {
		if session.Status == statusRunning || session.Status == statusStarting {
			return nextSession{}, false, nil
		}
	}
	for _, session := range sessions {
		if session.Status == statusFailed {
			return nextSession{}, false, fmt.Errorf("%s session %s failed", session.Kind, session.ID)
		}
	}
	completedTesters := map[int]bool{}
	for _, session := range sessions {
		if session.Kind == "tester" && session.Status == statusCompleted {
			completedTesters[session.TaskIndex] = true
		}
	}
	if len(completedTesters) < len(run.Tasks) {
		taskIndex := len(completedTesters) + 1
		b := bundle.Result{SHA256: run.BundleSHA256, Manifest: bundle.Manifest{Files: make([]bundle.FileRecord, run.BundleFiles)}}
		return nextSession{
			Kind:      "tester",
			TaskIndex: taskIndex,
			AgentID:   run.TesterAgentID,
			VersionID: run.TesterVersionID,
			Prompt:    runner.FeedbackPrompt(run.Tasks[taskIndex-1], b, run.LiveVerify, secretNameMap(run.SecretNames)),
		}, true, nil
	}
	if run.Mode != "optimize" {
		return nextSession{}, false, nil
	}
	for _, session := range sessions {
		if session.Kind == "editor" {
			return nextSession{}, false, nil
		}
	}
	reports, ready, err := s.collectTesterReports(ctx, run, sessions)
	if err != nil {
		return nextSession{}, false, err
	}
	if !ready {
		return nextSession{}, false, nil
	}
	return nextSession{
		Kind:      "editor",
		TaskIndex: 0,
		AgentID:   run.EditorAgentID,
		VersionID: run.EditorVersionID,
		Prompt:    runner.EditorPrompt(reports),
	}, true, nil
}

func (s *Server) failStartedRun(ctx context.Context, run queuedRun, code persistedErrorCode, cause error) error {
	if err := s.finishRun(ctx, run, "", code); err != nil {
		return err
	}
	return cause
}

func (s *Server) recoverStaleStartingRuns(ctx context.Context) error {
	_, err := s.db.Exec(ctx, `
UPDATE runs
SET status=$1, updated_at=now()
WHERE status=$2 AND updated_at < now() - interval '2 minutes'
  AND NOT EXISTS (
    SELECT 1 FROM run_sessions
    WHERE run_sessions.run_id = runs.id AND run_sessions.status IN ($3, $4)
  )
	`, statusQueued, statusStarting, statusStarting, statusRunning)
	return err
}

func (s *Server) recoverStaleStartingSessions(ctx context.Context) error {
	if s.cfg.SessionStartStaleAfter <= 0 {
		return nil
	}
	rows, err := s.db.Query(ctx, `
SELECT session_id, run_id, kind, task_index, status, created_at, last_poll_error_at, coalesce(last_poll_error,'')
FROM run_sessions
WHERE status=$1 AND created_at < $2
ORDER BY created_at
LIMIT 50
`, statusStarting, time.Now().Add(-s.cfg.SessionStartStaleAfter))
	if err != nil {
		return err
	}
	defer rows.Close()
	var sessions []runSessionRecord
	for rows.Next() {
		var session runSessionRecord
		if err := rows.Scan(&session.ID, &session.RunID, &session.Kind, &session.TaskIndex, &session.Status, &session.CreatedAt, &session.LastPollErrorAt, &session.LastPollError); err != nil {
			return err
		}
		sessions = append(sessions, session)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for _, session := range sessions {
		if err := s.recoverStartingSession(ctx, session); err != nil {
			log.Printf("recover starting session %s: %v", session.ID, err)
		}
	}
	return nil
}

func (s *Server) recoverStartingSession(ctx context.Context, session runSessionRecord) error {
	remote, err := s.dari.GetSession(ctx, session.ID)
	if err != nil {
		stale, recordErr := s.recordSessionPollError(ctx, session, err)
		if recordErr != nil {
			return recordErr
		}
		if !stale {
			return nil
		}
		return s.failRunSession(ctx, session, persistedErrSessionPollStale)
	}
	if sessionHasMessageActivity(remote) {
		tag, err := s.db.Exec(ctx, `
UPDATE run_sessions
SET status=$2, last_polled_at=now(), last_poll_error_at=NULL, last_poll_error=NULL
WHERE session_id=$1 AND status=$3
`, session.ID, statusRunning, statusStarting)
		if err != nil {
			return err
		}
		if tag.RowsAffected() > 0 {
			return s.reconcileRunProgress(ctx, session.RunID)
		}
		return nil
	}
	return s.failRunSession(ctx, session, persistedErrSessionMessageFailed)
}

func sessionHasMessageActivity(session dari.Session) bool {
	return session.LastMessageID != nil || session.LastMessageStatus != nil
}

func (s *Server) recoverStaleUploadingRuns(ctx context.Context) error {
	rows, err := s.db.Query(ctx, `
UPDATE runs
SET status=$1, error=$3, updated_at=now(), completed_at=now()
WHERE status=$2 AND updated_at < now() - interval '10 minutes'
RETURNING id, reserved_cents
`, statusFailed, statusUploading, persistedErrorString(persistedErrBundleUploadIncomplete))
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var runID string
		var reserve int64
		if err := rows.Scan(&runID, &reserve); err != nil {
			return err
		}
		s.clearRuntimeSecrets(ctx, runID)
		s.releaseReservation(ctx, runID, reserve)
	}
	return rows.Err()
}

func (s *Server) reconcileRunningSessions(ctx context.Context) error {
	rows, err := s.db.Query(ctx, `
SELECT session_id, run_id, kind, task_index, status, created_at, last_poll_error_at
FROM run_sessions
WHERE status=$1
ORDER BY created_at
LIMIT 50
`, statusRunning)
	if err != nil {
		return err
	}
	defer rows.Close()
	var sessions []runSessionRecord
	for rows.Next() {
		var session runSessionRecord
		if err := rows.Scan(&session.ID, &session.RunID, &session.Kind, &session.TaskIndex, &session.Status, &session.CreatedAt, &session.LastPollErrorAt); err != nil {
			return err
		}
		sessions = append(sessions, session)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for _, session := range sessions {
		if err := s.reconcileSession(ctx, session); err != nil {
			log.Printf("reconcile session %s: %v", session.ID, err)
		}
	}
	return nil
}

func (s *Server) reconcileSession(ctx context.Context, session runSessionRecord) error {
	remote, err := s.dari.GetSession(ctx, session.ID)
	if err != nil {
		stale, recordErr := s.recordSessionPollError(ctx, session, err)
		if recordErr != nil {
			return recordErr
		}
		if stale {
			return s.failRunSession(ctx, session, persistedErrSessionPollStale)
		}
		return nil
	}
	lastStatus := ""
	if remote.LastMessageStatus != nil {
		lastStatus = *remote.LastMessageStatus
	}
	switch lastStatus {
	case "completed":
		_, err = s.db.Exec(ctx, `
UPDATE run_sessions
SET status=$2, completed_at=now(), last_polled_at=now(), last_poll_error_at=NULL, last_poll_error=NULL
WHERE session_id=$1 AND status=$3
`, session.ID, statusCompleted, statusRunning)
		if err != nil {
			return err
		}
		return s.reconcileRunProgress(ctx, session.RunID)
	case "failed":
		_, err = s.db.Exec(ctx, `
UPDATE run_sessions
SET status=$2, completed_at=now(), last_polled_at=now(), last_poll_error_at=NULL, last_poll_error=$4
WHERE session_id=$1 AND status=$3
`, session.ID, statusFailed, statusRunning, persistedErrorString(persistedErrSessionFailed))
		if err != nil {
			return err
		}
		return s.reconcileRunProgress(ctx, session.RunID)
	default:
		if s.cfg.SessionStaleAfter > 0 && time.Since(session.CreatedAt) > s.cfg.SessionStaleAfter {
			return s.failRunSession(ctx, session, persistedErrSessionStale)
		}
		_, err = s.db.Exec(ctx, `
UPDATE run_sessions
SET last_polled_at=now(), last_poll_error_at=NULL, last_poll_error=NULL
WHERE session_id=$1 AND status=$2
`, session.ID, statusRunning)
		return err
	}
}

func (s *Server) recordSessionPollError(ctx context.Context, session runSessionRecord, _ error) (bool, error) {
	firstErrorAt := session.LastPollErrorAt
	if firstErrorAt == nil {
		now := time.Now()
		firstErrorAt = &now
		_, err := s.db.Exec(ctx, `
UPDATE run_sessions
SET last_polled_at=now(), last_poll_error_at=coalesce(last_poll_error_at, now()), last_poll_error=$2
WHERE session_id=$1 AND status=$3
`, session.ID, persistedErrorString(persistedErrSessionPollFailed), session.Status)
		if err != nil {
			return false, err
		}
	} else {
		_, err := s.db.Exec(ctx, `
UPDATE run_sessions
SET last_polled_at=now(), last_poll_error=$2
WHERE session_id=$1 AND status=$3
`, session.ID, persistedErrorString(persistedErrSessionPollFailed), session.Status)
		if err != nil {
			return false, err
		}
	}
	return s.cfg.PollErrorStaleAfter > 0 && time.Since(*firstErrorAt) > s.cfg.PollErrorStaleAfter, nil
}

func (s *Server) failRunSession(ctx context.Context, session runSessionRecord, code persistedErrorCode) error {
	tag, err := s.db.Exec(ctx, `
UPDATE run_sessions
SET status=$2, completed_at=now(), last_polled_at=now(), last_poll_error=$3
WHERE session_id=$1 AND status=$4
`, session.ID, statusFailed, persistedErrorString(code), session.Status)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return nil
	}
	return s.reconcileRunProgress(ctx, session.RunID)
}

func runErrorCodeFromSession(session runSessionRecord) persistedErrorCode {
	return persistedErrorCodeFromString(session.LastPollError, persistedErrSessionFailed)
}

func (s *Server) reconcileRunProgress(ctx context.Context, runID string) error {
	run, err := s.loadRun(ctx, runID)
	if err != nil {
		return err
	}
	if run.ID == "" {
		return nil
	}
	sessions, err := s.loadRunSessions(ctx, run.ID)
	if err != nil {
		return err
	}
	for _, session := range sessions {
		if session.Status == statusRunning || session.Status == statusStarting {
			_, err := s.db.Exec(ctx, `UPDATE runs SET status=$2, updated_at=now() WHERE id=$1 AND status IN ($3,$4)`, run.ID, statusRunning, statusQueued, statusStarting)
			return err
		}
	}
	for _, session := range sessions {
		if session.Status == statusFailed {
			return s.finishRun(ctx, run, "", runErrorCodeFromSession(session))
		}
	}
	_, reportsReady, err := s.collectTesterReports(ctx, run, sessions)
	if err != nil {
		return err
	}
	if !reportsReady {
		_, err := s.db.Exec(ctx, `UPDATE runs SET status=$2, updated_at=now() WHERE id=$1 AND status IN ($3,$4)`, run.ID, statusQueued, statusRunning, statusStarting)
		return err
	}
	if run.Mode == "check" {
		return s.finishRun(ctx, run, "", "")
	}
	var editor *runSessionRecord
	for i := range sessions {
		if sessions[i].Kind == "editor" {
			editor = &sessions[i]
			break
		}
	}
	if editor == nil {
		_, err := s.db.Exec(ctx, `UPDATE runs SET status=$2, updated_at=now() WHERE id=$1 AND status IN ($3,$4)`, run.ID, statusQueued, statusRunning, statusStarting)
		return err
	}
	if editor.Status == statusFailed {
		return s.finishRun(ctx, run, "", runErrorCodeFromSession(*editor))
	}
	if editor.Status != statusCompleted {
		return nil
	}
	return s.finishRun(ctx, run, editor.ID, "")
}

func (s *Server) loadRun(ctx context.Context, runID string) (queuedRun, error) {
	var run queuedRun
	var tasksJSON, secretNamesJSON []byte
	err := s.db.QueryRow(ctx, `
SELECT r.id, r.user_id, r.mode, r.tasks,
  r.tester_agent_id,
  r.tester_version_id,
  r.editor_agent_id,
  r.editor_version_id,
  coalesce(r.bundle_file_id,''), r.bundle_sha256, r.bundle_files, r.live_verify, r.runtime_secret_names, r.reserved_cents
FROM runs r
WHERE r.id=$1 AND r.status IN ($2,$3,$4)
	`, runID, statusQueued, statusStarting, statusRunning).Scan(
		&run.ID, &run.UserID, &run.Mode, &tasksJSON,
		&run.TesterAgentID, &run.TesterVersionID, &run.EditorAgentID, &run.EditorVersionID,
		&run.BundleFileID, &run.BundleSHA256, &run.BundleFiles, &run.LiveVerify, &secretNamesJSON, &run.ReservedCents,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return queuedRun{}, nil
	}
	if err != nil {
		return queuedRun{}, err
	}
	if err := json.Unmarshal(tasksJSON, &run.Tasks); err != nil {
		return queuedRun{}, err
	}
	_ = json.Unmarshal(secretNamesJSON, &run.SecretNames)
	return run, nil
}

func (s *Server) finishRun(ctx context.Context, run queuedRun, editorSessionID string, failureCode persistedErrorCode) error {
	var (
		tag pgconn.CommandTag
		err error
	)
	if failureCode != "" {
		tag, err = s.db.Exec(ctx, `
UPDATE runs SET status=$2, error=$3, updated_at=now(), completed_at=now()
WHERE id=$1 AND status IN ($4,$5,$6)
`, run.ID, statusFailed, persistedErrorString(failureCode), statusQueued, statusStarting, statusRunning)
	} else {
		tag, err = s.db.Exec(ctx, `
UPDATE runs SET status=$2, error=NULL, editor_session_id=$3, updated_at=now(), completed_at=now()
WHERE id=$1 AND status IN ($4,$5,$6)
`, run.ID, statusCompleted, editorSessionID, statusQueued, statusStarting, statusRunning)
	}
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return nil
	}
	s.clearRuntimeSecrets(ctx, run.ID)
	return s.settleRun(ctx, run.ID, run.ReservedCents)
}

func (s *Server) settleUnsettledRuns(ctx context.Context) error {
	rows, err := s.db.Query(ctx, `
	SELECT id, reserved_cents
	FROM runs
	WHERE status IN ($1,$2) AND (cost_status IS NULL OR cost_status='estimated')
	ORDER BY completed_at NULLS FIRST, updated_at
	LIMIT 10
	`, statusCompleted, statusFailed)
	if err != nil {
		return err
	}
	defer rows.Close()
	type unsettledRun struct {
		ID            string
		ReservedCents int64
	}
	var runs []unsettledRun
	for rows.Next() {
		var run unsettledRun
		if err := rows.Scan(&run.ID, &run.ReservedCents); err != nil {
			return err
		}
		runs = append(runs, run)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for _, run := range runs {
		if err := s.settleRun(ctx, run.ID, run.ReservedCents); err != nil {
			log.Printf("settle run %s: %v", run.ID, err)
		}
	}
	return nil
}

func (s *Server) loadRunSessions(ctx context.Context, runID string) ([]runSessionRecord, error) {
	rows, err := s.db.Query(ctx, `
SELECT session_id, run_id, kind, task_index, status, created_at, last_poll_error_at, coalesce(last_poll_error,'')
FROM run_sessions
WHERE run_id=$1
ORDER BY created_at
`, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var sessions []runSessionRecord
	for rows.Next() {
		var session runSessionRecord
		if err := rows.Scan(&session.ID, &session.RunID, &session.Kind, &session.TaskIndex, &session.Status, &session.CreatedAt, &session.LastPollErrorAt, &session.LastPollError); err != nil {
			return nil, err
		}
		sessions = append(sessions, session)
	}
	return sessions, rows.Err()
}

func (s *Server) collectTesterReports(ctx context.Context, run queuedRun, sessions []runSessionRecord) ([]string, bool, error) {
	reports := make([]string, len(run.Tasks))
	seen := make([]bool, len(run.Tasks))
	for _, session := range sessions {
		if session.Kind != "tester" || session.Status != statusCompleted || session.TaskIndex < 1 || session.TaskIndex > len(run.Tasks) {
			continue
		}
		tr, err := s.dari.GetTranscript(ctx, session.ID)
		if err != nil {
			return nil, false, fmt.Errorf("get transcript %s: %w", session.ID, err)
		}
		reports[session.TaskIndex-1] = dari.FinalAssistantText(tr)
		seen[session.TaskIndex-1] = true
	}
	for _, ok := range seen {
		if !ok {
			return reports, false, nil
		}
	}
	return reports, true, nil
}

func (s *Server) settleRun(ctx context.Context, runID string, reserve int64) error {
	rows, err := s.db.Query(ctx, `SELECT session_id FROM run_sessions WHERE run_id=$1`, runID)
	if err != nil {
		return err
	}
	defer rows.Close()
	var sessionIDs []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return err
		}
		sessionIDs = append(sessionIDs, id)
	}
	charges := map[string]int64{}
	deadline := time.Now().Add(s.cfg.CostFetchTimeout)
	for len(charges) < len(sessionIDs) && time.Now().Before(deadline) {
		for _, id := range sessionIDs {
			if _, ok := charges[id]; ok {
				continue
			}
			cost, err := s.dari.GetSessionCost(ctx, id)
			if err != nil {
				continue
			}
			costCents := usdStringToCentsCeil(cost.TotalCostUSD)
			charges[id] = costCents + s.cfg.ServiceFeeCents
			_, _ = s.db.Exec(ctx, `UPDATE run_sessions SET cost_cents=$2, charge_cents=$3 WHERE session_id=$1`, id, costCents, charges[id])
		}
		if len(charges) == len(sessionIDs) || s.cfg.CostFetchTimeout == 0 {
			break
		}
		time.Sleep(5 * time.Second)
	}
	totalCharge := int64(0)
	costStatus := "actual"
	if len(charges) < len(sessionIDs) {
		totalCharge = reserve
		costStatus = "estimated"
	} else {
		for _, c := range charges {
			totalCharge += c
		}
	}
	var delta int64
	kind := "run_reservation_release"
	source := "release:" + runID
	if reserve > totalCharge {
		delta = reserve - totalCharge
	} else if totalCharge > reserve {
		delta = -(totalCharge - reserve)
		kind = "run_overage"
		source = "overage:" + runID
	}
	if delta != 0 {
		_, err = s.db.Exec(ctx, `
INSERT INTO credit_ledger (id, user_id, amount_cents, kind, source_id, run_id)
SELECT $1, user_id, $2, $3, $4, id FROM runs WHERE id=$5
ON CONFLICT (source_id) DO NOTHING
`, "led_"+randomToken(18), delta, kind, source, runID)
		if err != nil {
			return err
		}
	}
	_, err = s.db.Exec(ctx, `UPDATE runs SET charged_cents=$2, cost_status=$3 WHERE id=$1`, runID, totalCharge, costStatus)
	return err
}
