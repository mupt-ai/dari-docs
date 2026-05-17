package managedservice

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/logger"
)

type managedRunStore struct {
	db *gorm.DB
}

type managedRunModel struct {
	ID                       string     `gorm:"column:id;primaryKey"`
	UserID                   string     `gorm:"column:user_id"`
	Mode                     string     `gorm:"column:mode"`
	Status                   string     `gorm:"column:status"`
	Tasks                    []byte     `gorm:"column:tasks"`
	TesterAgentID            string     `gorm:"column:tester_agent_id"`
	TesterVersionID          string     `gorm:"column:tester_version_id"`
	EditorAgentID            string     `gorm:"column:editor_agent_id"`
	EditorVersionID          string     `gorm:"column:editor_version_id"`
	BundleFileID             *string    `gorm:"column:bundle_file_id"`
	BundleSHA256             string     `gorm:"column:bundle_sha256"`
	BundleFiles              int        `gorm:"column:bundle_files"`
	LiveVerify               bool       `gorm:"column:live_verify"`
	RuntimeSecretNames       []byte     `gorm:"column:runtime_secret_names"`
	ReservedCents            int64      `gorm:"column:reserved_cents"`
	ChargedCents             int64      `gorm:"column:charged_cents"`
	CostStatus               *string    `gorm:"column:cost_status"`
	Error                    *string    `gorm:"column:error"`
	EditorSessionID          *string    `gorm:"column:editor_session_id"`
	RuntimeSecretsNonce      []byte     `gorm:"column:runtime_secrets_nonce"`
	RuntimeSecretsCiphertext []byte     `gorm:"column:runtime_secrets_ciphertext"`
	CreatedAt                time.Time  `gorm:"column:created_at"`
	UpdatedAt                time.Time  `gorm:"column:updated_at"`
	CompletedAt              *time.Time `gorm:"column:completed_at"`
}

func (managedRunModel) TableName() string { return "runs" }

type runSessionModel struct {
	SessionID       string     `gorm:"column:session_id;primaryKey"`
	RunID           string     `gorm:"column:run_id"`
	Kind            string     `gorm:"column:kind"`
	TaskIndex       int        `gorm:"column:task_index"`
	Status          string     `gorm:"column:status"`
	VersionID       string     `gorm:"column:version_id"`
	CostCents       int64      `gorm:"column:cost_cents"`
	ChargeCents     int64      `gorm:"column:charge_cents"`
	LastPollError   *string    `gorm:"column:last_poll_error"`
	LastPollErrorAt *time.Time `gorm:"column:last_poll_error_at"`
	LastPolledAt    *time.Time `gorm:"column:last_polled_at"`
	CreatedAt       time.Time  `gorm:"column:created_at"`
	CompletedAt     *time.Time `gorm:"column:completed_at"`
}

func (runSessionModel) TableName() string { return "run_sessions" }

type creditLedgerModel struct {
	ID          string `gorm:"column:id;primaryKey"`
	UserID      string `gorm:"column:user_id"`
	AmountCents int64  `gorm:"column:amount_cents"`
	Kind        string `gorm:"column:kind"`
	SourceID    string `gorm:"column:source_id"`
	RunID       string `gorm:"column:run_id"`
}

func (creditLedgerModel) TableName() string { return "credit_ledger" }

func openManagedGormDB(databaseURL string) (*gorm.DB, error) {
	return gorm.Open(postgres.Open(databaseURL), &gorm.Config{
		SkipDefaultTransaction: true,
		Logger:                 logger.Default.LogMode(logger.Silent),
	})
}

func newManagedRunStore(db *gorm.DB) *managedRunStore {
	return &managedRunStore{db: db}
}

func (s *Server) runs() (*managedRunStore, error) {
	if s.runStore != nil {
		return s.runStore, nil
	}
	if s.gormDB == nil {
		return nil, errors.New("managed run store is not configured")
	}
	s.runStore = newManagedRunStore(s.gormDB)
	return s.runStore, nil
}

func (store *managedRunStore) ClaimStartableRun(ctx context.Context) (queuedRun, bool, error) {
	var model managedRunModel
	err := store.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).
			Where("status = ?", statusQueued).
			Order("created_at").
			First(&model).Error; err != nil {
			return err
		}
		return tx.Model(&managedRunModel{}).
			Where("id = ?", model.ID).
			Updates(map[string]any{"status": statusStarting, "updated_at": time.Now()}).Error
	})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return queuedRun{}, false, nil
	}
	if err != nil {
		return queuedRun{}, false, err
	}
	run, err := model.toQueuedRun()
	return run, err == nil, err
}

func (store *managedRunStore) InsertStartedRunSession(
	ctx context.Context,
	sessionID string,
	runID string,
	kind string,
	taskIndex int,
	versionID string,
) error {
	return store.db.WithContext(ctx).
		Clauses(clause.OnConflict{DoNothing: true}).
		Create(&runSessionModel{
			SessionID: sessionID,
			RunID:     runID,
			Kind:      kind,
			TaskIndex: taskIndex,
			Status:    statusRunning,
			VersionID: versionID,
		}).Error
}

func (store *managedRunStore) MarkRunRunningFromStarting(ctx context.Context, runID string) error {
	return store.db.WithContext(ctx).Model(&managedRunModel{}).
		Where("id = ?", runID).
		Where("status = ?", statusStarting).
		Updates(map[string]any{"status": statusRunning, "updated_at": time.Now()}).Error
}

func (store *managedRunStore) RecoverStaleStartingRuns(ctx context.Context, staleBefore time.Time) error {
	subquery := store.db.Model(&runSessionModel{}).
		Select("1").
		Where("run_sessions.run_id = runs.id").
		Where("run_sessions.status IN ?", []string{statusStarting, statusRunning})
	return store.db.WithContext(ctx).Model(&managedRunModel{}).
		Where("status = ?", statusStarting).
		Where("updated_at < ?", staleBefore).
		Where("NOT EXISTS (?)", subquery).
		Updates(map[string]any{"status": statusQueued, "updated_at": time.Now()}).Error
}

func (store *managedRunStore) RecoverStaleUploadingRuns(
	ctx context.Context,
	staleBefore time.Time,
	code persistedErrorCode,
) ([]queuedRun, error) {
	var models []managedRunModel
	err := store.db.WithContext(ctx).Model(&models).
		Clauses(clause.Returning{Columns: []clause.Column{{Name: "id"}, {Name: "reserved_cents"}}}).
		Where("status = ?", statusUploading).
		Where("updated_at < ?", staleBefore).
		Updates(map[string]any{
			"status":       statusFailed,
			"error":        persistedErrorString(code),
			"updated_at":   time.Now(),
			"completed_at": time.Now(),
		}).Error
	if err != nil {
		return nil, err
	}
	runs := make([]queuedRun, 0, len(models))
	for _, model := range models {
		runs = append(runs, queuedRun{ID: model.ID, ReservedCents: model.ReservedCents})
	}
	return runs, nil
}

func (store *managedRunStore) ListRunningSessions(ctx context.Context, limit int) ([]runSessionRecord, error) {
	var models []runSessionModel
	if err := store.db.WithContext(ctx).
		Where("status = ?", statusRunning).
		Order("created_at").
		Limit(limit).
		Find(&models).Error; err != nil {
		return nil, err
	}
	return sessionModelsToRecords(models), nil
}

func (store *managedRunStore) MarkSessionCompleted(ctx context.Context, sessionID string) error {
	return store.updateRunningSession(ctx, sessionID, map[string]any{
		"status":             statusCompleted,
		"completed_at":       time.Now(),
		"last_polled_at":     time.Now(),
		"last_poll_error_at": nil,
		"last_poll_error":    nil,
	})
}

func (store *managedRunStore) MarkSessionFailed(ctx context.Context, sessionID string, code persistedErrorCode) error {
	return store.updateRunningSession(ctx, sessionID, map[string]any{
		"status":             statusFailed,
		"completed_at":       time.Now(),
		"last_polled_at":     time.Now(),
		"last_poll_error_at": nil,
		"last_poll_error":    persistedErrorString(code),
	})
}

func (store *managedRunStore) MarkSessionPollSucceeded(ctx context.Context, sessionID string) error {
	return store.updateRunningSession(ctx, sessionID, map[string]any{
		"last_polled_at":     time.Now(),
		"last_poll_error_at": nil,
		"last_poll_error":    nil,
	})
}

func (store *managedRunStore) RecordSessionPollError(
	ctx context.Context,
	session runSessionRecord,
	code persistedErrorCode,
) (time.Time, error) {
	firstErrorAt := session.LastPollErrorAt
	updates := map[string]any{
		"last_polled_at":  time.Now(),
		"last_poll_error": persistedErrorString(code),
	}
	if firstErrorAt == nil {
		now := time.Now()
		firstErrorAt = &now
		updates["last_poll_error_at"] = now
	}
	return *firstErrorAt, store.db.WithContext(ctx).Model(&runSessionModel{}).
		Where("session_id = ?", session.ID).
		Where("status = ?", session.Status).
		Updates(updates).Error
}

func (store *managedRunStore) FailRunSession(ctx context.Context, session runSessionRecord, code persistedErrorCode) (bool, error) {
	result := store.db.WithContext(ctx).Model(&runSessionModel{}).
		Where("session_id = ?", session.ID).
		Where("status = ?", session.Status).
		Updates(map[string]any{
			"status":          statusFailed,
			"completed_at":    time.Now(),
			"last_polled_at":  time.Now(),
			"last_poll_error": persistedErrorString(code),
		})
	return result.RowsAffected > 0, result.Error
}

func (store *managedRunStore) MarkRunRunningIfStartable(ctx context.Context, runID string) error {
	return store.db.WithContext(ctx).Model(&managedRunModel{}).
		Where("id = ?", runID).
		Where("status IN ?", []string{statusQueued, statusStarting}).
		Updates(map[string]any{"status": statusRunning, "updated_at": time.Now()}).Error
}

func (store *managedRunStore) MarkRunQueuedIfActive(ctx context.Context, runID string) error {
	return store.db.WithContext(ctx).Model(&managedRunModel{}).
		Where("id = ?", runID).
		Where("status IN ?", []string{statusQueued, statusRunning, statusStarting}).
		Updates(map[string]any{"status": statusQueued, "updated_at": time.Now()}).Error
}

func (store *managedRunStore) LoadActiveRun(ctx context.Context, runID string) (queuedRun, error) {
	var model managedRunModel
	err := store.db.WithContext(ctx).
		Where("id = ?", runID).
		Where("status IN ?", []string{statusQueued, statusStarting, statusRunning}).
		First(&model).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return queuedRun{}, nil
	}
	if err != nil {
		return queuedRun{}, err
	}
	return model.toQueuedRun()
}

func (store *managedRunStore) FinishRun(
	ctx context.Context,
	run queuedRun,
	editorSessionID string,
	failureCode persistedErrorCode,
) (bool, error) {
	updates := map[string]any{
		"updated_at":   time.Now(),
		"completed_at": time.Now(),
	}
	if failureCode != "" {
		updates["status"] = statusFailed
		updates["error"] = persistedErrorString(failureCode)
	} else {
		updates["status"] = statusCompleted
		updates["error"] = nil
		updates["editor_session_id"] = editorSessionID
	}
	result := store.db.WithContext(ctx).Model(&managedRunModel{}).
		Where("id = ?", run.ID).
		Where("status IN ?", []string{statusQueued, statusStarting, statusRunning}).
		Updates(updates)
	return result.RowsAffected > 0, result.Error
}

func (store *managedRunStore) ListUnsettledRuns(ctx context.Context, limit int) ([]queuedRun, error) {
	var models []managedRunModel
	if err := store.db.WithContext(ctx).
		Where("status IN ?", []string{statusCompleted, statusFailed}).
		Where("cost_status IS NULL OR cost_status = ?", "estimated").
		Order("completed_at NULLS FIRST, updated_at").
		Limit(limit).
		Find(&models).Error; err != nil {
		return nil, err
	}
	runs := make([]queuedRun, 0, len(models))
	for _, model := range models {
		runs = append(runs, queuedRun{ID: model.ID, ReservedCents: model.ReservedCents})
	}
	return runs, nil
}

func (store *managedRunStore) LoadRunSessions(ctx context.Context, runID string) ([]runSessionRecord, error) {
	var models []runSessionModel
	if err := store.db.WithContext(ctx).
		Where("run_id = ?", runID).
		Order("created_at").
		Find(&models).Error; err != nil {
		return nil, err
	}
	return sessionModelsToRecords(models), nil
}

func (store *managedRunStore) ListRunSessionIDs(ctx context.Context, runID string) ([]string, error) {
	var ids []string
	if err := store.db.WithContext(ctx).Model(&runSessionModel{}).
		Where("run_id = ?", runID).
		Pluck("session_id", &ids).Error; err != nil {
		return nil, err
	}
	return ids, nil
}

func (store *managedRunStore) UpdateSessionCost(ctx context.Context, sessionID string, costCents, chargeCents int64) error {
	return store.db.WithContext(ctx).Model(&runSessionModel{}).
		Where("session_id = ?", sessionID).
		Updates(map[string]any{"cost_cents": costCents, "charge_cents": chargeCents}).Error
}

func (store *managedRunStore) InsertRunLedgerAdjustment(ctx context.Context, runID string, amountCents int64, kind, sourceID string) error {
	var run managedRunModel
	if err := store.db.WithContext(ctx).Select("id", "user_id").First(&run, "id = ?", runID).Error; err != nil {
		return err
	}
	return store.db.WithContext(ctx).
		Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "source_id"}}, DoNothing: true}).
		Create(&creditLedgerModel{
			ID:          "led_" + randomToken(18),
			UserID:      run.UserID,
			AmountCents: amountCents,
			Kind:        kind,
			SourceID:    sourceID,
			RunID:       runID,
		}).Error
}

func (store *managedRunStore) MarkRunSettled(ctx context.Context, runID string, totalCharge int64, costStatus string) error {
	return store.db.WithContext(ctx).Model(&managedRunModel{}).
		Where("id = ?", runID).
		Updates(map[string]any{"charged_cents": totalCharge, "cost_status": costStatus}).Error
}

func (store *managedRunStore) updateRunningSession(ctx context.Context, sessionID string, updates map[string]any) error {
	return store.db.WithContext(ctx).Model(&runSessionModel{}).
		Where("session_id = ?", sessionID).
		Where("status = ?", statusRunning).
		Updates(updates).Error
}

func (model managedRunModel) toQueuedRun() (queuedRun, error) {
	run := queuedRun{
		ID:              model.ID,
		UserID:          model.UserID,
		Mode:            model.Mode,
		TesterAgentID:   model.TesterAgentID,
		TesterVersionID: model.TesterVersionID,
		EditorAgentID:   model.EditorAgentID,
		EditorVersionID: model.EditorVersionID,
		BundleSHA256:    model.BundleSHA256,
		BundleFiles:     model.BundleFiles,
		LiveVerify:      model.LiveVerify,
		ReservedCents:   model.ReservedCents,
	}
	if model.BundleFileID != nil {
		run.BundleFileID = *model.BundleFileID
	}
	if len(model.Tasks) > 0 {
		if err := json.Unmarshal(model.Tasks, &run.Tasks); err != nil {
			return queuedRun{}, fmt.Errorf("decode run tasks: %w", err)
		}
	}
	if len(model.RuntimeSecretNames) > 0 {
		if err := json.Unmarshal(model.RuntimeSecretNames, &run.SecretNames); err != nil {
			return queuedRun{}, fmt.Errorf("decode runtime secret names: %w", err)
		}
	}
	return run, nil
}

func sessionModelsToRecords(models []runSessionModel) []runSessionRecord {
	records := make([]runSessionRecord, 0, len(models))
	for _, model := range models {
		lastPollError := ""
		if model.LastPollError != nil {
			lastPollError = *model.LastPollError
		}
		records = append(records, runSessionRecord{
			ID:              model.SessionID,
			RunID:           model.RunID,
			Kind:            model.Kind,
			TaskIndex:       model.TaskIndex,
			Status:          model.Status,
			CreatedAt:       model.CreatedAt,
			LastPollErrorAt: model.LastPollErrorAt,
			LastPollError:   lastPollError,
		})
	}
	return records
}
