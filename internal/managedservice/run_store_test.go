package managedservice

import (
	"context"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type fakeRunStoreDB struct {
	execCalls []fakeExecCall
	execErr   error
}

type fakeExecCall struct {
	sql  string
	args []any
}

func (db *fakeRunStoreDB) BeginTx(context.Context, pgx.TxOptions) (pgx.Tx, error) {
	panic("BeginTx should not be called")
}

func (db *fakeRunStoreDB) Exec(_ context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	db.execCalls = append(db.execCalls, fakeExecCall{
		sql:  sql,
		args: append([]any(nil), args...),
	})
	return pgconn.NewCommandTag("UPDATE 1"), db.execErr
}

func (db *fakeRunStoreDB) Query(context.Context, string, ...any) (pgx.Rows, error) {
	panic("Query should not be called")
}

func (db *fakeRunStoreDB) QueryRow(context.Context, string, ...any) pgx.Row {
	panic("QueryRow should not be called")
}

func TestManagedRunStoreExecOnlyMethodsPassSQLAndArgumentsInOrder(t *testing.T) {
	ctx := context.Background()
	staleBefore := time.Unix(1700000000, 0).UTC()
	startableStatuses := []string{statusQueued, statusStarting, statusRunning}
	activeStatuses := []string{statusQueued, statusRunning, statusStarting}

	tests := []struct {
		name            string
		run             func(context.Context, *managedRunStore) error
		wantSQLContains string
		wantArgs        []any
	}{
		{
			name: "insert started run session",
			run: func(ctx context.Context, store *managedRunStore) error {
				return store.InsertStartedRunSession(ctx, "sess_test", "run_test", "tester", 2, "ver_test", "llm_test")
			},
			wantSQLContains: "INSERT INTO run_sessions",
			wantArgs:        []any{"sess_test", "run_test", "tester", 2, statusRunning, "ver_test", "llm_test"},
		},
		{
			name: "mark run running from starting",
			run: func(ctx context.Context, store *managedRunStore) error {
				return store.MarkRunRunningFromStarting(ctx, "run_test")
			},
			wantSQLContains: "UPDATE runs",
			wantArgs:        []any{statusRunning, "run_test", statusStarting},
		},
		{
			name: "recover stale starting runs",
			run: func(ctx context.Context, store *managedRunStore) error {
				return store.RecoverStaleStartingRuns(ctx, staleBefore)
			},
			wantSQLContains: "NOT EXISTS",
			wantArgs:        []any{statusQueued, statusStarting, staleBefore, []string{statusStarting, statusRunning}},
		},
		{
			name: "mark session completed",
			run: func(ctx context.Context, store *managedRunStore) error {
				return store.MarkSessionCompleted(ctx, "sess_test", "llm_test")
			},
			wantSQLContains: "WHERE session_id=$3",
			wantArgs:        []any{statusCompleted, "llm_test", "sess_test", statusRunning},
		},
		{
			name: "mark session failed",
			run: func(ctx context.Context, store *managedRunStore) error {
				return store.MarkSessionFailed(ctx, "sess_test", persistedErrSessionFailed, "llm_test")
			},
			wantSQLContains: "WHERE session_id=$4",
			wantArgs:        []any{statusFailed, persistedErrorString(persistedErrSessionFailed), "llm_test", "sess_test", statusRunning},
		},
		{
			name: "mark session poll succeeded",
			run: func(ctx context.Context, store *managedRunStore) error {
				return store.MarkSessionPollSucceeded(ctx, "sess_test", "llm_test")
			},
			wantSQLContains: "WHERE session_id=$2",
			wantArgs:        []any{"llm_test", "sess_test", statusRunning},
		},
		{
			name: "fail run session",
			run: func(ctx context.Context, store *managedRunStore) error {
				updated, err := store.FailRunSession(ctx, runSessionRecord{ID: "sess_test", Status: statusRunning}, persistedErrSessionStale)
				if err != nil {
					return err
				}
				if !updated {
					t.Fatalf("FailRunSession updated = false, want true")
				}
				return nil
			},
			wantSQLContains: "UPDATE run_sessions",
			wantArgs:        []any{statusFailed, persistedErrorString(persistedErrSessionStale), "sess_test", statusRunning},
		},
		{
			name: "mark run running if startable",
			run: func(ctx context.Context, store *managedRunStore) error {
				return store.MarkRunRunningIfStartable(ctx, "run_test")
			},
			wantSQLContains: "status = ANY($3)",
			wantArgs:        []any{statusRunning, "run_test", []string{statusQueued, statusStarting}},
		},
		{
			name: "mark run queued if active",
			run: func(ctx context.Context, store *managedRunStore) error {
				return store.MarkRunQueuedIfActive(ctx, "run_test")
			},
			wantSQLContains: "status = ANY($3)",
			wantArgs:        []any{statusQueued, "run_test", activeStatuses},
		},
		{
			name: "finish run completed",
			run: func(ctx context.Context, store *managedRunStore) error {
				updated, err := store.FinishRun(ctx, queuedRun{ID: "run_test"}, "sess_editor", "")
				if err != nil {
					return err
				}
				if !updated {
					t.Fatalf("FinishRun updated = false, want true")
				}
				return nil
			},
			wantSQLContains: "editor_session_id=$2",
			wantArgs:        []any{statusCompleted, "sess_editor", "run_test", startableStatuses},
		},
		{
			name: "finish run failed",
			run: func(ctx context.Context, store *managedRunStore) error {
				updated, err := store.FinishRun(ctx, queuedRun{ID: "run_test"}, "", persistedErrSessionFailed)
				if err != nil {
					return err
				}
				if !updated {
					t.Fatalf("FinishRun updated = false, want true")
				}
				return nil
			},
			wantSQLContains: "error=$2",
			wantArgs:        []any{statusFailed, persistedErrorString(persistedErrSessionFailed), "run_test", startableStatuses},
		},
		{
			name: "update session cost",
			run: func(ctx context.Context, store *managedRunStore) error {
				return store.UpdateSessionCost(ctx, "sess_test", 12, 17)
			},
			wantSQLContains: "WHERE session_id=$3",
			wantArgs:        []any{int64(12), int64(17), "sess_test"},
		},
		{
			name: "mark run settled",
			run: func(ctx context.Context, store *managedRunStore) error {
				return store.MarkRunSettled(ctx, "run_test", 99, "actual")
			},
			wantSQLContains: "cost_status=$2",
			wantArgs:        []any{int64(99), "actual", "run_test"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := &fakeRunStoreDB{}
			store := &managedRunStore{db: db}

			if err := tt.run(ctx, store); err != nil {
				t.Fatal(err)
			}
			if len(db.execCalls) != 1 {
				t.Fatalf("Exec calls = %d, want 1", len(db.execCalls))
			}
			got := db.execCalls[0]
			if !strings.Contains(got.sql, tt.wantSQLContains) {
				t.Fatalf("SQL = %q, want to contain %q", got.sql, tt.wantSQLContains)
			}
			if !reflect.DeepEqual(got.args, tt.wantArgs) {
				t.Fatalf("args = %#v, want %#v", got.args, tt.wantArgs)
			}
		})
	}
}

func TestManagedRunStoreRecordSessionPollErrorExecPaths(t *testing.T) {
	ctx := context.Background()

	t.Run("first poll error records timestamp", func(t *testing.T) {
		db := &fakeRunStoreDB{}
		store := &managedRunStore{db: db}

		firstErrorAt, err := store.RecordSessionPollError(
			ctx,
			runSessionRecord{ID: "sess_test", Status: statusRunning},
			persistedErrSessionPollFailed,
		)
		if err != nil {
			t.Fatal(err)
		}
		if firstErrorAt.IsZero() {
			t.Fatal("firstErrorAt is zero")
		}
		if len(db.execCalls) != 1 {
			t.Fatalf("Exec calls = %d, want 1", len(db.execCalls))
		}
		got := db.execCalls[0]
		if !strings.Contains(got.sql, "last_poll_error_at=$2") {
			t.Fatalf("SQL = %q, want first-error-at update", got.sql)
		}
		if len(got.args) != 4 || got.args[0] != persistedErrorString(persistedErrSessionPollFailed) || got.args[2] != "sess_test" || got.args[3] != statusRunning {
			t.Fatalf("args = %#v, want error, timestamp, session id, status", got.args)
		}
		if _, ok := got.args[1].(time.Time); !ok {
			t.Fatalf("args[1] = %T, want time.Time", got.args[1])
		}
	})

	t.Run("existing poll error preserves first timestamp", func(t *testing.T) {
		first := time.Unix(1700000000, 0).UTC()
		db := &fakeRunStoreDB{}
		store := &managedRunStore{db: db}

		firstErrorAt, err := store.RecordSessionPollError(
			ctx,
			runSessionRecord{ID: "sess_test", Status: statusRunning, LastPollErrorAt: &first},
			persistedErrSessionPollStale,
		)
		if err != nil {
			t.Fatal(err)
		}
		if !firstErrorAt.Equal(first) {
			t.Fatalf("firstErrorAt = %s, want %s", firstErrorAt, first)
		}
		if len(db.execCalls) != 1 {
			t.Fatalf("Exec calls = %d, want 1", len(db.execCalls))
		}
		got := db.execCalls[0]
		if strings.Contains(got.sql, "last_poll_error_at=$2") {
			t.Fatalf("SQL = %q, should not reset first-error-at", got.sql)
		}
		wantArgs := []any{persistedErrorString(persistedErrSessionPollStale), "sess_test", statusRunning}
		if !reflect.DeepEqual(got.args, wantArgs) {
			t.Fatalf("args = %#v, want %#v", got.args, wantArgs)
		}
	})
}
