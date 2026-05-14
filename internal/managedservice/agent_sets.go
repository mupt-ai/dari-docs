package managedservice

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/mupt-ai/dari-docs/internal/agentbundle"
	"github.com/mupt-ai/dari-docs/internal/dari"
)

type agentSetDeployResponse struct {
	ID              string `json:"id"`
	DeployID        string `json:"deploy_id"`
	AgentSetID      string `json:"agent_set_id"`
	Status          string `json:"status"`
	Step            string `json:"step,omitempty"`
	TesterAgentID   string `json:"tester_agent_id,omitempty"`
	TesterVersionID string `json:"tester_version_id,omitempty"`
	EditorAgentID   string `json:"editor_agent_id,omitempty"`
	EditorVersionID string `json:"editor_version_id,omitempty"`
	Applied         bool   `json:"applied"`
	Error           string `json:"error,omitempty"`
}

type agentSetDeploy struct {
	ID                     string
	UserID                 string
	AgentSetID             string
	TargetTesterAgentID    string
	TargetEditorAgentID    string
	TesterSourceSnapshotID string
	EditorSourceSnapshotID string
	TesterSHA256           string
	EditorSHA256           string
	Sequence               int64
}

func (s *Server) handleAgentSets(w http.ResponseWriter, r *http.Request, u user) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if !requireScope(w, u, scopeManagedAgentsDeploy) {
		return
	}
	maxBytes := 2*managedMaxAgentBundleBytes + 1<<20
	if r.ContentLength > maxBytes {
		writeError(w, http.StatusRequestEntityTooLarge, "agent bundle exceeds managed size limit")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
	if err := r.ParseMultipartForm(maxBytes); err != nil {
		if isRequestBodyTooLarge(err) {
			writeError(w, http.StatusRequestEntityTooLarge, "agent bundle exceeds managed size limit")
			return
		}
		writeLoggedError(w, http.StatusBadRequest, "invalid multipart form", err)
		return
	}
	deployRequestID := strings.TrimSpace(r.FormValue("deploy_request_id"))
	if deployRequestID == "" {
		writeError(w, http.StatusBadRequest, "deploy_request_id is required")
		return
	}
	if len(deployRequestID) > 128 {
		writeError(w, http.StatusBadRequest, "deploy_request_id is too large")
		return
	}
	if existing, ok, err := s.loadAgentSetDeployByRequest(r.Context(), u.ID, deployRequestID); err != nil {
		writeLoggedError(w, http.StatusInternalServerError, "could not load managed agent deploy", err)
		return
	} else if ok {
		writeJSON(w, http.StatusAccepted, existing)
		return
	}

	testerBytes, err := readUploadedFormFile(r, "tester_bundle", managedMaxAgentBundleBytes)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	editorBytes, err := readUploadedFormFile(r, "editor_bundle", managedMaxAgentBundleBytes)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := agentbundle.ValidateManagedBundle(testerBytes, "tester agent"); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := agentbundle.ValidateManagedBundle(editorBytes, "editor agent"); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	existingID := strings.TrimSpace(r.FormValue("agent_set_id"))
	var targetTester, targetEditor string
	if existingID != "" {
		err := s.db.QueryRow(r.Context(), `
SELECT tester_agent_id, editor_agent_id FROM agent_sets WHERE id=$1 AND user_id=$2
`, existingID, u.ID).Scan(&targetTester, &targetEditor)
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "managed agent set not found or does not belong to this user")
			return
		}
		if err != nil {
			writeLoggedError(w, http.StatusInternalServerError, "could not load managed agent set", err)
			return
		}
	}

	testerBundle := dari.NewSourceBundle(testerBytes)
	editorBundle := dari.NewSourceBundle(editorBytes)
	testerSource, err := s.dari.StageSourceBundle(r.Context(), testerBundle)
	if err != nil {
		writeLoggedError(w, http.StatusBadGateway, "could not stage tester agent", err)
		return
	}
	editorSource, err := s.dari.StageSourceBundle(r.Context(), editorBundle)
	if err != nil {
		_ = s.dari.DeleteSourceSnapshot(context.Background(), testerSource.ID)
		writeLoggedError(w, http.StatusBadGateway, "could not stage editor agent", err)
		return
	}

	agentSetID := existingID
	if agentSetID == "" {
		agentSetID = "mags_" + randomToken(18)
	}
	deployID := "magd_" + randomToken(18)
	inserted := false
	err = s.db.QueryRow(r.Context(), `
INSERT INTO agent_set_deploys (
  id, user_id, deploy_request_id, agent_set_id, target_tester_agent_id, target_editor_agent_id,
  tester_source_snapshot_id, editor_source_snapshot_id, tester_sha256, editor_sha256,
  status, step, heartbeat_at
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, 'queued', now())
ON CONFLICT (user_id, deploy_request_id) DO NOTHING
RETURNING true
`, deployID, u.ID, deployRequestID, agentSetID, targetTester, targetEditor, testerSource.ID, editorSource.ID, testerBundle.SHA256, editorBundle.SHA256, statusQueued).Scan(&inserted)
	if errors.Is(err, pgx.ErrNoRows) {
		_ = s.dari.DeleteSourceSnapshot(context.Background(), testerSource.ID)
		_ = s.dari.DeleteSourceSnapshot(context.Background(), editorSource.ID)
		existing, ok, loadErr := s.loadAgentSetDeployByRequest(r.Context(), u.ID, deployRequestID)
		if loadErr != nil {
			writeLoggedError(w, http.StatusInternalServerError, "could not load managed agent deploy", loadErr)
			return
		}
		if !ok {
			writeError(w, http.StatusConflict, "managed agent deploy request conflicted; retry")
			return
		}
		writeJSON(w, http.StatusAccepted, existing)
		return
	}
	if err != nil {
		_ = s.dari.DeleteSourceSnapshot(context.Background(), testerSource.ID)
		_ = s.dari.DeleteSourceSnapshot(context.Background(), editorSource.ID)
		writeLoggedError(w, http.StatusInternalServerError, "could not queue managed agent deploy", err)
		return
	}
	resp, err := s.loadAgentSetDeploy(r.Context(), u.ID, deployID)
	if err != nil {
		writeLoggedError(w, http.StatusInternalServerError, "could not load managed agent deploy", err)
		return
	}
	writeJSON(w, http.StatusAccepted, resp)
}

func (s *Server) handleAgentSetDeployByID(w http.ResponseWriter, r *http.Request, u user) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if !requireScope(w, u, scopeManagedRead) {
		return
	}
	deployID := strings.Trim(strings.TrimPrefix(r.URL.Path, "/v1/agent-set-deploys/"), "/")
	if deployID == "" {
		writeError(w, http.StatusNotFound, "managed agent deploy not found")
		return
	}
	resp, err := s.loadAgentSetDeploy(r.Context(), u.ID, deployID)
	if err != nil {
		writeError(w, http.StatusNotFound, "managed agent deploy not found")
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) loadAgentSetDeployByRequest(ctx context.Context, userID, deployRequestID string) (agentSetDeployResponse, bool, error) {
	var deployID string
	err := s.db.QueryRow(ctx, `SELECT id FROM agent_set_deploys WHERE user_id=$1 AND deploy_request_id=$2`, userID, deployRequestID).Scan(&deployID)
	if errors.Is(err, pgx.ErrNoRows) {
		return agentSetDeployResponse{}, false, nil
	}
	if err != nil {
		return agentSetDeployResponse{}, false, err
	}
	resp, err := s.loadAgentSetDeploy(ctx, userID, deployID)
	return resp, err == nil, err
}

func (s *Server) loadAgentSetDeploy(ctx context.Context, userID, deployID string) (agentSetDeployResponse, error) {
	var resp agentSetDeployResponse
	err := s.db.QueryRow(ctx, `
SELECT id, agent_set_id, status, coalesce(step,''), coalesce(tester_agent_id,''), coalesce(tester_version_id,''),
  coalesce(editor_agent_id,''), coalesce(editor_version_id,''), applied, coalesce(error,'')
FROM agent_set_deploys
WHERE id=$1 AND user_id=$2
`, deployID, userID).Scan(&resp.DeployID, &resp.AgentSetID, &resp.Status, &resp.Step, &resp.TesterAgentID, &resp.TesterVersionID, &resp.EditorAgentID, &resp.EditorVersionID, &resp.Applied, &resp.Error)
	if err != nil {
		return resp, err
	}
	resp.ID = resp.AgentSetID
	return resp, nil
}

func (s *Server) agentSetDeployLoop(ctx context.Context) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		if err := s.recoverStaleAgentSetDeploys(ctx); err != nil {
			log.Printf("recover stale agent deploys: %v", err)
		}
		if err := s.startQueuedAgentSetDeploys(ctx); err != nil {
			log.Printf("start agent deploys: %v", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (s *Server) startQueuedAgentSetDeploys(ctx context.Context) error {
	for i := 0; i < s.agentDeployClaimBatchSize(); i++ {
		deploy, ok, err := s.claimAgentSetDeploy(ctx)
		if err != nil {
			return err
		}
		if !ok {
			return nil
		}
		go s.processAgentSetDeploy(ctx, deploy)
	}
	return nil
}

func (s *Server) agentDeployClaimBatchSize() int {
	if s.cfg.AgentDeployClaimBatchSize > 0 {
		return s.cfg.AgentDeployClaimBatchSize
	}
	return int(managedAgentDeployClaimBatchSize)
}

func (s *Server) claimAgentSetDeploy(ctx context.Context) (agentSetDeploy, bool, error) {
	var d agentSetDeploy
	err := s.db.QueryRow(ctx, `
UPDATE agent_set_deploys d
SET status=$1, step='starting', heartbeat_at=now(), updated_at=now()
WHERE d.id = (
  SELECT queued.id
  FROM agent_set_deploys queued
  WHERE queued.status=$2
    AND pg_try_advisory_xact_lock(hashtext(queued.agent_set_id)::bigint)
    AND NOT EXISTS (
      SELECT 1 FROM agent_set_deploys active
      WHERE active.agent_set_id=queued.agent_set_id AND active.status=$1
    )
  ORDER BY queued.sequence
  LIMIT 1
  FOR UPDATE SKIP LOCKED
)
RETURNING d.id, d.user_id, d.agent_set_id, coalesce(d.target_tester_agent_id,''), coalesce(d.target_editor_agent_id,''),
  d.tester_source_snapshot_id, d.editor_source_snapshot_id, d.tester_sha256, d.editor_sha256, d.sequence
`, statusRunning, statusQueued).Scan(&d.ID, &d.UserID, &d.AgentSetID, &d.TargetTesterAgentID, &d.TargetEditorAgentID, &d.TesterSourceSnapshotID, &d.EditorSourceSnapshotID, &d.TesterSHA256, &d.EditorSHA256, &d.Sequence)
	if errors.Is(err, pgx.ErrNoRows) {
		return agentSetDeploy{}, false, nil
	}
	if err != nil {
		return agentSetDeploy{}, false, err
	}
	return d, true, nil
}

func (s *Server) processAgentSetDeploy(parent context.Context, d agentSetDeploy) {
	ctx, cancel := context.WithCancel(parent)
	defer cancel()
	go s.heartbeatAgentSetDeploy(ctx, d.ID)
	if err := s.publishAgentSetDeploy(ctx, d); err != nil {
		log.Printf("agent deploy %s failed: %v", d.ID, err)
		if markErr := s.failAgentSetDeploy(context.Background(), d, err); markErr != nil {
			log.Printf("mark agent deploy %s failed: %v", d.ID, markErr)
		}
	}
}

func (s *Server) heartbeatAgentSetDeploy(ctx context.Context, deployID string) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			_, _ = s.db.Exec(context.Background(), `
UPDATE agent_set_deploys SET heartbeat_at=now(), updated_at=now() WHERE id=$1 AND status=$2
`, deployID, statusRunning)
		}
	}
}

func (s *Server) publishAgentSetDeploy(ctx context.Context, d agentSetDeploy) error {
	if err := s.updateAgentSetDeployStep(ctx, d.ID, "publishing_tester"); err != nil {
		return withPersistedErrorCode(persistedErrAgentDeployUpdateFailed, err)
	}
	tester, err := s.dari.PublishAgentFromSnapshot(ctx, d.TesterSourceSnapshotID, d.TargetTesterAgentID)
	if err != nil {
		return withPersistedErrorCode(persistedErrAgentDeployPublishTesterFailed, fmt.Errorf("publish tester agent: %w", err))
	}
	if _, err := s.db.Exec(ctx, `
UPDATE agent_set_deploys
SET tester_agent_id=$2, tester_version_id=$3, step='publishing_editor', heartbeat_at=now(), updated_at=now()
WHERE id=$1 AND status=$4
`, d.ID, tester.AgentID, tester.VersionID, statusRunning); err != nil {
		return withPersistedErrorCode(persistedErrAgentDeployUpdateFailed, err)
	}
	editor, err := s.dari.PublishAgentFromSnapshot(ctx, d.EditorSourceSnapshotID, d.TargetEditorAgentID)
	if err != nil {
		return withPersistedErrorCode(persistedErrAgentDeployPublishEditorFailed, fmt.Errorf("publish editor agent: %w", err))
	}
	if _, err := s.db.Exec(ctx, `
UPDATE agent_set_deploys
SET editor_agent_id=$2, editor_version_id=$3, step='applying', heartbeat_at=now(), updated_at=now()
WHERE id=$1 AND status=$4
`, d.ID, editor.AgentID, editor.VersionID, statusRunning); err != nil {
		return withPersistedErrorCode(persistedErrAgentDeployUpdateFailed, err)
	}
	if err := s.applyAgentSetDeploy(ctx, d, tester, editor); err != nil {
		return withPersistedErrorCode(persistedErrAgentDeployApplyFailed, err)
	}
	return nil
}

func (s *Server) updateAgentSetDeployStep(ctx context.Context, deployID, step string) error {
	_, err := s.db.Exec(ctx, `
UPDATE agent_set_deploys SET step=$2, heartbeat_at=now(), updated_at=now() WHERE id=$1 AND status=$3
`, deployID, step, statusRunning)
	return err
}

func (s *Server) applyAgentSetDeploy(ctx context.Context, d agentSetDeploy, tester, editor dari.PublishedAgent) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	tag, err := tx.Exec(ctx, `
INSERT INTO agent_sets (
  id, user_id, tester_agent_id, editor_agent_id, tester_version_id, editor_version_id,
  tester_sha256, editor_sha256, applied_deploy_id, applied_deploy_sequence, updated_at
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, now())
ON CONFLICT (id) DO UPDATE SET
  tester_agent_id=EXCLUDED.tester_agent_id,
  editor_agent_id=EXCLUDED.editor_agent_id,
  tester_version_id=EXCLUDED.tester_version_id,
  editor_version_id=EXCLUDED.editor_version_id,
  tester_sha256=EXCLUDED.tester_sha256,
  editor_sha256=EXCLUDED.editor_sha256,
  applied_deploy_id=EXCLUDED.applied_deploy_id,
  applied_deploy_sequence=EXCLUDED.applied_deploy_sequence,
  updated_at=now()
WHERE agent_sets.user_id=EXCLUDED.user_id
  AND agent_sets.applied_deploy_sequence < EXCLUDED.applied_deploy_sequence
`, d.AgentSetID, d.UserID, tester.AgentID, editor.AgentID, tester.VersionID, editor.VersionID, d.TesterSHA256, d.EditorSHA256, d.ID, d.Sequence)
	if err != nil {
		return err
	}
	applied := tag.RowsAffected() > 0
	_, err = tx.Exec(ctx, `
UPDATE agent_set_deploys
SET status=$2, step='completed', applied=$3, error=NULL, heartbeat_at=now(), updated_at=now(), completed_at=now()
WHERE id=$1 AND status=$4
`, d.ID, statusCompleted, applied, statusRunning)
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Server) failAgentSetDeploy(ctx context.Context, d agentSetDeploy, cause error) error {
	_, err := s.db.Exec(ctx, `
UPDATE agent_set_deploys
SET status=$2, error=$3, updated_at=now(), completed_at=now()
WHERE id=$1 AND status IN ($4,$5)
`, d.ID, statusFailed, persistedErrorString(persistedErrorCodeFromError(cause, persistedErrAgentDeployFailed)), statusQueued, statusRunning)
	_ = s.dari.DeleteSourceSnapshot(ctx, d.TesterSourceSnapshotID)
	_ = s.dari.DeleteSourceSnapshot(ctx, d.EditorSourceSnapshotID)
	return err
}

func (s *Server) recoverStaleAgentSetDeploys(ctx context.Context) error {
	if s.cfg.AgentDeployStaleAfter <= 0 {
		return nil
	}
	rows, err := s.db.Query(ctx, `
UPDATE agent_set_deploys
SET status=$1, error=$2, updated_at=now(), completed_at=now()
WHERE status=$3 AND heartbeat_at < now() - ($4::double precision * interval '1 second')
RETURNING id, tester_source_snapshot_id, editor_source_snapshot_id
`, statusFailed, persistedErrorString(persistedErrAgentDeployStale), statusRunning, s.cfg.AgentDeployStaleAfter.Seconds())
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var deployID, testerSourceID, editorSourceID string
		if err := rows.Scan(&deployID, &testerSourceID, &editorSourceID); err != nil {
			return err
		}
		_ = s.dari.DeleteSourceSnapshot(ctx, testerSourceID)
		_ = s.dari.DeleteSourceSnapshot(ctx, editorSourceID)
		log.Printf("marked stale managed agent deploy %s failed", deployID)
	}
	return rows.Err()
}
