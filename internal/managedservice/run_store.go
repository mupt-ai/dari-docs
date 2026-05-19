package managedservice

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

type managedRunDB interface {
	BeginTx(context.Context, pgx.TxOptions) (pgx.Tx, error)
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}

type managedRunStore struct {
	db managedRunDB
}

func newManagedRunStore(db *pgxpool.Pool) *managedRunStore {
	return &managedRunStore{db: db}
}

func (s *Server) runs() (*managedRunStore, error) {
	if s.runStore != nil {
		return s.runStore, nil
	}
	if s.db == nil {
		return nil, errors.New("managed run store is not configured")
	}
	s.runStore = newManagedRunStore(s.db)
	return s.runStore, nil
}

func (store *managedRunStore) ClaimStartableRun(ctx context.Context) (queuedRun, bool, error) {
	tx, err := store.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return queuedRun{}, false, err
	}
	defer tx.Rollback(ctx)

	run, err := scanQueuedRun(tx.QueryRow(ctx, `
SELECT id,
       user_id,
       mode,
       tasks,
       tester_agent_id,
       editor_agent_id,
       tester_llm_ids,
       editor_llm_id,
       COALESCE(bundle_file_id, ''),
       bundle_sha256,
       bundle_files,
       live_verify,
       runtime_secret_names,
       reserved_cents
FROM runs
WHERE status=$1
ORDER BY created_at
FOR UPDATE SKIP LOCKED
LIMIT 1
`, statusQueued))
	if errors.Is(err, pgx.ErrNoRows) {
		return queuedRun{}, false, nil
	}
	if err != nil {
		return queuedRun{}, false, err
	}
	if _, err := tx.Exec(ctx, `
UPDATE runs
SET status=$1,
    updated_at=now()
WHERE id=$2
`, statusStarting, run.ID); err != nil {
		return queuedRun{}, false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return queuedRun{}, false, err
	}
	return run, true, nil
}

func (store *managedRunStore) InsertStartedRunSession(
	ctx context.Context,
	sessionID string,
	runID string,
	kind string,
	taskIndex int,
	llmID string,
) error {
	_, err := store.db.Exec(ctx, `
INSERT INTO run_sessions (session_id, run_id, kind, task_index, status, version_id, llm_id)
VALUES ($1, $2, $3, $4, $5, $6, NULLIF($7, ''))
ON CONFLICT (session_id) DO NOTHING
`, sessionID, runID, kind, taskIndex, statusRunning, managedAgentVersionCompatibilityValue, llmID)
	return err
}

func (store *managedRunStore) MarkRunRunningFromStarting(ctx context.Context, runID string) error {
	_, err := store.db.Exec(ctx, `
UPDATE runs
SET status=$1,
    updated_at=now()
WHERE id=$2
  AND status=$3
`, statusRunning, runID, statusStarting)
	return err
}

func (store *managedRunStore) RecoverStaleStartingRuns(ctx context.Context, staleBefore time.Time) error {
	_, err := store.db.Exec(ctx, `
UPDATE runs
SET status=$1,
    updated_at=now()
WHERE status=$2
  AND updated_at < $3
  AND NOT EXISTS (
    SELECT 1
    FROM run_sessions
    WHERE run_sessions.run_id = runs.id
      AND run_sessions.status = ANY($4)
  )
`, statusQueued, statusStarting, staleBefore, []string{statusStarting, statusRunning})
	return err
}

func (store *managedRunStore) RecoverStaleUploadingRuns(
	ctx context.Context,
	staleBefore time.Time,
	code persistedErrorCode,
) ([]queuedRun, error) {
	rows, err := store.db.Query(ctx, `
UPDATE runs
SET status=$1,
    error=$2,
    updated_at=now(),
    completed_at=now()
WHERE status=$3
  AND updated_at < $4
RETURNING id, reserved_cents
`, statusFailed, persistedErrorString(code), statusUploading, staleBefore)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	runs := []queuedRun{}
	for rows.Next() {
		var run queuedRun
		if err := rows.Scan(&run.ID, &run.ReservedCents); err != nil {
			return nil, err
		}
		runs = append(runs, run)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return runs, nil
}

func (store *managedRunStore) ListRunningSessions(ctx context.Context, limit int) ([]runSessionRecord, error) {
	rows, err := store.db.Query(ctx, `
SELECT session_id,
       run_id,
       kind,
       task_index,
       status,
       COALESCE(llm_id, ''),
       created_at,
       last_poll_error_at,
       COALESCE(last_poll_error, '')
FROM run_sessions
WHERE status=$1
ORDER BY created_at
LIMIT $2
`, statusRunning, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanRunSessionRecords(rows)
}

func (store *managedRunStore) MarkSessionCompleted(ctx context.Context, sessionID string, llmID string) error {
	_, err := store.db.Exec(ctx, `
UPDATE run_sessions
SET status=$1,
    completed_at=now(),
    last_polled_at=now(),
    last_poll_error_at=NULL,
    last_poll_error=NULL,
    llm_id=COALESCE(NULLIF($2, ''), llm_id)
WHERE session_id=$3
  AND status=$4
`, statusCompleted, llmID, sessionID, statusRunning)
	return err
}

func (store *managedRunStore) MarkSessionFailed(ctx context.Context, sessionID string, code persistedErrorCode, llmID string) error {
	_, err := store.db.Exec(ctx, `
UPDATE run_sessions
SET status=$1,
    completed_at=now(),
    last_polled_at=now(),
    last_poll_error_at=NULL,
    last_poll_error=$2,
    llm_id=COALESCE(NULLIF($3, ''), llm_id)
WHERE session_id=$4
  AND status=$5
`, statusFailed, persistedErrorString(code), llmID, sessionID, statusRunning)
	return err
}

func (store *managedRunStore) MarkSessionPollSucceeded(ctx context.Context, sessionID string, llmID string) error {
	_, err := store.db.Exec(ctx, `
UPDATE run_sessions
SET last_polled_at=now(),
    last_poll_error_at=NULL,
    last_poll_error=NULL,
    llm_id=COALESCE(NULLIF($1, ''), llm_id)
WHERE session_id=$2
  AND status=$3
`, llmID, sessionID, statusRunning)
	return err
}

func (store *managedRunStore) RecordSessionPollError(
	ctx context.Context,
	session runSessionRecord,
	code persistedErrorCode,
) (time.Time, error) {
	firstErrorAt := session.LastPollErrorAt
	if firstErrorAt == nil {
		now := time.Now()
		firstErrorAt = &now
		_, err := store.db.Exec(ctx, `
UPDATE run_sessions
SET last_polled_at=now(),
    last_poll_error=$1,
    last_poll_error_at=$2
WHERE session_id=$3
  AND status=$4
`, persistedErrorString(code), now, session.ID, session.Status)
		return *firstErrorAt, err
	}

	_, err := store.db.Exec(ctx, `
UPDATE run_sessions
SET last_polled_at=now(),
    last_poll_error=$1
WHERE session_id=$2
  AND status=$3
`, persistedErrorString(code), session.ID, session.Status)
	return *firstErrorAt, err
}

func (store *managedRunStore) FailRunSession(ctx context.Context, session runSessionRecord, code persistedErrorCode) (bool, error) {
	commandTag, err := store.db.Exec(ctx, `
UPDATE run_sessions
SET status=$1,
    completed_at=now(),
    last_polled_at=now(),
    last_poll_error=$2
WHERE session_id=$3
  AND status=$4
`, statusFailed, persistedErrorString(code), session.ID, session.Status)
	return commandTag.RowsAffected() > 0, err
}

func (store *managedRunStore) MarkRunRunningIfStartable(ctx context.Context, runID string) error {
	_, err := store.db.Exec(ctx, `
UPDATE runs
SET status=$1,
    updated_at=now()
WHERE id=$2
  AND status = ANY($3)
`, statusRunning, runID, []string{statusQueued, statusStarting})
	return err
}

func (store *managedRunStore) MarkRunQueuedIfActive(ctx context.Context, runID string) error {
	_, err := store.db.Exec(ctx, `
UPDATE runs
SET status=$1,
    updated_at=now()
WHERE id=$2
  AND status = ANY($3)
`, statusQueued, runID, []string{statusQueued, statusRunning, statusStarting})
	return err
}

func (store *managedRunStore) LoadActiveRun(ctx context.Context, runID string) (queuedRun, error) {
	run, err := scanQueuedRun(store.db.QueryRow(ctx, `
SELECT id,
       user_id,
       mode,
       tasks,
       tester_agent_id,
       editor_agent_id,
       tester_llm_ids,
       editor_llm_id,
       COALESCE(bundle_file_id, ''),
       bundle_sha256,
       bundle_files,
       live_verify,
       runtime_secret_names,
       reserved_cents
FROM runs
WHERE id=$1
  AND status = ANY($2)
`, runID, []string{statusQueued, statusStarting, statusRunning}))
	if errors.Is(err, pgx.ErrNoRows) {
		return queuedRun{}, nil
	}
	return run, err
}

func (store *managedRunStore) FinishRun(
	ctx context.Context,
	run queuedRun,
	editorSessionID string,
	failureCode persistedErrorCode,
) (bool, error) {
	activeStatuses := []string{statusQueued, statusStarting, statusRunning}
	if failureCode != "" {
		commandTag, err := store.db.Exec(ctx, `
UPDATE runs
SET status=$1,
    error=$2,
    updated_at=now(),
    completed_at=now()
WHERE id=$3
  AND status = ANY($4)
`, statusFailed, persistedErrorString(failureCode), run.ID, activeStatuses)
		return commandTag.RowsAffected() > 0, err
	}
	commandTag, err := store.db.Exec(ctx, `
UPDATE runs
SET status=$1,
    error=NULL,
    editor_session_id=$2,
    updated_at=now(),
    completed_at=now()
WHERE id=$3
  AND status = ANY($4)
`, statusCompleted, editorSessionID, run.ID, activeStatuses)
	return commandTag.RowsAffected() > 0, err
}

func (store *managedRunStore) ListUnsettledRuns(ctx context.Context, limit int) ([]queuedRun, error) {
	rows, err := store.db.Query(ctx, `
SELECT id, reserved_cents
FROM runs
WHERE status = ANY($1)
  AND (cost_status IS NULL OR cost_status=$2)
ORDER BY completed_at NULLS FIRST, updated_at
LIMIT $3
`, []string{statusCompleted, statusFailed}, "estimated", limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	runs := []queuedRun{}
	for rows.Next() {
		var run queuedRun
		if err := rows.Scan(&run.ID, &run.ReservedCents); err != nil {
			return nil, err
		}
		runs = append(runs, run)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return runs, nil
}

func (store *managedRunStore) LoadRunSessions(ctx context.Context, runID string) ([]runSessionRecord, error) {
	rows, err := store.db.Query(ctx, `
SELECT session_id,
       run_id,
       kind,
       task_index,
       status,
       COALESCE(llm_id, ''),
       created_at,
       last_poll_error_at,
       COALESCE(last_poll_error, '')
FROM run_sessions
WHERE run_id=$1
ORDER BY CASE kind WHEN 'tester' THEN 1 WHEN 'editor' THEN 2 ELSE 3 END, task_index, llm_id, created_at
`, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanRunSessionRecords(rows)
}

func (store *managedRunStore) ListRunSessionIDs(ctx context.Context, runID string) ([]string, error) {
	rows, err := store.db.Query(ctx, `
SELECT session_id
FROM run_sessions
WHERE run_id=$1
`, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	ids := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return ids, nil
}

func (store *managedRunStore) UpdateSessionCost(ctx context.Context, sessionID string, costCents, chargeCents int64) error {
	_, err := store.db.Exec(ctx, `
UPDATE run_sessions
SET cost_cents=$1,
    charge_cents=$2
WHERE session_id=$3
`, costCents, chargeCents, sessionID)
	return err
}

func (store *managedRunStore) InsertRunLedgerAdjustment(
	ctx context.Context,
	runID string,
	amountCents int64,
	kind string,
	sourceID string,
) error {
	var userID string
	if err := store.db.QueryRow(ctx, `
SELECT user_id
FROM runs
WHERE id=$1
`, runID).Scan(&userID); err != nil {
		return err
	}
	_, err := store.db.Exec(ctx, `
INSERT INTO credit_ledger (id, user_id, amount_cents, kind, source_id, run_id)
VALUES ($1, $2, $3, $4, $5, $6)
ON CONFLICT (source_id) DO NOTHING
`, "led_"+randomToken(18), userID, amountCents, kind, sourceID, runID)
	return err
}

func (store *managedRunStore) MarkRunSettled(ctx context.Context, runID string, totalCharge int64, costStatus string) error {
	_, err := store.db.Exec(ctx, `
UPDATE runs
SET charged_cents=$1,
    cost_status=$2
WHERE id=$3
`, totalCharge, costStatus, runID)
	return err
}

func scanQueuedRun(row pgx.Row) (queuedRun, error) {
	var run queuedRun
	var tasksJSON []byte
	var testerLLMIDsJSON []byte
	var secretNamesJSON []byte
	if err := row.Scan(
		&run.ID,
		&run.UserID,
		&run.Mode,
		&tasksJSON,
		&run.TesterAgentID,
		&run.EditorAgentID,
		&testerLLMIDsJSON,
		&run.EditorLLMID,
		&run.BundleFileID,
		&run.BundleSHA256,
		&run.BundleFiles,
		&run.LiveVerify,
		&secretNamesJSON,
		&run.ReservedCents,
	); err != nil {
		return queuedRun{}, err
	}
	if len(tasksJSON) > 0 {
		if err := json.Unmarshal(tasksJSON, &run.Tasks); err != nil {
			return queuedRun{}, fmt.Errorf("decode run tasks: %w", err)
		}
	}
	if len(testerLLMIDsJSON) > 0 {
		if err := json.Unmarshal(testerLLMIDsJSON, &run.TesterLLMIDs); err != nil {
			return queuedRun{}, fmt.Errorf("decode tester LLM IDs: %w", err)
		}
	}
	var err error
	run.TesterLLMIDs, err = normalizeManagedLLMIDs(run.TesterLLMIDs)
	if err != nil {
		return queuedRun{}, fmt.Errorf("decode tester LLM IDs: %w", err)
	}
	run.EditorLLMID, err = normalizeManagedLLMID(run.EditorLLMID)
	if err != nil {
		return queuedRun{}, fmt.Errorf("decode editor LLM ID: %w", err)
	}
	if len(secretNamesJSON) > 0 {
		if err := json.Unmarshal(secretNamesJSON, &run.SecretNames); err != nil {
			return queuedRun{}, fmt.Errorf("decode runtime secret names: %w", err)
		}
	}
	return run, nil
}

func scanRunSessionRecords(rows pgx.Rows) ([]runSessionRecord, error) {
	records := []runSessionRecord{}
	for rows.Next() {
		var record runSessionRecord
		var lastPollErrorAt pgtype.Timestamptz
		if err := rows.Scan(
			&record.ID,
			&record.RunID,
			&record.Kind,
			&record.TaskIndex,
			&record.Status,
			&record.LLMID,
			&record.CreatedAt,
			&lastPollErrorAt,
			&record.LastPollError,
		); err != nil {
			return nil, err
		}
		if lastPollErrorAt.Valid {
			record.LastPollErrorAt = &lastPollErrorAt.Time
		}
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return records, nil
}
