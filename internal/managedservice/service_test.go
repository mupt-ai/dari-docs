package managedservice

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/mupt-ai/dari-docs/internal/dari"
	stripe "github.com/stripe/stripe-go/v82"
)

func TestUSDStringToCentsCeil(t *testing.T) {
	tests := map[string]int64{
		"0":       0,
		"0.0004":  1,
		"0.0100":  1,
		"1.23":    123,
		"12.3456": 1235,
		"500.00":  50000,
	}
	for in, want := range tests {
		if got := usdStringToCentsCeil(in); got != want {
			t.Fatalf("usdStringToCentsCeil(%q) = %d, want %d", in, got, want)
		}
	}
}

func TestConfigFromEnvUsesManagedConstants(t *testing.T) {
	setRequiredManagedConfigEnv(t)
	clearManagedConfigOptionalEnv(t)

	cfg, err := ConfigFromEnv()
	if err != nil {
		t.Fatal(err)
	}

	if cfg.Addr != ":"+defaultPort {
		t.Fatalf("Addr = %q, want :%s", cfg.Addr, defaultPort)
	}
	if cfg.PublicBaseURL != defaultPublicBaseURL {
		t.Fatalf("PublicBaseURL = %q, want %q", cfg.PublicBaseURL, defaultPublicBaseURL)
	}
	if cfg.DariAPIBaseURL != defaultDariAPIBaseURL {
		t.Fatalf("DariAPIBaseURL = %q, want %q", cfg.DariAPIBaseURL, defaultDariAPIBaseURL)
	}
	for name, gotWant := range map[string][2]int64{
		"FreeGrantCents":             {cfg.FreeGrantCents, managedFreeGrantCents},
		"TesterReserveCents":         {cfg.TesterReserveCents, managedTesterReserveCents},
		"EditorReserveCents":         {cfg.EditorReserveCents, managedEditorReserveCents},
		"ServiceFeeCents":            {cfg.ServiceFeeCents, managedServiceFeeCents},
		"MaxBundleBytes":             {cfg.MaxBundleBytes, managedMaxBundleBytes},
		"BundleMaxUncompressedBytes": {cfg.BundleMaxUncompressedBytes, managedBundleMaxUncompressedBytes},
		"BundleMaxFileBytes":         {cfg.BundleMaxFileBytes, managedBundleMaxFileBytes},
		"MaxTaskBytes":               {cfg.MaxTaskBytes, managedMaxTaskBytes},
		"MaxActiveRunsPerUser":       {int64(cfg.MaxActiveRunsPerUser), managedMaxActiveRunsPerUser},
		"AgentDeployClaimBatchSize":  {int64(cfg.AgentDeployClaimBatchSize), managedAgentDeployClaimBatchSize},
		"MaxTasksPerRun":             {int64(cfg.MaxTasksPerRun), managedMaxTasksPerRun},
	} {
		if gotWant[0] != gotWant[1] {
			t.Fatalf("%s = %d, want %d", name, gotWant[0], gotWant[1])
		}
	}
	for name, gotWant := range map[string][2]time.Duration{
		"SessionStaleAfter":      {cfg.SessionStaleAfter, time.Duration(managedSessionStaleAfterSeconds) * time.Second},
		"SessionStartStaleAfter": {cfg.SessionStartStaleAfter, time.Duration(managedSessionStartStaleAfterSeconds) * time.Second},
		"PollErrorStaleAfter":    {cfg.PollErrorStaleAfter, time.Duration(managedPollErrorStaleAfterSeconds) * time.Second},
		"CostFetchTimeout":       {cfg.CostFetchTimeout, time.Duration(managedCostFetchTimeoutSeconds) * time.Second},
		"StripeWebhookTolerance": {cfg.StripeWebhookTolerance, time.Duration(managedStripeWebhookToleranceSeconds) * time.Second},
		"HTTPReadHeaderTimeout":  {cfg.HTTPReadHeaderTimeout, time.Duration(managedHTTPReadHeaderTimeoutSeconds) * time.Second},
		"HTTPReadTimeout":        {cfg.HTTPReadTimeout, time.Duration(managedHTTPReadTimeoutSeconds) * time.Second},
		"HTTPWriteTimeout":       {cfg.HTTPWriteTimeout, time.Duration(managedHTTPWriteTimeoutSeconds) * time.Second},
		"HTTPIdleTimeout":        {cfg.HTTPIdleTimeout, time.Duration(managedHTTPIdleTimeoutSeconds) * time.Second},
		"OutboundHTTPTimeout":    {cfg.OutboundHTTPTimeout, time.Duration(managedOutboundHTTPTimeoutSeconds) * time.Second},
	} {
		if gotWant[0] != gotWant[1] {
			t.Fatalf("%s = %s, want %s", name, gotWant[0], gotWant[1])
		}
	}
}

func TestConfigFromEnvKeepsDeploymentOverridesAndIgnoresManagedEnvKnobs(t *testing.T) {
	setRequiredManagedConfigEnv(t)
	clearManagedConfigOptionalEnv(t)
	t.Setenv("PORT", "9090")
	t.Setenv("PUBLIC_BASE_URL", "https://docs.example.test")
	t.Setenv("DARI_API_BASE_URL", "https://api.example.test")

	t.Setenv("FREE_CREDIT_CENTS", "1234")
	t.Setenv("MAX_TASKS_PER_RUN", "9")
	t.Setenv("MAX_ACTIVE_RUNS_PER_USER", "9")
	t.Setenv("AGENT_DEPLOY_CLAIM_BATCH_SIZE", "100")
	t.Setenv("HTTP_READ_TIMEOUT_SECONDS", "45")

	cfg, err := ConfigFromEnv()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Addr != ":9090" {
		t.Fatalf("Addr = %q, want :9090", cfg.Addr)
	}
	if cfg.PublicBaseURL != "https://docs.example.test" {
		t.Fatalf("PublicBaseURL = %q, want https://docs.example.test", cfg.PublicBaseURL)
	}
	if cfg.DariAPIBaseURL != "https://api.example.test" {
		t.Fatalf("DariAPIBaseURL = %q, want https://api.example.test", cfg.DariAPIBaseURL)
	}
	if cfg.FreeGrantCents != managedFreeGrantCents {
		t.Fatalf("FreeGrantCents = %d, want %d", cfg.FreeGrantCents, managedFreeGrantCents)
	}
	if cfg.MaxTasksPerRun != int(managedMaxTasksPerRun) {
		t.Fatalf("MaxTasksPerRun = %d, want %d", cfg.MaxTasksPerRun, managedMaxTasksPerRun)
	}
	if cfg.MaxActiveRunsPerUser != int(managedMaxActiveRunsPerUser) {
		t.Fatalf("MaxActiveRunsPerUser = %d, want %d", cfg.MaxActiveRunsPerUser, managedMaxActiveRunsPerUser)
	}
	if cfg.AgentDeployClaimBatchSize != int(managedAgentDeployClaimBatchSize) {
		t.Fatalf("AgentDeployClaimBatchSize = %d, want %d", cfg.AgentDeployClaimBatchSize, managedAgentDeployClaimBatchSize)
	}
	if cfg.HTTPReadTimeout != time.Duration(managedHTTPReadTimeoutSeconds)*time.Second {
		t.Fatalf("HTTPReadTimeout = %s, want %ds", cfg.HTTPReadTimeout, managedHTTPReadTimeoutSeconds)
	}
}

func TestConfigFromEnvRequiresRuntimeSecretEncryptionKey(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://example.invalid/dari_docs")
	t.Setenv("DARI_API_KEY", "dari_test")
	_, err := ConfigFromEnv()
	if err == nil || !strings.Contains(err.Error(), "DARI_DOCS_SECRET_ENCRYPTION_KEY is required") {
		t.Fatalf("error = %v", err)
	}
}

func TestBaselineMigrationMatchesManagedSQLShape(t *testing.T) {
	data, err := migrationFS.ReadFile("migrations/0001_baseline.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := string(data)
	for _, want := range []string{
		"tasks JSONB NOT NULL",
		"bundle_file_id TEXT",
		"bundle_sha256 TEXT NOT NULL",
		"tester_version_id TEXT",
		"editor_version_id TEXT",
		"reserved_cents BIGINT NOT NULL DEFAULT 0",
		"charged_cents BIGINT NOT NULL DEFAULT 0",
		"runtime_secret_names JSONB NOT NULL DEFAULT '[]'::jsonb",
		"runtime_secrets_nonce BYTEA",
		"session_id TEXT PRIMARY KEY",
		"version_id TEXT",
		"cost_cents BIGINT",
		"last_poll_error_at TIMESTAMPTZ",
		"CREATE TABLE agent_set_deploys",
		"CREATE SEQUENCE agent_set_deploy_sequence",
	} {
		if !strings.Contains(sql, want) {
			t.Fatalf("baseline migration is missing %q", want)
		}
	}
	for _, wrong := range []string{
		"tasks_json",
		"bundle_name",
		"reserve_cents",
		"runtime_secret_names_json",
		"feedback_reports",
		"aggregate_feedback",
		"poll_error_count",
		"checkout_url",
	} {
		if strings.Contains(sql, wrong) {
			t.Fatalf("baseline migration contains stale column name %q", wrong)
		}
	}
}

func TestPersistedErrorCodesDoNotIncludeCauseText(t *testing.T) {
	secret := "blue-cactus-123"
	err := withPersistedErrorCode(persistedErrSessionCreateFailed, fmt.Errorf("upstream echoed %s", secret))
	code := persistedErrorCodeFromError(err, persistedErrRunFailed)
	got := persistedErrorString(code)
	if got != "session_create_failed" {
		t.Fatalf("persisted error code = %q", got)
	}
	if strings.Contains(got, secret) {
		t.Fatalf("persisted error code leaked secret: %q", got)
	}
}

func TestPersistedErrorCodeFromStringRejectsLegacyRawValues(t *testing.T) {
	secret := "blue-cactus-123"
	got := persistedErrorString(persistedErrorCodeFromString("raw upstream error "+secret, persistedErrSessionFailed))
	if got != "session_failed" {
		t.Fatalf("persisted fallback code = %q", got)
	}
	if strings.Contains(got, secret) {
		t.Fatalf("persisted fallback leaked secret: %q", got)
	}
}

func TestRunErrorCodeFromSessionUsesOnlyKnownCodes(t *testing.T) {
	if got := runErrorCodeFromSession(runSessionRecord{LastPollError: string(persistedErrSessionPollStale)}); got != persistedErrSessionPollStale {
		t.Fatalf("runErrorCodeFromSession known code = %q", got)
	}
	if got := runErrorCodeFromSession(runSessionRecord{LastPollError: "raw session error blue-cactus-123"}); got != persistedErrSessionFailed {
		t.Fatalf("runErrorCodeFromSession raw value = %q, want %q", got, persistedErrSessionFailed)
	}
}

func TestSanitizePersistedErrorsMigrationMentionsKnownCodes(t *testing.T) {
	data, err := migrationFS.ReadFile("migrations/0003_sanitize_persisted_errors.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := string(data)
	for code := range validPersistedErrorCodes {
		if !strings.Contains(sql, string(code)) {
			t.Fatalf("sanitize migration is missing persisted error code %q", code)
		}
	}
}

func TestRunRequestIDMigrationAddsUniqueUserRequestIndex(t *testing.T) {
	data, err := migrationFS.ReadFile("migrations/0005_run_request_ids.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := string(data)
	for _, want := range []string{
		"ADD COLUMN run_request_id TEXT",
		"idx_runs_user_request_id",
		"runs (user_id, run_request_id)",
	} {
		if !strings.Contains(sql, want) {
			t.Fatalf("run request migration is missing %q", want)
		}
	}
}

func TestStripeCheckoutIntentMigrationAddsDurableLookupColumns(t *testing.T) {
	data, err := migrationFS.ReadFile("migrations/0004_stripe_checkout_intents.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := string(data)
	for _, want := range []string{
		"checkout_intent_id",
		"stripe_session_id",
		"idx_stripe_checkout_sessions_intent",
		"idx_stripe_checkout_sessions_stripe_session",
	} {
		if !strings.Contains(sql, want) {
			t.Fatalf("checkout intent migration is missing %q", want)
		}
	}
}

func TestRuntimeSecretNamesFromJSON(t *testing.T) {
	names, err := runtimeSecretNamesFromJSON(`{"STRIPE_TEST_KEY":"sk_test","GITHUB_TOKEN":"ghp_test"}`)
	if err != nil {
		t.Fatal(err)
	}
	got := strings.Join(names, ",")
	if got != "GITHUB_TOKEN,STRIPE_TEST_KEY" {
		t.Fatalf("names = %s", got)
	}
	if _, err := runtimeSecretNamesFromJSON(`[]`); err == nil {
		t.Fatal("expected non-object JSON to fail")
	}
	if _, err := runtimeSecretNamesFromJSON(`{"EMPTY":""}`); err == nil {
		t.Fatal("expected empty secret value to fail")
	}
}

func TestIsFinalSecretBearingSession(t *testing.T) {
	tests := []struct {
		name string
		run  queuedRun
		next nextSession
		want bool
	}{
		{
			name: "optimize first tester is not final",
			run:  queuedRun{Mode: "optimize", Tasks: []string{"one", "two"}},
			next: nextSession{Kind: "tester", TaskIndex: 1},
			want: false,
		},
		{
			name: "optimize last tester is not final",
			run:  queuedRun{Mode: "optimize", Tasks: []string{"one", "two"}},
			next: nextSession{Kind: "tester", TaskIndex: 2},
			want: false,
		},
		{
			name: "optimize editor is final",
			run:  queuedRun{Mode: "optimize", Tasks: []string{"one", "two"}},
			next: nextSession{Kind: "editor"},
			want: true,
		},
		{
			name: "check last tester is final",
			run:  queuedRun{Mode: "check", Tasks: []string{"one", "two"}},
			next: nextSession{Kind: "tester", TaskIndex: 2},
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isFinalSecretBearingSession(tt.run, tt.next); got != tt.want {
				t.Fatalf("isFinalSecretBearingSession() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestShouldAttachRuntimeSecretsOnlyForLiveVerifySessions(t *testing.T) {
	tests := []struct {
		name string
		run  queuedRun
		next nextSession
		want bool
	}{
		{
			name: "live tester gets secrets",
			run:  queuedRun{LiveVerify: true},
			next: nextSession{Kind: "tester"},
			want: true,
		},
		{
			name: "live editor gets secrets",
			run:  queuedRun{LiveVerify: true},
			next: nextSession{Kind: "editor"},
			want: true,
		},
		{
			name: "non-live tester does not get secrets",
			run:  queuedRun{LiveVerify: false},
			next: nextSession{Kind: "tester"},
			want: false,
		},
		{
			name: "non-live editor does not get secrets",
			run:  queuedRun{LiveVerify: false},
			next: nextSession{Kind: "editor"},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := shouldAttachRuntimeSecrets(tt.run, tt.next); got != tt.want {
				t.Fatalf("shouldAttachRuntimeSecrets() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSessionHasMessageActivity(t *testing.T) {
	messageID := "msg_test"
	status := "queued"
	tests := []struct {
		name    string
		session dari.Session
		want    bool
	}{
		{name: "no message", session: dari.Session{}, want: false},
		{name: "message id", session: dari.Session{LastMessageID: &messageID}, want: true},
		{name: "message status", session: dari.Session{LastMessageStatus: &status}, want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := sessionHasMessageActivity(tt.session); got != tt.want {
				t.Fatalf("sessionHasMessageActivity() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRecoverStaleStartingSessionPromotesWhenDariHasMessageActivity(t *testing.T) {
	db := openManagedServiceTestDB(t)
	ctx := context.Background()

	userID, runID, sessionID := insertStartingRunSession(t, db)
	t.Cleanup(func() {
		cleanupRunSessionTestRows(userID, runID, db)
	})

	status := "queued"
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/v1/sessions/"+sessionID {
			t.Fatalf("unexpected upstream request %s %s", r.Method, r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(dari.Session{ID: sessionID, LastMessageStatus: &status})
	}))
	defer upstream.Close()

	s := &Server{
		db:   db,
		dari: dari.New(upstream.URL, "dari_test"),
		cfg:  Config{SessionStartStaleAfter: time.Nanosecond, PollErrorStaleAfter: time.Hour},
	}
	if err := s.recoverStaleStartingSessions(ctx); err != nil {
		t.Fatal(err)
	}

	var sessionStatus, runStatus string
	if err := db.QueryRow(ctx, `SELECT status FROM run_sessions WHERE session_id=$1`, sessionID).Scan(&sessionStatus); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(ctx, `SELECT status FROM runs WHERE id=$1`, runID).Scan(&runStatus); err != nil {
		t.Fatal(err)
	}
	if sessionStatus != statusRunning || runStatus != statusRunning {
		t.Fatalf("session status=%q run status=%q, want both running", sessionStatus, runStatus)
	}
}

func TestRecoverStaleStartingSessionFailsWhenDariHasNoMessageActivity(t *testing.T) {
	db := openManagedServiceTestDB(t)
	ctx := context.Background()

	userID, runID, sessionID := insertStartingRunSession(t, db)
	t.Cleanup(func() {
		cleanupRunSessionTestRows(userID, runID, db)
	})

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/v1/sessions/"+sessionID {
			t.Fatalf("unexpected upstream request %s %s", r.Method, r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(dari.Session{ID: sessionID})
	}))
	defer upstream.Close()

	s := &Server{
		db:   db,
		dari: dari.New(upstream.URL, "dari_test"),
		cfg:  Config{SessionStartStaleAfter: time.Nanosecond, PollErrorStaleAfter: time.Hour},
	}
	if err := s.recoverStaleStartingSessions(ctx); err != nil {
		t.Fatal(err)
	}

	var sessionStatus, sessionError, runStatus, runError string
	if err := db.QueryRow(ctx, `SELECT status, coalesce(last_poll_error,'') FROM run_sessions WHERE session_id=$1`, sessionID).Scan(&sessionStatus, &sessionError); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(ctx, `SELECT status, coalesce(error,'') FROM runs WHERE id=$1`, runID).Scan(&runStatus, &runError); err != nil {
		t.Fatal(err)
	}
	if sessionStatus != statusFailed || runStatus != statusFailed {
		t.Fatalf("session status=%q run status=%q, want both failed", sessionStatus, runStatus)
	}
	if sessionError != string(persistedErrSessionMessageFailed) || runError != string(persistedErrSessionMessageFailed) {
		t.Fatalf("session error=%q run error=%q, want %q", sessionError, runError, persistedErrSessionMessageFailed)
	}
}

func TestStripeWebhookRequiresConfiguredSecret(t *testing.T) {
	s := &Server{}
	req := httptest.NewRequest(http.MethodPost, "/v1/stripe/webhook", strings.NewReader(`{"type":"checkout.session.completed"}`))
	rec := httptest.NewRecorder()
	s.handleStripeWebhook(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusServiceUnavailable)
	}
}

func TestHealthAndReadinessHandlers(t *testing.T) {
	s := &Server{}

	healthReq := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	healthRec := httptest.NewRecorder()
	s.handleHealth(healthRec, healthReq)
	if healthRec.Code != http.StatusOK {
		t.Fatalf("health status = %d, want %d", healthRec.Code, http.StatusOK)
	}

	readyReq := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	readyRec := httptest.NewRecorder()
	s.handleReady(readyRec, readyReq)
	if readyRec.Code != http.StatusServiceUnavailable {
		t.Fatalf("ready status = %d, want %d", readyRec.Code, http.StatusServiceUnavailable)
	}

	postReq := httptest.NewRequest(http.MethodPost, "/readyz", nil)
	postRec := httptest.NewRecorder()
	s.handleReady(postRec, postReq)
	if postRec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("ready POST status = %d, want %d", postRec.Code, http.StatusMethodNotAllowed)
	}
}

func TestHandleCheckoutValidatesAmount(t *testing.T) {
	s := &Server{cfg: Config{StripeSecretKey: "sk_test"}}
	req := httptest.NewRequest(http.MethodPost, "/v1/billing/checkout", strings.NewReader(`{"amount_cents":499}`))
	rec := httptest.NewRecorder()
	s.handleCheckout(rec, req, user{ID: "usr_test", Email: "user@example.test"})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestHandleRunConfigReturnsLaunchPricingAndLimits(t *testing.T) {
	s := &Server{cfg: Config{
		FreeGrantCents:             500,
		TesterReserveCents:         75,
		EditorReserveCents:         150,
		ServiceFeeCents:            0,
		MaxTasksPerRun:             3,
		MaxTaskBytes:               10000,
		MaxActiveRunsPerUser:       3,
		MaxBundleBytes:             25 * 1024 * 1024,
		BundleMaxUncompressedBytes: 100 * 1024 * 1024,
		BundleMaxFileBytes:         5 * 1024 * 1024,
	}}
	req := httptest.NewRequest(http.MethodGet, "/v1/runs/config", nil)
	rec := httptest.NewRecorder()

	s.handleRunConfig(rec, req, user{ID: "usr_test", Email: "user@example.test"})

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	var got map[string]int64
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	for key, want := range map[string]int64{
		"free_credit_cents":             500,
		"tester_session_reserve_cents":  75,
		"editor_session_reserve_cents":  150,
		"service_fee_cents":             0,
		"max_tasks_per_run":             3,
		"max_task_bytes":                10000,
		"max_active_runs_per_user":      3,
		"max_bundle_bytes":              25 * 1024 * 1024,
		"bundle_max_uncompressed_bytes": 100 * 1024 * 1024,
		"bundle_max_file_bytes":         5 * 1024 * 1024,
	} {
		if got[key] != want {
			t.Fatalf("%s = %d, want %d; body=%s", key, got[key], want, rec.Body.String())
		}
	}
}

func TestStripeWebhookRejectsInvalidSignature(t *testing.T) {
	s := &Server{cfg: Config{StripeWebhookSecret: "whsec_test", StripeWebhookTolerance: 5 * time.Minute}}
	req := httptest.NewRequest(http.MethodPost, "/v1/stripe/webhook", strings.NewReader(`{"type":"checkout.session.completed"}`))
	req.Header.Set("Stripe-Signature", "t="+strconv.FormatInt(time.Now().Unix(), 10)+",v1=bad")
	rec := httptest.NewRecorder()
	s.handleStripeWebhook(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestStripeWebhookAcceptsSignedIgnoredEvent(t *testing.T) {
	secret := "whsec_test"
	payload := []byte(`{"id":"evt_test","api_version":"2025-08-27.basil","type":"customer.created","data":{"object":{"id":"cus_test"}}}`)
	s := &Server{cfg: Config{StripeWebhookSecret: secret, StripeWebhookTolerance: 5 * time.Minute}}
	req := httptest.NewRequest(http.MethodPost, "/v1/stripe/webhook", strings.NewReader(string(payload)))
	req.Header.Set("Stripe-Signature", stripeSignatureHeader(payload, secret))
	rec := httptest.NewRecorder()
	s.handleStripeWebhook(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestStripeWebhookIgnoresUnpaidCheckoutSession(t *testing.T) {
	secret := "whsec_test"
	payload := []byte(`{
		"id":"evt_test",
		"api_version":"2025-08-27.basil",
		"type":"checkout.session.completed",
		"data":{"object":{
			"id":"cs_test",
			"object":"checkout.session",
			"payment_status":"unpaid",
			"amount_total":500,
			"currency":"usd",
			"metadata":{
				"billing_kind":"dari_docs_credit_purchase",
				"checkout_intent_id":"sci_test"
			}
		}}
	}`)
	s := &Server{cfg: Config{StripeWebhookSecret: secret, StripeWebhookTolerance: 5 * time.Minute}}
	req := httptest.NewRequest(http.MethodPost, "/v1/stripe/webhook", strings.NewReader(string(payload)))
	req.Header.Set("Stripe-Signature", stripeSignatureHeader(payload, secret))
	rec := httptest.NewRecorder()
	s.handleStripeWebhook(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"credited":false`) {
		t.Fatalf("body = %s", rec.Body.String())
	}
}

func TestCreditPaidStripeCheckoutValidatesSignedPayloadAgainstPersistedFieldsBeforeCrediting(t *testing.T) {
	db := openManagedServiceTestDB(t)
	ctx := context.Background()

	userID := "usr_test_" + randomToken(8)
	intentID := "sci_test_" + randomToken(8)
	sessionID := "cs_test_" + randomToken(8)
	t.Cleanup(func() {
		_, _ = db.Exec(context.Background(), `DELETE FROM credit_ledger WHERE source_id=$1`, sessionID)
		_, _ = db.Exec(context.Background(), `DELETE FROM stripe_checkout_sessions WHERE id=$1`, intentID)
		_, _ = db.Exec(context.Background(), `DELETE FROM users WHERE id=$1`, userID)
	})
	if _, err := db.Exec(ctx, `INSERT INTO users (id, auth_subject, email) VALUES ($1, $2, $3)`, userID, "auth_"+userID, userID+"@example.test"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, `
INSERT INTO stripe_checkout_sessions (id, checkout_intent_id, user_id, amount_cents, currency, status)
VALUES ($1, $1, $2, 500, 'usd', 'pending')
`, intentID, userID); err != nil {
		t.Fatal(err)
	}
	session := stripe.CheckoutSession{
		ID:            sessionID,
		PaymentStatus: stripe.CheckoutSessionPaymentStatusPaid,
		AmountTotal:   501,
		Currency:      stripe.CurrencyUSD,
		Metadata: map[string]string{
			"billing_kind":       "dari_docs_credit_purchase",
			"checkout_intent_id": intentID,
		},
	}
	_, err := (&Server{db: db}).creditPaidStripeCheckout(ctx, session)
	if !errors.Is(err, errStripeCheckoutInvalid) {
		t.Fatalf("err = %v, want errStripeCheckoutInvalid", err)
	}
}

func TestBillingReturnPages(t *testing.T) {
	s := &Server{}
	for _, tt := range []struct {
		path string
		fn   func(http.ResponseWriter, *http.Request)
		want string
	}{
		{path: "/billing/success", fn: s.handleBillingSuccess, want: "credits purchased"},
		{path: "/billing/cancel", fn: s.handleBillingCancel, want: "checkout canceled"},
	} {
		req := httptest.NewRequest(http.MethodGet, tt.path, nil)
		rec := httptest.NewRecorder()
		tt.fn(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s status = %d, want %d", tt.path, rec.Code, http.StatusOK)
		}
		if !strings.Contains(rec.Body.String(), tt.want) {
			t.Fatalf("%s body = %s", tt.path, rec.Body.String())
		}
	}
}

func TestCreditPaidStripeCheckoutCreditsPersistedSessionOnce(t *testing.T) {
	dsn := os.Getenv("MANAGEDSERVICE_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set MANAGEDSERVICE_TEST_DATABASE_URL to run managed service database integration tests")
	}
	ctx := context.Background()
	db, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	s := &Server{db: db}
	if err := runMigrations(ctx, dsn); err != nil {
		t.Fatal(err)
	}
	userID := "usr_test_" + randomToken(8)
	sessionID := "cs_test_" + randomToken(8)
	email := userID + "@example.test"
	t.Cleanup(func() {
		_, _ = db.Exec(context.Background(), `DELETE FROM credit_ledger WHERE source_id=$1`, sessionID)
		_, _ = db.Exec(context.Background(), `DELETE FROM stripe_checkout_sessions WHERE id=$1`, sessionID)
		_, _ = db.Exec(context.Background(), `DELETE FROM users WHERE id=$1`, userID)
	})
	if _, err := db.Exec(ctx, `INSERT INTO users (id, auth_subject, email) VALUES ($1, $2, $3)`, userID, "auth_"+userID, email); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, `
INSERT INTO stripe_checkout_sessions (id, checkout_intent_id, stripe_session_id, user_id, amount_cents, currency, status)
VALUES ($1, $1, $1, $2, 1200, 'usd', 'created')
`, sessionID, userID); err != nil {
		t.Fatal(err)
	}
	session := stripe.CheckoutSession{
		ID:            sessionID,
		PaymentStatus: stripe.CheckoutSessionPaymentStatusPaid,
		AmountTotal:   1200,
		Currency:      stripe.CurrencyUSD,
		Metadata: map[string]string{
			"billing_kind": "dari_docs_credit_purchase",
		},
	}
	credited, err := s.creditPaidStripeCheckout(ctx, session)
	if err != nil {
		t.Fatal(err)
	}
	if !credited {
		t.Fatal("first webhook should credit the checkout")
	}
	credited, err = s.creditPaidStripeCheckout(ctx, session)
	if err != nil {
		t.Fatal(err)
	}
	if credited {
		t.Fatal("duplicate webhook should not credit again")
	}
	balance, err := s.balanceCents(ctx, userID)
	if err != nil {
		t.Fatal(err)
	}
	if balance != 1200 {
		t.Fatalf("balance = %d, want 1200", balance)
	}
}

func TestCreateStripeCheckoutPersistsIntentBeforeStripeSession(t *testing.T) {
	db := openManagedServiceTestDB(t)
	ctx := context.Background()

	userID := "usr_test_" + randomToken(8)
	t.Cleanup(func() {
		_, _ = db.Exec(context.Background(), `DELETE FROM stripe_checkout_sessions WHERE user_id=$1`, userID)
		_, _ = db.Exec(context.Background(), `DELETE FROM users WHERE id=$1`, userID)
	})
	if _, err := db.Exec(ctx, `INSERT INTO users (id, auth_subject, email) VALUES ($1, $2, $3)`, userID, "auth_"+userID, userID+"@example.test"); err != nil {
		t.Fatal(err)
	}

	oldCreate := createStripeCheckoutSession
	t.Cleanup(func() {
		createStripeCheckoutSession = oldCreate
	})
	sessionID := "cs_test_" + randomToken(8)
	var gotMetadata map[string]string
	createStripeCheckoutSession = func(ctx context.Context, secret string, httpClient *http.Client, params *stripe.CheckoutSessionCreateParams) (*stripe.CheckoutSession, error) {
		gotMetadata = params.Metadata
		return &stripe.CheckoutSession{ID: sessionID, URL: "https://checkout.stripe.test/session"}, nil
	}

	s := &Server{db: db, cfg: Config{StripeSecretKey: "sk_test", PublicBaseURL: "https://docs.example.test"}}
	checkout, err := s.createStripeCheckout(ctx, user{ID: userID, Email: userID + "@example.test"}, 500)
	if err != nil {
		t.Fatal(err)
	}
	if checkout.ID != sessionID || checkout.URL == "" {
		t.Fatalf("checkout = %#v", checkout)
	}
	intentID := gotMetadata["checkout_intent_id"]
	if intentID == "" {
		t.Fatalf("metadata missing checkout_intent_id: %#v", gotMetadata)
	}
	if _, ok := gotMetadata["user_id"]; ok {
		t.Fatalf("metadata leaked user_id: %#v", gotMetadata)
	}
	if _, ok := gotMetadata["amount_cents"]; ok {
		t.Fatalf("metadata leaked amount_cents: %#v", gotMetadata)
	}
	var rowStatus, rowSessionID string
	if err := db.QueryRow(ctx, `
SELECT status, coalesce(stripe_session_id,'')
FROM stripe_checkout_sessions
WHERE checkout_intent_id=$1
`, intentID).Scan(&rowStatus, &rowSessionID); err != nil {
		t.Fatal(err)
	}
	if rowStatus != "created" || rowSessionID != sessionID {
		t.Fatalf("status=%q stripe_session_id=%q, want created/%s", rowStatus, rowSessionID, sessionID)
	}
}

func TestCreditPaidStripeCheckoutCreditsPersistedIntentWithoutSessionID(t *testing.T) {
	db := openManagedServiceTestDB(t)
	ctx := context.Background()

	userID := "usr_test_" + randomToken(8)
	intentID := "sci_test_" + randomToken(8)
	sessionID := "cs_test_" + randomToken(8)
	t.Cleanup(func() {
		_, _ = db.Exec(context.Background(), `DELETE FROM credit_ledger WHERE source_id=$1`, sessionID)
		_, _ = db.Exec(context.Background(), `DELETE FROM stripe_checkout_sessions WHERE id=$1`, intentID)
		_, _ = db.Exec(context.Background(), `DELETE FROM users WHERE id=$1`, userID)
	})
	if _, err := db.Exec(ctx, `INSERT INTO users (id, auth_subject, email) VALUES ($1, $2, $3)`, userID, "auth_"+userID, userID+"@example.test"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, `
INSERT INTO stripe_checkout_sessions (id, checkout_intent_id, user_id, amount_cents, currency, status)
VALUES ($1, $1, $2, 1200, 'usd', 'pending')
`, intentID, userID); err != nil {
		t.Fatal(err)
	}
	session := stripe.CheckoutSession{
		ID:            sessionID,
		PaymentStatus: stripe.CheckoutSessionPaymentStatusPaid,
		AmountTotal:   1200,
		Currency:      stripe.CurrencyUSD,
		Metadata: map[string]string{
			"billing_kind":       "dari_docs_credit_purchase",
			"checkout_intent_id": intentID,
		},
	}
	credited, err := (&Server{db: db}).creditPaidStripeCheckout(ctx, session)
	if err != nil {
		t.Fatal(err)
	}
	if !credited {
		t.Fatal("first webhook should credit the checkout intent")
	}
	credited, err = (&Server{db: db}).creditPaidStripeCheckout(ctx, session)
	if err != nil {
		t.Fatal(err)
	}
	if credited {
		t.Fatal("duplicate webhook should not credit again")
	}
	var rowStatus, rowSessionID string
	if err := db.QueryRow(ctx, `SELECT status, coalesce(stripe_session_id,'') FROM stripe_checkout_sessions WHERE id=$1`, intentID).Scan(&rowStatus, &rowSessionID); err != nil {
		t.Fatal(err)
	}
	if rowStatus != "credited" || rowSessionID != sessionID {
		t.Fatalf("status=%q stripe_session_id=%q, want credited/%s", rowStatus, rowSessionID, sessionID)
	}
	balance, err := (&Server{db: db}).balanceCents(ctx, userID)
	if err != nil {
		t.Fatal(err)
	}
	if balance != 1200 {
		t.Fatalf("balance = %d, want 1200", balance)
	}
}

func TestPreflightRunAllowsConfiguredActiveRunLimit(t *testing.T) {
	dsn := os.Getenv("MANAGEDSERVICE_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set MANAGEDSERVICE_TEST_DATABASE_URL to run managed service database integration tests")
	}
	ctx := context.Background()
	db, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	if err := runMigrations(ctx, dsn); err != nil {
		t.Fatal(err)
	}
	s := &Server{db: db, cfg: Config{MaxActiveRunsPerUser: 3}}
	userID := "usr_test_" + randomToken(8)
	agentSetID := "mags_test_" + randomToken(8)
	t.Cleanup(func() {
		_, _ = db.Exec(context.Background(), `DELETE FROM runs WHERE user_id=$1`, userID)
		_, _ = db.Exec(context.Background(), `DELETE FROM credit_ledger WHERE user_id=$1`, userID)
		_, _ = db.Exec(context.Background(), `DELETE FROM agent_sets WHERE id=$1`, agentSetID)
		_, _ = db.Exec(context.Background(), `DELETE FROM users WHERE id=$1`, userID)
	})
	if _, err := db.Exec(ctx, `INSERT INTO users (id, auth_subject, email) VALUES ($1, $2, $3)`, userID, "auth_"+userID, userID+"@example.test"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, `
INSERT INTO agent_sets (id, user_id, tester_agent_id, editor_agent_id, tester_version_id, editor_version_id, tester_sha256, editor_sha256)
VALUES ($1, $2, 'agt_tester', 'agt_editor', 'ver_tester', 'ver_editor', 'tester_sha', 'editor_sha')
`, agentSetID, userID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, `INSERT INTO credit_ledger (id, user_id, amount_cents, kind, source_id) VALUES ($1, $2, 500, 'test_credit', $3)`, "cred_"+randomToken(8), userID, "src_"+randomToken(8)); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if _, err := db.Exec(ctx, `
INSERT INTO runs (id, user_id, mode, status, tasks, agent_set_id, bundle_sha256, bundle_files)
VALUES ($1, $2, 'check', $3, '["task"]'::jsonb, $4, 'sha', 1)
`, "run_"+randomToken(8), userID, statusRunning, agentSetID); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.preflightRun(ctx, userID, agentSetID, 75); err != nil {
		t.Fatalf("preflight with 2 active runs returned error: %v", err)
	}
	if _, err := db.Exec(ctx, `
INSERT INTO runs (id, user_id, mode, status, tasks, agent_set_id, bundle_sha256, bundle_files)
VALUES ($1, $2, 'check', $3, '["task"]'::jsonb, $4, 'sha', 1)
`, "run_"+randomToken(8), userID, statusQueued, agentSetID); err != nil {
		t.Fatal(err)
	}
	err = s.preflightRun(ctx, userID, agentSetID, 75)
	var activeErr *activeRunLimitError
	if !errors.As(err, &activeErr) || activeErr.Limit != 3 {
		t.Fatalf("preflight error = %v, want active run limit 3", err)
	}
}

func TestFetchDariUserInfoForwardsBearer(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/auth/userinfo" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer supabase-token" {
			t.Fatalf("authorization = %q", got)
		}
		_ = json.NewEncoder(w).Encode(dariUserInfo{
			AuthSubject: "sup_user_123",
			Email:       "user@example.test",
			DisplayName: "User",
		})
	}))
	defer upstream.Close()

	got, err := (&Server{cfg: Config{DariAPIBaseURL: upstream.URL}}).fetchDariUserInfo(context.Background(), "supabase-token")
	if err != nil {
		t.Fatal(err)
	}
	if got.AuthSubject != "sup_user_123" || got.Email != "user@example.test" || got.DisplayName != "User" {
		t.Fatalf("userinfo = %#v", got)
	}
}

func TestDariAuthExchangeRequiresBearer(t *testing.T) {
	s := &Server{}
	req := httptest.NewRequest(http.MethodPost, "/v1/auth/dari/exchange", nil)
	rec := httptest.NewRecorder()
	s.handleDariAuthExchange(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestNormalizeScopesTrimsAndDeduplicates(t *testing.T) {
	got := normalizeScopes([]string{" managed:read ", "managed:check", "managed:read", ""})
	want := []string{"managed:read", "managed:check"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("scopes = %#v, want %#v", got, want)
	}
}

func TestInteractiveTokenHasAllScopes(t *testing.T) {
	u := user{TokenKind: tokenKindInteractive}
	for _, scope := range allManagedScopes {
		if !u.hasScope(scope) {
			t.Fatalf("interactive token missing scope %s", scope)
		}
	}
}

func TestAutomationTokenRequiresExplicitScope(t *testing.T) {
	u := user{TokenKind: tokenKindAutomation, TokenScopes: []string{scopeManagedRead}}
	if !u.hasScope(scopeManagedRead) {
		t.Fatal("automation token should have explicit read scope")
	}
	if u.hasScope(scopeManagedBilling) {
		t.Fatal("automation token should not inherit billing scope")
	}
}

func TestCreateAuthTokenCannotGrantScopesCallerDoesNotHave(t *testing.T) {
	body := strings.NewReader(`{"name":"github-actions","scopes":["managed:read","managed:billing"]}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/auth/tokens", body)
	rec := httptest.NewRecorder()

	s := &Server{}
	s.handleCreateAuthToken(rec, req, user{
		ID:          "usr_test",
		TokenKind:   tokenKindAutomation,
		TokenScopes: []string{scopeManagedRead, scopeManagedTokens},
	})

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusForbidden, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "token cannot grant scope managed:billing") {
		t.Fatalf("body = %s", rec.Body.String())
	}
}

func TestCreateAuthTokenDefaultScopesMustBeGrantableByCaller(t *testing.T) {
	body := strings.NewReader(`{"name":"github-actions"}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/auth/tokens", body)
	rec := httptest.NewRecorder()

	s := &Server{}
	s.handleCreateAuthToken(rec, req, user{
		ID:          "usr_test",
		TokenKind:   tokenKindAutomation,
		TokenScopes: []string{scopeManagedRead, scopeManagedTokens},
	})

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusForbidden, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "token cannot grant scope managed:check") {
		t.Fatalf("body = %s", rec.Body.String())
	}
}

func TestLogoutAllRevokesOnlyCurrentUserTokens(t *testing.T) {
	dsn := os.Getenv("MANAGEDSERVICE_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set MANAGEDSERVICE_TEST_DATABASE_URL to run managed service database integration tests")
	}
	ctx := context.Background()
	db, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	if err := runMigrations(ctx, dsn); err != nil {
		t.Fatal(err)
	}
	s := &Server{db: db}
	userID := "usr_test_" + randomToken(8)
	otherUserID := "usr_test_" + randomToken(8)
	token := "managed_token_" + randomToken(8)
	token2 := "managed_token_" + randomToken(8)
	otherToken := "managed_token_" + randomToken(8)
	t.Cleanup(func() {
		_, _ = db.Exec(context.Background(), `DELETE FROM api_tokens WHERE user_id IN ($1, $2)`, userID, otherUserID)
		_, _ = db.Exec(context.Background(), `DELETE FROM users WHERE id IN ($1, $2)`, userID, otherUserID)
	})
	if _, err := db.Exec(ctx, `INSERT INTO users (id, auth_subject, email) VALUES ($1, $2, $3), ($4, $5, $6)`,
		userID, "auth_"+userID, userID+"@example.test",
		otherUserID, "auth_"+otherUserID, otherUserID+"@example.test",
	); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, `
INSERT INTO api_tokens (id, user_id, token_hash)
VALUES ($1, $2, $3), ($4, $2, $5), ($6, $7, $8)
`, "tok_"+randomToken(8), userID, sha256Hex(token), "tok_"+randomToken(8), sha256Hex(token2), "tok_"+randomToken(8), otherUserID, sha256Hex(otherToken)); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/auth/logout-all", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	s.routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 body=%s", rec.Code, rec.Body.String())
	}
	var currentRevoked, otherRevoked int
	if err := db.QueryRow(ctx, `SELECT count(*) FROM api_tokens WHERE user_id=$1 AND revoked_at IS NOT NULL`, userID).Scan(&currentRevoked); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(ctx, `SELECT count(*) FROM api_tokens WHERE user_id=$1 AND revoked_at IS NOT NULL`, otherUserID).Scan(&otherRevoked); err != nil {
		t.Fatal(err)
	}
	if currentRevoked != 2 || otherRevoked != 0 {
		t.Fatalf("revoked current=%d other=%d, want current=2 other=0", currentRevoked, otherRevoked)
	}
}

func TestActiveRunLimitErrorMessageUsesLimit(t *testing.T) {
	err := (&activeRunLimitError{Limit: 3}).Error()
	want := "you already have 3 active managed runs; wait for one to finish before starting another run"
	if err != want {
		t.Fatalf("activeRunLimitError = %q, want %q", err, want)
	}
}

func TestRunStatusResponseDoesNotExposeEditorSessionID(t *testing.T) {
	b, err := json.Marshal(runStatusResponse{
		ID:                   "run_test",
		Mode:                 "optimize",
		Status:               statusCompleted,
		UpdatedDocsAvailable: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), "editor_session_id") {
		t.Fatalf("run status response exposed editor_session_id: %s", string(b))
	}
}

func TestHandleRunsReturnsExistingRunForDuplicateRunRequestID(t *testing.T) {
	db := openManagedServiceTestDB(t)
	ctx := context.Background()

	userID := "usr_test_" + randomToken(8)
	runID := "run_test_" + randomToken(8)
	runRequestID := "mrr_test_" + randomToken(8)
	t.Cleanup(func() {
		_, _ = db.Exec(context.Background(), `DELETE FROM runs WHERE id=$1`, runID)
		_, _ = db.Exec(context.Background(), `DELETE FROM users WHERE id=$1`, userID)
	})
	if _, err := db.Exec(ctx, `INSERT INTO users (id, auth_subject, email) VALUES ($1, $2, $3)`, userID, "auth_"+userID, userID+"@example.test"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, `
INSERT INTO runs (id, user_id, run_request_id, mode, status, tasks, bundle_sha256, bundle_files)
VALUES ($1, $2, $3, 'check', $4, '["task"]'::jsonb, 'sha', 1)
`, runID, userID, runRequestID, statusQueued); err != nil {
		t.Fatal(err)
	}

	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	if err := mw.WriteField("agent_set_id", "mags_missing"); err != nil {
		t.Fatal(err)
	}
	if err := mw.WriteField("run_request_id", runRequestID); err != nil {
		t.Fatal(err)
	}
	if err := mw.WriteField("mode", "check"); err != nil {
		t.Fatal(err)
	}
	if err := mw.WriteField("tasks_json", `["check the docs"]`); err != nil {
		t.Fatal(err)
	}
	if _, err := mw.CreateFormFile("bundle", "input-docs-bundle.tar.gz"); err != nil {
		t.Fatal(err)
	}
	if err := mw.Close(); err != nil {
		t.Fatal(err)
	}

	s := &Server{db: db, cfg: Config{MaxBundleBytes: 1 << 20, MaxTasksPerRun: 3, MaxTaskBytes: 10000}}
	req := httptest.NewRequest(http.MethodPost, "/v1/runs", &body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	rec := httptest.NewRecorder()

	s.handleRuns(rec, req, user{ID: userID, TokenKind: tokenKindInteractive})

	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusAccepted, rec.Body.String())
	}
	var got map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got["run_id"] != runID || got["status"] != statusQueued {
		t.Fatalf("response = %#v", got)
	}
}

func TestValidateManagedTasksRejectsOversizedTask(t *testing.T) {
	err := validateManagedTasks([]string{"short", strings.Repeat("x", 11)}, 10)
	if err == nil || !strings.Contains(err.Error(), "task 2 exceeds managed task text limit of 10 bytes") {
		t.Fatalf("err = %v, want task size error", err)
	}
}

func TestHandleRunsRejectsOversizedTaskText(t *testing.T) {
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	if err := mw.WriteField("mode", "check"); err != nil {
		t.Fatal(err)
	}
	if err := mw.WriteField("tasks_json", `["`+strings.Repeat("x", 11)+`"]`); err != nil {
		t.Fatal(err)
	}
	if err := mw.Close(); err != nil {
		t.Fatal(err)
	}
	s := &Server{cfg: Config{MaxBundleBytes: 1 << 20, MaxTasksPerRun: 3, MaxTaskBytes: 10}}
	req := httptest.NewRequest(http.MethodPost, "/v1/runs", &body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	rec := httptest.NewRecorder()
	s.handleRuns(rec, req, user{ID: "usr_test"})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "task 1 exceeds managed task text limit") {
		t.Fatalf("body = %s", rec.Body.String())
	}
}

func TestHandleRunsReturns413ForOversizedMultipartBody(t *testing.T) {
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	if err := mw.WriteField("mode", "check"); err != nil {
		t.Fatal(err)
	}
	if err := mw.WriteField("tasks_json", `["check the docs"]`); err != nil {
		t.Fatal(err)
	}
	part, err := mw.CreateFormFile("bundle", "input-docs-bundle.tar.gz")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write(bytes.Repeat([]byte("x"), 2<<20)); err != nil {
		t.Fatal(err)
	}
	if err := mw.Close(); err != nil {
		t.Fatal(err)
	}

	s := &Server{cfg: Config{MaxBundleBytes: 1}}
	req := httptest.NewRequest(http.MethodPost, "/v1/runs", &body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	rec := httptest.NewRecorder()

	s.handleRuns(rec, req, user{ID: "usr_test"})

	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusRequestEntityTooLarge, rec.Body.String())
	}
	if got := rec.Body.String(); !strings.Contains(got, "bundle exceeds managed size limit") || strings.Contains(got, "http: request body too large") {
		t.Fatalf("body = %s", got)
	}
}

func TestHandleRunsRequiresFieldsBeforeBundle(t *testing.T) {
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	part, err := mw.CreateFormFile("bundle", "input-docs-bundle.tar.gz")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write([]byte("not a real bundle")); err != nil {
		t.Fatal(err)
	}
	if err := mw.WriteField("mode", "check"); err != nil {
		t.Fatal(err)
	}
	if err := mw.WriteField("tasks_json", `["check the docs"]`); err != nil {
		t.Fatal(err)
	}
	if err := mw.Close(); err != nil {
		t.Fatal(err)
	}

	s := &Server{cfg: Config{MaxBundleBytes: 1 << 20, MaxTasksPerRun: 3, MaxTaskBytes: 10000}}
	req := httptest.NewRequest(http.MethodPost, "/v1/runs", &body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	rec := httptest.NewRecorder()

	s.handleRuns(rec, req, user{ID: "usr_test"})

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "agent_set_id, mode, and tasks_json must be sent before bundle") {
		t.Fatalf("body = %s", rec.Body.String())
	}
}

func stripeSignatureHeader(payload []byte, secret string) string {
	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(timestamp + "."))
	mac.Write(payload)
	return "t=" + timestamp + ",v1=" + hex.EncodeToString(mac.Sum(nil))
}

func setRequiredManagedConfigEnv(t *testing.T) {
	t.Helper()
	t.Setenv("DATABASE_URL", "postgres://example.invalid/dari_docs")
	t.Setenv("DARI_API_KEY", "dari_test")
	t.Setenv("DARI_DOCS_SECRET_ENCRYPTION_KEY", testManagedSecretEncryptionKey())
}

func testManagedSecretEncryptionKey() string {
	return base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{7}, 32))
}

func clearManagedConfigOptionalEnv(t *testing.T) {
	t.Helper()
	for _, key := range []string{
		"ADDR",
		"PORT",
		"PUBLIC_BASE_URL",
		"DARI_API_BASE_URL",
		"STRIPE_SECRET_KEY",
		"STRIPE_WEBHOOK_SECRET",
	} {
		t.Setenv(key, "")
	}
}

func openManagedServiceTestDB(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("MANAGEDSERVICE_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set MANAGEDSERVICE_TEST_DATABASE_URL to run managed service database integration tests")
	}
	ctx := context.Background()
	db, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(db.Close)
	if err := runMigrations(ctx, dsn); err != nil {
		t.Fatal(err)
	}
	return db
}

func insertStartingRunSession(t *testing.T, db *pgxpool.Pool) (userID, runID, sessionID string) {
	t.Helper()
	ctx := context.Background()
	userID = "usr_test_" + randomToken(8)
	runID = "run_test_" + randomToken(8)
	agentSetID := "mags_test_" + randomToken(8)
	sessionID = "sess_test_" + randomToken(8)
	if _, err := db.Exec(ctx, `INSERT INTO users (id, auth_subject, email) VALUES ($1, $2, $3)`, userID, "auth_"+userID, userID+"@example.test"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, `
INSERT INTO agent_sets (id, user_id, tester_agent_id, editor_agent_id, tester_version_id, editor_version_id, tester_sha256, editor_sha256)
VALUES ($1, $2, 'agt_tester', 'agt_editor', 'ver_tester', 'ver_editor', 'tester_sha', 'editor_sha')
`, agentSetID, userID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, `
INSERT INTO runs (id, user_id, mode, status, tasks, agent_set_id, bundle_file_id, bundle_sha256, bundle_files)
VALUES ($1, $2, 'check', $3, '["task"]'::jsonb, $4, 'file_test', 'sha', 1)
`, runID, userID, statusStarting, agentSetID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, `
INSERT INTO run_sessions (session_id, run_id, kind, task_index, status, version_id, created_at)
VALUES ($1, $2, 'tester', 1, $3, 'ver_tester', now() - interval '10 minutes')
`, sessionID, runID, statusStarting); err != nil {
		t.Fatal(err)
	}
	return userID, runID, sessionID
}

func cleanupRunSessionTestRows(userID, runID string, db *pgxpool.Pool) {
	ctx := context.Background()
	_, _ = db.Exec(ctx, `DELETE FROM credit_ledger WHERE run_id=$1 OR user_id=$2`, runID, userID)
	_, _ = db.Exec(ctx, `DELETE FROM run_sessions WHERE run_id=$1`, runID)
	_, _ = db.Exec(ctx, `DELETE FROM runs WHERE id=$1`, runID)
	_, _ = db.Exec(ctx, `DELETE FROM agent_sets WHERE user_id=$1`, userID)
	_, _ = db.Exec(ctx, `DELETE FROM users WHERE id=$1`, userID)
}
