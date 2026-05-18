package managedservice

import (
	"context"
	"fmt"
	"log"
	"time"

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
	TesterLLMIDs    []string
	EditorLLMID     string
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
	LLMID           string
	CreatedAt       time.Time
	LastPollErrorAt *time.Time
	LastPollError   string
}

type nextSession struct {
	Kind      string
	TaskIndex int
	AgentID   string
	VersionID string
	LLMID     string
	Prompt    string
}

func (s *Server) sessionStarterLoop(ctx context.Context) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		if err := s.recoverStaleUploadingRuns(ctx); err != nil {
			log.Printf("recover stale uploading runs: %v", err)
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
	store, err := s.runs()
	if err != nil {
		return queuedRun{}, false, err
	}
	return store.ClaimStartableRun(ctx)
}

func (s *Server) startNextSession(ctx context.Context, run queuedRun) error {
	sessions, err := s.loadRunSessions(ctx, run.ID)
	if err != nil {
		return err
	}
	if shouldStartTesterBatch(run, sessions) {
		return s.startTesterBatch(ctx, run)
	}
	next, ok, err := s.nextSession(ctx, run, sessions)
	if err != nil {
		return s.failStartedRun(ctx, run, persistedErrSessionFailed, err)
	}
	if !ok {
		return s.reconcileRunProgress(ctx, run.ID)
	}
	return s.startSingleSessionBatch(ctx, run, next)
}

func (s *Server) startSingleSessionBatch(ctx context.Context, run queuedRun, next nextSession) error {
	var secrets map[string]string
	if shouldAttachRuntimeSecrets(run, next) {
		secretJSON, err := s.runtimeSecretsJSON(ctx, run.ID)
		if err != nil {
			return s.failStartedRun(ctx, run, persistedErrRuntimeSecretsLoadFailed, fmt.Errorf("load runtime secrets: %w", err))
		}
		if secretJSON != "" {
			secrets = map[string]string{managedRuntimeSecretsName: secretJSON}
		}
	}
	batch, err := s.dari.CreateSessionBatch(ctx, dari.CreateSessionBatchRequest{
		IdempotencyKey: "dari-docs-managed-" + run.ID + "-" + next.Kind,
		Items: []dari.CreateSessionBatchItem{{
			AgentID:   next.AgentID,
			VersionID: next.VersionID,
			LLMID:     managedLLMIDOrDefault(next.LLMID),
			Metadata:  managedSessionMetadata(run, next),
			Secrets:   secrets,
			Message: dari.CreateSessionBatchMessage{Content: []dari.ContentBlock{
				dari.TextBlock(next.Prompt),
				dari.FileBlock(run.BundleFileID),
			}},
		}},
	})
	if err != nil {
		return s.failStartedRun(ctx, run, persistedErrSessionCreateFailed, fmt.Errorf("create %s session batch: %w", next.Kind, err))
	}
	if !singleBatchSessionStarted(batch) {
		msg := "missing session"
		if len(batch.Sessions) > 0 && batch.Sessions[0].Error != "" {
			msg = batch.Sessions[0].Error
		}
		return s.failStartedRun(ctx, run, persistedErrSessionCreateFailed, fmt.Errorf("create %s session: %s", next.Kind, msg))
	}
	item := batch.Sessions[0]
	versionID := item.VersionID
	if versionID == "" {
		versionID = next.VersionID
	}
	llmID := managedLLMIDOrDefault(item.LLMID)
	store, err := s.runs()
	if err != nil {
		return err
	}
	if err := store.InsertStartedRunSession(ctx, item.SessionID, run.ID, next.Kind, next.TaskIndex, versionID, llmID); err != nil {
		return err
	}
	if isFinalSecretBearingSession(run, next) {
		s.clearRuntimeSecrets(ctx, run.ID)
	}
	return store.MarkRunRunningFromStarting(ctx, run.ID)
}

func singleBatchSessionStarted(batch dari.SessionBatch) bool {
	return len(batch.Sessions) == 1 &&
		batch.Sessions[0].SessionID != "" &&
		batch.Sessions[0].Status != statusFailed &&
		batch.Sessions[0].Error == ""
}

func managedSessionMetadata(run queuedRun, next nextSession) map[string]string {
	metadata := map[string]string{
		"managed_run_id": run.ID,
		"kind":           next.Kind,
	}
	if next.TaskIndex > 0 {
		metadata["task_index"] = fmt.Sprintf("%d", next.TaskIndex)
	}
	return metadata
}

func shouldStartTesterBatch(run queuedRun, sessions []runSessionRecord) bool {
	return len(run.Tasks) > 0 && len(sessions) == 0
}

func (s *Server) startTesterBatch(ctx context.Context, run queuedRun) error {
	b := bundle.Result{SHA256: run.BundleSHA256, Manifest: bundle.Manifest{Files: make([]bundle.FileRecord, run.BundleFiles)}}
	var secrets map[string]string
	if run.LiveVerify {
		secretJSON, err := s.runtimeSecretsJSON(ctx, run.ID)
		if err != nil {
			return s.failStartedRun(ctx, run, persistedErrRuntimeSecretsLoadFailed, fmt.Errorf("load runtime secrets: %w", err))
		}
		if secretJSON != "" {
			secrets = map[string]string{managedRuntimeSecretsName: secretJSON}
		}
	}
	batchReq := dari.CreateSessionBatchRequest{
		IdempotencyKey: "dari-docs-managed-" + run.ID + "-testers",
		Items:          make([]dari.CreateSessionBatchItem, 0, expectedTesterSessions(run)),
	}
	items := testerBatchItems(run)
	for _, item := range items {
		metadata := map[string]string{
			"managed_run_id": run.ID,
			"kind":           "tester",
			"task_index":     fmt.Sprintf("%d", item.taskIndex+1),
			"llm_id":         item.llmID,
		}
		batchReq.Items = append(batchReq.Items, dari.CreateSessionBatchItem{
			AgentID:   run.TesterAgentID,
			VersionID: run.TesterVersionID,
			LLMID:     item.llmID,
			Metadata:  metadata,
			Secrets:   secrets,
			Message: dari.CreateSessionBatchMessage{Content: []dari.ContentBlock{
				dari.TextBlock(runner.FeedbackPrompt(item.task, b, run.LiveVerify, secretNameMap(run.SecretNames))),
				dari.FileBlock(run.BundleFileID),
			}},
		})
	}
	batch, err := s.dari.CreateSessionBatch(ctx, batchReq)
	if err != nil {
		return s.failStartedRun(ctx, run, persistedErrSessionCreateFailed, fmt.Errorf("create tester session batch: %w", err))
	}
	store, err := s.runs()
	if err != nil {
		return err
	}
	var createErr error
	for _, item := range batch.Sessions {
		if item.Index < 0 || item.Index >= len(items) {
			return s.failStartedRun(ctx, run, persistedErrSessionCreateFailed, fmt.Errorf("tester batch returned invalid item index %d", item.Index))
		}
		expected := items[item.Index]
		if item.Status == statusFailed || item.SessionID == "" || item.Error != "" {
			if createErr == nil {
				createErr = fmt.Errorf("create tester session %d: %s", item.Index+1, item.Error)
			}
			continue
		}
		versionID := item.VersionID
		if versionID == "" {
			versionID = run.TesterVersionID
		}
		if err := store.InsertStartedRunSession(ctx, item.SessionID, run.ID, "tester", expected.taskIndex+1, versionID, expected.llmID); err != nil {
			return err
		}
	}
	if createErr == nil && len(batch.Sessions) != len(items) {
		createErr = fmt.Errorf("tester batch returned %d sessions, want %d", len(batch.Sessions), len(items))
	}
	if createErr != nil {
		return s.failStartedRun(ctx, run, persistedErrSessionCreateFailed, createErr)
	}
	if run.Mode == "check" {
		s.clearRuntimeSecrets(ctx, run.ID)
	}
	return store.MarkRunRunningFromStarting(ctx, run.ID)
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
	if completedTesterSessionCount(run, sessions) < expectedTesterSessions(run) {
		return nextSession{}, false, nil
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
		LLMID:     run.EditorLLMID,
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
	store, err := s.runs()
	if err != nil {
		return err
	}
	return store.RecoverStaleStartingRuns(ctx, time.Now().Add(-2*time.Minute))
}

func (s *Server) recoverStaleUploadingRuns(ctx context.Context) error {
	store, err := s.runs()
	if err != nil {
		return err
	}
	runs, err := store.RecoverStaleUploadingRuns(ctx, time.Now().Add(-10*time.Minute), persistedErrBundleUploadIncomplete)
	if err != nil {
		return err
	}
	for _, run := range runs {
		s.clearRuntimeSecrets(ctx, run.ID)
		s.releaseReservation(ctx, run.ID, run.ReservedCents)
	}
	return nil
}

func (s *Server) reconcileRunningSessions(ctx context.Context) error {
	store, err := s.runs()
	if err != nil {
		return err
	}
	sessions, err := store.ListRunningSessions(ctx, 50)
	if err != nil {
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
	store, err := s.runs()
	if err != nil {
		return err
	}
	switch lastStatus {
	case "completed":
		if err := store.MarkSessionCompleted(ctx, session.ID, sessionLLMID(session.LLMID, remote)); err != nil {
			return err
		}
		return s.reconcileRunProgress(ctx, session.RunID)
	case "failed":
		if err := store.MarkSessionFailed(ctx, session.ID, persistedErrSessionFailed, sessionLLMID(session.LLMID, remote)); err != nil {
			return err
		}
		return s.reconcileRunProgress(ctx, session.RunID)
	default:
		if s.cfg.SessionStaleAfter > 0 && time.Since(session.CreatedAt) > s.cfg.SessionStaleAfter {
			return s.failRunSession(ctx, session, persistedErrSessionStale)
		}
		return store.MarkSessionPollSucceeded(ctx, session.ID, sessionLLMID(session.LLMID, remote))
	}
}

func sessionLLMID(current string, remote dari.Session) string {
	if current != "" {
		return current
	}
	if remote.LLMID != nil {
		return *remote.LLMID
	}
	return current
}

func (s *Server) recordSessionPollError(ctx context.Context, session runSessionRecord, _ error) (bool, error) {
	store, err := s.runs()
	if err != nil {
		return false, err
	}
	firstErrorAt, err := store.RecordSessionPollError(ctx, session, persistedErrSessionPollFailed)
	if err != nil {
		return false, err
	}
	return s.cfg.PollErrorStaleAfter > 0 && time.Since(firstErrorAt) > s.cfg.PollErrorStaleAfter, nil
}

func (s *Server) failRunSession(ctx context.Context, session runSessionRecord, code persistedErrorCode) error {
	store, err := s.runs()
	if err != nil {
		return err
	}
	updated, err := store.FailRunSession(ctx, session, code)
	if err != nil || !updated {
		return err
	}
	return s.reconcileRunProgress(ctx, session.RunID)
}

func runErrorCodeFromSession(session runSessionRecord) persistedErrorCode {
	return persistedErrorCodeFromString(session.LastPollError, persistedErrSessionFailed)
}

func (s *Server) reconcileRunProgress(ctx context.Context, runID string) error {
	store, err := s.runs()
	if err != nil {
		return err
	}
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
			return store.MarkRunRunningIfStartable(ctx, run.ID)
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
		return store.MarkRunQueuedIfActive(ctx, run.ID)
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
		return store.MarkRunQueuedIfActive(ctx, run.ID)
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
	store, err := s.runs()
	if err != nil {
		return queuedRun{}, err
	}
	return store.LoadActiveRun(ctx, runID)
}

func (s *Server) finishRun(ctx context.Context, run queuedRun, editorSessionID string, failureCode persistedErrorCode) error {
	store, err := s.runs()
	if err != nil {
		return err
	}
	updated, err := store.FinishRun(ctx, run, editorSessionID, failureCode)
	if err != nil || !updated {
		return err
	}
	s.clearRuntimeSecrets(ctx, run.ID)
	return s.settleRun(ctx, run.ID, run.ReservedCents)
}

func (s *Server) settleUnsettledRuns(ctx context.Context) error {
	store, err := s.runs()
	if err != nil {
		return err
	}
	runs, err := store.ListUnsettledRuns(ctx, 10)
	if err != nil {
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
	store, err := s.runs()
	if err != nil {
		return nil, err
	}
	return store.LoadRunSessions(ctx, runID)
}

func (s *Server) collectTesterReports(ctx context.Context, run queuedRun, sessions []runSessionRecord) ([]string, bool, error) {
	expected := expectedTesterKeys(run)
	reports := make([]string, 0, len(expected))
	seen := map[string]bool{}
	for _, session := range sessions {
		if session.Kind != "tester" || session.Status != statusCompleted || session.TaskIndex < 1 || session.TaskIndex > len(run.Tasks) {
			continue
		}
		llmID := managedLLMIDOrDefault(session.LLMID)
		key := testerSessionKey(session.TaskIndex, llmID)
		if !expected[key] || seen[key] {
			continue
		}
		tr, err := s.dari.GetTranscript(ctx, session.ID)
		if err != nil {
			return nil, false, fmt.Errorf("get transcript %s: %w", session.ID, err)
		}
		reports = append(reports, formatManagedFeedbackReport(session.TaskIndex, llmID, dari.FinalAssistantText(tr)))
		seen[key] = true
	}
	for key := range expected {
		if !seen[key] {
			return reports, false, nil
		}
	}
	return reports, true, nil
}

type testerBatchItem struct {
	taskIndex int
	task      string
	llmID     string
}

func testerBatchItems(run queuedRun) []testerBatchItem {
	llmIDs := run.TesterLLMIDs
	if len(llmIDs) == 0 {
		llmIDs = defaultManagedTesterLLMIDs()
	}
	items := make([]testerBatchItem, 0, len(run.Tasks)*len(llmIDs))
	for taskIndex, task := range run.Tasks {
		for _, llmID := range llmIDs {
			items = append(items, testerBatchItem{taskIndex: taskIndex, task: task, llmID: managedLLMIDOrDefault(llmID)})
		}
	}
	return items
}

func expectedTesterSessions(run queuedRun) int {
	return len(testerBatchItems(run))
}

func completedTesterSessionCount(run queuedRun, sessions []runSessionRecord) int {
	expected := expectedTesterKeys(run)
	seen := map[string]bool{}
	for _, session := range sessions {
		if session.Kind != "tester" || session.Status != statusCompleted {
			continue
		}
		key := testerSessionKey(session.TaskIndex, managedLLMIDOrDefault(session.LLMID))
		if expected[key] {
			seen[key] = true
		}
	}
	return len(seen)
}

func expectedTesterKeys(run queuedRun) map[string]bool {
	out := map[string]bool{}
	for _, item := range testerBatchItems(run) {
		out[testerSessionKey(item.taskIndex+1, item.llmID)] = true
	}
	return out
}

func testerSessionKey(taskIndex int, llmID string) string {
	return fmt.Sprintf("%d:%s", taskIndex, managedLLMIDOrDefault(llmID))
}

func formatManagedFeedbackReport(taskIndex int, llmID string, report string) string {
	header := fmt.Sprintf("Task index: %d\nTester LLM: %s", taskIndex, managedLLMIDOrDefault(llmID))
	return header + "\n\n" + report
}

func waitForCostRetry(ctx context.Context, deadline time.Time) error {
	wait := 5 * time.Second
	remaining := time.Until(deadline)
	if remaining <= 0 {
		return nil
	}
	if remaining < wait {
		wait = remaining
	}
	timer := time.NewTimer(wait)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func (s *Server) settleRun(ctx context.Context, runID string, reserve int64) error {
	store, err := s.runs()
	if err != nil {
		return err
	}
	sessionIDs, err := store.ListRunSessionIDs(ctx, runID)
	if err != nil {
		return err
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
			if err := store.UpdateSessionCost(ctx, id, costCents, charges[id]); err != nil {
				return fmt.Errorf("update session cost: %w", err)
			}
		}
		if len(charges) == len(sessionIDs) || s.cfg.CostFetchTimeout == 0 {
			break
		}
		if err := waitForCostRetry(ctx, deadline); err != nil {
			return err
		}
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
		if err := store.InsertRunLedgerAdjustment(ctx, runID, delta, kind, source); err != nil {
			return err
		}
	}
	return store.MarkRunSettled(ctx, runID, totalCharge, costStatus)
}
