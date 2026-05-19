package managedservice

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/mupt-ai/dari-docs/internal/bundle"
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
	if cfg.ManagedTesterAgentID != "agt_tester" || cfg.ManagedEditorAgentID != "agt_editor" || cfg.ReleaseAdminToken != "release-admin-token" {
		t.Fatalf("managed agent config = %q/%q %q/%q", cfg.ManagedTesterAgentID, cfg.ManagedTesterVersionID, cfg.ManagedEditorAgentID, cfg.ManagedEditorVersionID)
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
	if cfg.HTTPReadTimeout != time.Duration(managedHTTPReadTimeoutSeconds)*time.Second {
		t.Fatalf("HTTPReadTimeout = %s, want %ds", cfg.HTTPReadTimeout, managedHTTPReadTimeoutSeconds)
	}
}

func TestConfigFromEnvRequiresRuntimeSecretEncryptionKey(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://example.invalid/dari_docs")
	t.Setenv("DARI_API_KEY", "dari_test")
	t.Setenv("DARI_DOCS_RELEASE_ADMIN_TOKEN", "release-admin-token")
	setRequiredManagedAgentEnv(t)
	_, err := ConfigFromEnv()
	if err == nil || !strings.Contains(err.Error(), "DARI_DOCS_SECRET_ENCRYPTION_KEY is required") {
		t.Fatalf("error = %v", err)
	}
}

func TestConfigFromEnvRequiresManagedHostedAgents(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://example.invalid/dari_docs")
	t.Setenv("DARI_API_KEY", "dari_test")
	t.Setenv("DARI_DOCS_SECRET_ENCRYPTION_KEY", testManagedSecretEncryptionKey())
	t.Setenv("DARI_DOCS_RELEASE_ADMIN_TOKEN", "release-admin-token")
	_, err := ConfigFromEnv()
	if err == nil || !strings.Contains(err.Error(), "MANAGED_TESTER_AGENT_ID is required") {
		t.Fatalf("error = %v", err)
	}
}

func TestConfigFromEnvRequiresReleaseAdminToken(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://example.invalid/dari_docs")
	t.Setenv("DARI_API_KEY", "dari_test")
	t.Setenv("DARI_DOCS_SECRET_ENCRYPTION_KEY", testManagedSecretEncryptionKey())
	setRequiredManagedAgentEnv(t)
	_, err := ConfigFromEnv()
	if err == nil || !strings.Contains(err.Error(), "DARI_DOCS_RELEASE_ADMIN_TOKEN is required") {
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

func TestManagedAgentSetTablesAreDroppedByMigration(t *testing.T) {
	data, err := migrationFS.ReadFile("migrations/0005_drop_managed_agent_sets.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := string(data)
	for _, want := range []string{
		"DROP COLUMN IF EXISTS agent_set_id",
		"ALTER COLUMN tester_agent_id SET NOT NULL",
		"ALTER COLUMN tester_version_id SET NOT NULL",
		"ALTER COLUMN editor_agent_id SET NOT NULL",
		"ALTER COLUMN editor_version_id SET NOT NULL",
		"DROP TABLE IF EXISTS agent_set_deploys",
		"DROP SEQUENCE IF EXISTS agent_set_deploy_sequence",
		"DROP TABLE IF EXISTS agent_sets",
	} {
		if !strings.Contains(sql, want) {
			t.Fatalf("drop migration is missing %q", want)
		}
	}
}

func TestManagedAgentReleaseMigrationCreatesActiveReleaseTable(t *testing.T) {
	data, err := migrationFS.ReadFile("migrations/0006_managed_agent_releases.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := string(data)
	for _, want := range []string{
		"CREATE TABLE managed_agent_releases",
		"tester_agent_id TEXT NOT NULL",
		"tester_version_id TEXT NOT NULL",
		"editor_agent_id TEXT NOT NULL",
		"editor_version_id TEXT NOT NULL",
		"CREATE UNIQUE INDEX idx_managed_agent_releases_active",
	} {
		if !strings.Contains(sql, want) {
			t.Fatalf("managed release migration is missing %q", want)
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

func TestHandleBillingConfigReturnsCheckoutBounds(t *testing.T) {
	s := &Server{}
	req := httptest.NewRequest(http.MethodGet, "/v1/billing/config", nil)
	rec := httptest.NewRecorder()

	s.handleBillingConfig(rec, req, user{ID: "usr_test", Email: "user@example.test"})

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	var got map[string]int64
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	for key, want := range map[string]int64{
		"min_checkout_cents":     stripeCheckoutMinCents,
		"default_checkout_cents": stripeCheckoutDefaultCents,
		"max_checkout_cents":     stripeCheckoutMaxCents,
	} {
		if got[key] != want {
			t.Fatalf("%s = %d, want %d; body=%s", key, got[key], want, rec.Body.String())
		}
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
	var got struct {
		FreeCreditCents            int64    `json:"free_credit_cents"`
		TesterSessionReserveCents  int64    `json:"tester_session_reserve_cents"`
		EditorSessionReserveCents  int64    `json:"editor_session_reserve_cents"`
		ServiceFeeCents            int64    `json:"service_fee_cents"`
		MaxTasksPerRun             int64    `json:"max_tasks_per_run"`
		MaxTaskBytes               int64    `json:"max_task_bytes"`
		MaxActiveRunsPerUser       int64    `json:"max_active_runs_per_user"`
		MaxBundleBytes             int64    `json:"max_bundle_bytes"`
		BundleMaxUncompressedBytes int64    `json:"bundle_max_uncompressed_bytes"`
		BundleMaxFileBytes         int64    `json:"bundle_max_file_bytes"`
		DefaultLLMID               string   `json:"default_llm_id"`
		DefaultFeedbackLLMIDs      []string `json:"default_feedback_llm_ids"`
		AllowedLLMIDs              []string `json:"allowed_llm_ids"`
	}
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
		values := map[string]int64{
			"free_credit_cents":             got.FreeCreditCents,
			"tester_session_reserve_cents":  got.TesterSessionReserveCents,
			"editor_session_reserve_cents":  got.EditorSessionReserveCents,
			"service_fee_cents":             got.ServiceFeeCents,
			"max_tasks_per_run":             got.MaxTasksPerRun,
			"max_task_bytes":                got.MaxTaskBytes,
			"max_active_runs_per_user":      got.MaxActiveRunsPerUser,
			"max_bundle_bytes":              got.MaxBundleBytes,
			"bundle_max_uncompressed_bytes": got.BundleMaxUncompressedBytes,
			"bundle_max_file_bytes":         got.BundleMaxFileBytes,
		}
		if values[key] != want {
			t.Fatalf("%s = %d, want %d; body=%s", key, values[key], want, rec.Body.String())
		}
	}
	if got.DefaultLLMID != defaultManagedEditorLLMID() {
		t.Fatalf("default_llm_id = %q, want %q", got.DefaultLLMID, defaultManagedEditorLLMID())
	}
	if strings.Join(got.DefaultFeedbackLLMIDs, ",") != "dumb-claude,medium-claude,smart-claude" {
		t.Fatalf("default_feedback_llm_ids = %#v", got.DefaultFeedbackLLMIDs)
	}
	if strings.Join(got.AllowedLLMIDs, ",") != "dumb-claude,medium-claude,smart-claude,dumb-gpt,medium-gpt,smart-gpt" {
		t.Fatalf("allowed_llm_ids = %#v", got.AllowedLLMIDs)
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
	t.Cleanup(func() {
		_, _ = db.Exec(context.Background(), `DELETE FROM runs WHERE user_id=$1`, userID)
		_, _ = db.Exec(context.Background(), `DELETE FROM credit_ledger WHERE user_id=$1`, userID)
		_, _ = db.Exec(context.Background(), `DELETE FROM users WHERE id=$1`, userID)
	})
	if _, err := db.Exec(ctx, `INSERT INTO users (id, auth_subject, email) VALUES ($1, $2, $3)`, userID, "auth_"+userID, userID+"@example.test"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, `INSERT INTO credit_ledger (id, user_id, amount_cents, kind, source_id) VALUES ($1, $2, 500, 'test_credit', $3)`, "cred_"+randomToken(8), userID, "src_"+randomToken(8)); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if _, err := db.Exec(ctx, `
INSERT INTO runs (id, user_id, mode, status, tasks, tester_agent_id, tester_version_id, editor_agent_id, editor_version_id, bundle_sha256, bundle_files)
VALUES ($1, $2, 'check', $3, '["task"]'::jsonb, 'agt_tester', 'ver_tester', 'agt_editor', 'ver_editor', 'sha', 1)
`, "run_"+randomToken(8), userID, statusRunning); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.preflightRun(ctx, userID, 75); err != nil {
		t.Fatalf("preflight with 2 active runs returned error: %v", err)
	}
	if _, err := db.Exec(ctx, `
INSERT INTO runs (id, user_id, mode, status, tasks, tester_agent_id, tester_version_id, editor_agent_id, editor_version_id, bundle_sha256, bundle_files)
VALUES ($1, $2, 'check', $3, '["task"]'::jsonb, 'agt_tester', 'ver_tester', 'agt_editor', 'ver_editor', 'sha', 1)
`, "run_"+randomToken(8), userID, statusQueued); err != nil {
		t.Fatal(err)
	}
	err = s.preflightRun(ctx, userID, 75)
	var activeErr *activeRunLimitError
	if !errors.As(err, &activeErr) || activeErr.Limit != 3 {
		t.Fatalf("preflight error = %v, want active run limit 3", err)
	}
}

func TestReserveRunStoresConfiguredHostedAgents(t *testing.T) {
	db := openManagedServiceTestDB(t)
	ctx := context.Background()

	s := &Server{db: db, cfg: testManagedHostedAgentConfig()}
	userID := "usr_test_" + randomToken(8)
	runID := "run_test_" + randomToken(8)
	t.Cleanup(func() {
		_, _ = db.Exec(context.Background(), `DELETE FROM credit_ledger WHERE run_id=$1 OR user_id=$2`, runID, userID)
		_, _ = db.Exec(context.Background(), `DELETE FROM runs WHERE id=$1`, runID)
		_, _ = db.Exec(context.Background(), `DELETE FROM users WHERE id=$1`, userID)
	})
	if _, err := db.Exec(ctx, `INSERT INTO users (id, auth_subject, email) VALUES ($1, $2, $3)`, userID, "auth_"+userID, userID+"@example.test"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, `INSERT INTO credit_ledger (id, user_id, amount_cents, kind, source_id) VALUES ($1, $2, 500, 'test_credit', $3)`, "cred_"+randomToken(8), userID, "src_"+randomToken(8)); err != nil {
		t.Fatal(err)
	}

	result := bundle.Result{
		SHA256: "bundle_sha",
		Manifest: bundle.Manifest{Files: []bundle.FileRecord{
			{Path: "README.md", SizeBytes: 12, SHA256: "file_sha"},
		}},
	}
	if err := s.reserveRun(ctx, userID, runID, "check", []byte(`["task"]`), []byte(`["dumb-claude","smart-claude"]`), "smart-claude", result, 150, false, []byte(`[]`), nil, nil); err != nil {
		t.Fatal(err)
	}

	var testerAgentID, testerVersionID, editorAgentID, editorVersionID string
	var testerLLMIDsJSON []byte
	var editorLLMID string
	if err := db.QueryRow(ctx, `
SELECT tester_agent_id, tester_version_id, editor_agent_id, editor_version_id, tester_llm_ids, editor_llm_id
FROM runs WHERE id=$1
`, runID).Scan(&testerAgentID, &testerVersionID, &editorAgentID, &editorVersionID, &testerLLMIDsJSON, &editorLLMID); err != nil {
		t.Fatal(err)
	}
	if testerAgentID != "agt_tester" || testerVersionID != "ver_tester" || editorAgentID != "agt_editor" || editorVersionID != "ver_editor" {
		t.Fatalf("agent config = tester:%q/%q editor:%q/%q", testerAgentID, testerVersionID, editorAgentID, editorVersionID)
	}
	var testerLLMIDs []string
	if err := json.Unmarshal(testerLLMIDsJSON, &testerLLMIDs); err != nil {
		t.Fatal(err)
	}
	if strings.Join(testerLLMIDs, ",") != "dumb-claude,smart-claude" {
		t.Fatalf("tester_llm_ids = %#v", testerLLMIDs)
	}
	if editorLLMID != "smart-claude" {
		t.Fatalf("editor_llm_id = %q", editorLLMID)
	}
}

func TestReserveRunStoresActiveManagedAgentRelease(t *testing.T) {
	db := openManagedServiceTestDB(t)
	ctx := context.Background()

	s := &Server{db: db, cfg: testManagedHostedAgentConfig()}
	userID := "usr_test_" + randomToken(8)
	runID := "run_test_" + randomToken(8)
	t.Cleanup(func() {
		_, _ = db.Exec(context.Background(), `DELETE FROM credit_ledger WHERE run_id=$1 OR user_id=$2`, runID, userID)
		_, _ = db.Exec(context.Background(), `DELETE FROM runs WHERE id=$1`, runID)
		_, _ = db.Exec(context.Background(), `DELETE FROM managed_agent_releases WHERE id=$1`, "mar_test_"+runID)
		_, _ = db.Exec(context.Background(), `DELETE FROM users WHERE id=$1`, userID)
	})
	if _, err := db.Exec(ctx, `INSERT INTO users (id, auth_subject, email) VALUES ($1, $2, $3)`, userID, "auth_"+userID, userID+"@example.test"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, `INSERT INTO credit_ledger (id, user_id, amount_cents, kind, source_id) VALUES ($1, $2, 500, 'test_credit', $3)`, "cred_"+randomToken(8), userID, "src_"+randomToken(8)); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, `UPDATE managed_agent_releases SET active=false WHERE active`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, `
INSERT INTO managed_agent_releases (id, tester_agent_id, tester_version_id, editor_agent_id, editor_version_id, active, source)
VALUES ($1, 'agt_tester', 'ver_active_tester', 'agt_editor', 'ver_active_editor', true, 'test')
`, "mar_test_"+runID); err != nil {
		t.Fatal(err)
	}

	result := bundle.Result{
		SHA256: "bundle_sha",
		Manifest: bundle.Manifest{Files: []bundle.FileRecord{
			{Path: "README.md", SizeBytes: 12, SHA256: "file_sha"},
		}},
	}
	if err := s.reserveRun(ctx, userID, runID, "check", []byte(`["task"]`), []byte(`["medium-claude"]`), "medium-claude", result, 75, false, []byte(`[]`), nil, nil); err != nil {
		t.Fatal(err)
	}

	var testerVersionID, editorVersionID string
	if err := db.QueryRow(ctx, `
SELECT tester_version_id, editor_version_id FROM runs WHERE id=$1
`, runID).Scan(&testerVersionID, &editorVersionID); err != nil {
		t.Fatal(err)
	}
	if testerVersionID != "ver_active_tester" || editorVersionID != "ver_active_editor" {
		t.Fatalf("run versions = tester:%q editor:%q", testerVersionID, editorVersionID)
	}
}

func TestActivateManagedAgentReleasePreservesOmittedSide(t *testing.T) {
	db := openManagedServiceTestDB(t)
	ctx := context.Background()
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/agents/agt_tester/versions/ver_tester_new":
			writeJSON(w, http.StatusOK, map[string]any{
				"agent":   map[string]any{"id": "agt_tester", "active_version_id": "ver_tester_new"},
				"version": map[string]any{"id": "ver_tester_new", "agent_id": "agt_tester"},
			})
		case "/v1/agents/agt_editor/versions/ver_editor_old":
			writeJSON(w, http.StatusOK, map[string]any{
				"agent":   map[string]any{"id": "agt_editor", "active_version_id": "ver_editor_old"},
				"version": map[string]any{"id": "ver_editor_old", "agent_id": "agt_editor"},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer upstream.Close()

	s := &Server{db: db, cfg: Config{ManagedTesterAgentID: "agt_tester", ManagedEditorAgentID: "agt_editor"}, dari: dari.New(upstream.URL, "dari_test")}
	releaseID := "mar_test_" + randomToken(8)
	t.Cleanup(func() {
		_, _ = db.Exec(context.Background(), `DELETE FROM managed_agent_releases WHERE id=$1 OR source=$2`, releaseID, "github_actions")
	})
	if _, err := db.Exec(ctx, `UPDATE managed_agent_releases SET active=false WHERE active`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, `
INSERT INTO managed_agent_releases (id, tester_agent_id, tester_version_id, editor_agent_id, editor_version_id, active, source)
VALUES ($1, 'agt_tester', 'ver_tester_old', 'agt_editor', 'ver_editor_old', true, 'test')
`, releaseID); err != nil {
		t.Fatal(err)
	}

	release, err := s.activateManagedAgentRelease(ctx, activateManagedAgentReleaseRequest{
		TesterVersionID: "ver_tester_new",
		Source:          "github_actions",
	})
	if err != nil {
		t.Fatal(err)
	}
	if release.TesterVersionID != "ver_tester_new" || release.EditorVersionID != "ver_editor_old" {
		t.Fatalf("release versions = tester:%q editor:%q", release.TesterVersionID, release.EditorVersionID)
	}
	var activeCount int
	if err := db.QueryRow(ctx, `SELECT count(*) FROM managed_agent_releases WHERE active`).Scan(&activeCount); err != nil {
		t.Fatal(err)
	}
	if activeCount != 1 {
		t.Fatalf("active release count = %d, want 1", activeCount)
	}
}

func TestActivateManagedAgentReleaseRequiresCompleteFirstRelease(t *testing.T) {
	db := openManagedServiceTestDB(t)
	ctx := context.Background()
	s := &Server{db: db, cfg: Config{ManagedTesterAgentID: "agt_tester", ManagedEditorAgentID: "agt_editor"}, dari: dari.New("https://api.example.test", "dari_test")}
	if _, err := db.Exec(ctx, `UPDATE managed_agent_releases SET active=false WHERE active`); err != nil {
		t.Fatal(err)
	}

	_, err := s.activateManagedAgentRelease(ctx, activateManagedAgentReleaseRequest{TesterVersionID: "ver_tester"})
	if !errors.Is(err, errNoActiveManagedAgentRelease) {
		t.Fatalf("error = %v, want errNoActiveManagedAgentRelease", err)
	}
}

func TestActivateManagedAgentReleaseRejectsWrongAgentVersion(t *testing.T) {
	db := openManagedServiceTestDB(t)
	ctx := context.Background()
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{
			"agent":   map[string]any{"id": "agt_other", "active_version_id": "ver_wrong"},
			"version": map[string]any{"id": "ver_wrong", "agent_id": "agt_other"},
		})
	}))
	defer upstream.Close()
	s := &Server{
		db: db,
		cfg: Config{
			ManagedTesterAgentID:   "agt_tester",
			ManagedTesterVersionID: "ver_tester",
			ManagedEditorAgentID:   "agt_editor",
			ManagedEditorVersionID: "ver_editor",
		},
		dari: dari.New(upstream.URL, "dari_test"),
	}

	_, err := s.activateManagedAgentRelease(ctx, activateManagedAgentReleaseRequest{TesterVersionID: "ver_wrong"})
	if err == nil || !strings.Contains(err.Error(), "does not belong") {
		t.Fatalf("error = %v, want wrong-agent validation error", err)
	}
}

func TestActivateManagedAgentReleaseRejectsMissingVersionAsInvalidRelease(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer upstream.Close()
	s := &Server{
		cfg: Config{
			ManagedTesterAgentID:   "agt_tester",
			ManagedTesterVersionID: "ver_tester",
			ManagedEditorAgentID:   "agt_editor",
			ManagedEditorVersionID: "ver_editor",
		},
		dari: dari.New(upstream.URL, "dari_test"),
	}

	err := s.validateManagedAgentVersion(context.Background(), "tester_version_id", "agt_tester", "ver_missing")
	if !errors.Is(err, errInvalidManagedAgentRelease) {
		t.Fatalf("error = %v, want errInvalidManagedAgentRelease", err)
	}
	if !strings.Contains(err.Error(), "was not found") {
		t.Fatalf("error = %v, want missing-version message", err)
	}
}

func TestManagedAgentReleaseAdminAuthRejectsMissingAndWrongToken(t *testing.T) {
	s := &Server{cfg: Config{ReleaseAdminToken: "release-admin-token"}}
	for name, header := range map[string]string{
		"missing": "",
		"wrong":   "Bearer wrong-token",
	} {
		t.Run(name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/v1/admin/managed-agent-release", nil)
			if header != "" {
				req.Header.Set("Authorization", header)
			}
			rec := httptest.NewRecorder()
			s.handleManagedAgentRelease(rec, req)
			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
			}
		})
	}
	if !validBearerToken("Bearer release-admin-token", "release-admin-token") {
		t.Fatal("expected release admin token to validate")
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

func TestBrowserSessionAuthDoesNotCreateAPIToken(t *testing.T) {
	db := openManagedServiceTestDB(t)
	ctx := context.Background()
	authSubject := "sup_browser_" + randomToken(8)
	accessToken := "header.payload.signature"
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/auth/userinfo" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer "+accessToken {
			t.Fatalf("authorization = %q", got)
		}
		_ = json.NewEncoder(w).Encode(dariUserInfo{
			AuthSubject: authSubject,
			Email:       "browser@example.test",
			DisplayName: "Browser User",
		})
	}))
	defer upstream.Close()
	cleanup := func() {
		_, _ = db.Exec(context.Background(), `DELETE FROM credit_ledger WHERE user_id IN (SELECT id FROM users WHERE auth_subject=$1)`, authSubject)
		_, _ = db.Exec(context.Background(), `DELETE FROM api_tokens WHERE user_id IN (SELECT id FROM users WHERE auth_subject=$1)`, authSubject)
		_, _ = db.Exec(context.Background(), `DELETE FROM users WHERE auth_subject=$1`, authSubject)
	}
	cleanup()
	t.Cleanup(cleanup)

	s := &Server{db: db, cfg: Config{DariAPIBaseURL: upstream.URL, FreeGrantCents: 500}}
	req := httptest.NewRequest(http.MethodGet, "/v1/me", nil)
	req.Header.Set("Authorization", "Bearer "+accessToken)
	rec := httptest.NewRecorder()
	s.routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 body=%s", rec.Code, rec.Body.String())
	}
	var body struct {
		Email        string `json:"email"`
		BalanceCents int64  `json:"balance_cents"`
		Token        struct {
			ID     string   `json:"id"`
			Kind   string   `json:"kind"`
			Scopes []string `json:"scopes"`
		} `json:"token"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Email != "browser@example.test" || body.BalanceCents != 500 || body.Token.Kind != tokenKindBrowserSession || body.Token.ID != "" {
		t.Fatalf("body = %#v", body)
	}
	if len(body.Token.Scopes) != len(allManagedScopes) {
		t.Fatalf("browser scopes = %#v, want all managed scopes", body.Token.Scopes)
	}
	var tokenCount int
	if err := db.QueryRow(ctx, `
SELECT count(*)
FROM api_tokens
WHERE user_id IN (SELECT id FROM users WHERE auth_subject=$1)
`, authSubject).Scan(&tokenCount); err != nil {
		t.Fatal(err)
	}
	if tokenCount != 0 {
		t.Fatalf("api token count = %d, want 0", tokenCount)
	}
}

func TestBrowserSessionCanRevokeAllTokens(t *testing.T) {
	db := openManagedServiceTestDB(t)
	ctx := context.Background()
	authSubject := "sup_browser_revoke_" + randomToken(8)
	userID := "usr_browser_revoke_" + randomToken(8)
	email := "browser-revoke-" + randomToken(8) + "@example.test"
	accessToken := "header.payload.signature"
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/auth/userinfo" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer "+accessToken {
			t.Fatalf("authorization = %q", got)
		}
		_ = json.NewEncoder(w).Encode(dariUserInfo{
			AuthSubject: authSubject,
			Email:       email,
			DisplayName: "Browser Revoke",
		})
	}))
	defer upstream.Close()
	cleanup := func() {
		_, _ = db.Exec(context.Background(), `DELETE FROM api_tokens WHERE user_id=$1`, userID)
		_, _ = db.Exec(context.Background(), `DELETE FROM users WHERE id=$1`, userID)
	}
	cleanup()
	t.Cleanup(cleanup)

	if _, err := db.Exec(ctx, `
INSERT INTO users (id, auth_subject, email, free_credit_granted_at)
VALUES ($1, $2, $3, now())
`, userID, authSubject, email); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, `
INSERT INTO api_tokens (id, user_id, token_hash, kind, scopes)
VALUES
  ($1, $3, $4, $5, $7),
  ($2, $3, $6, $5, $7)
`, "tok_browser_revoke_a_"+randomToken(6), "tok_browser_revoke_b_"+randomToken(6), userID, sha256Hex("token-a"), tokenKindAutomation, sha256Hex("token-b"), mustJSON([]string{scopeManagedRead})); err != nil {
		t.Fatal(err)
	}

	s := &Server{db: db, cfg: Config{DariAPIBaseURL: upstream.URL}}
	req := httptest.NewRequest(http.MethodPost, "/v1/auth/logout-all", nil)
	req.Header.Set("Authorization", "Bearer "+accessToken)
	rec := httptest.NewRecorder()
	s.routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 body=%s", rec.Code, rec.Body.String())
	}
	var revoked int
	if err := db.QueryRow(ctx, `SELECT count(*) FROM api_tokens WHERE user_id=$1 AND revoked_at IS NOT NULL`, userID).Scan(&revoked); err != nil {
		t.Fatal(err)
	}
	if revoked != 2 {
		t.Fatalf("revoked tokens = %d, want 2", revoked)
	}
}

func TestDariAuthExchangeStillCreatesInteractiveToken(t *testing.T) {
	db := openManagedServiceTestDB(t)
	authSubject := "sup_cli_" + randomToken(8)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer supabase-token" {
			t.Fatalf("authorization = %q", got)
		}
		_ = json.NewEncoder(w).Encode(dariUserInfo{
			AuthSubject: authSubject,
			Email:       "cli@example.test",
			DisplayName: "CLI User",
		})
	}))
	defer upstream.Close()
	cleanup := func() {
		_, _ = db.Exec(context.Background(), `DELETE FROM credit_ledger WHERE user_id IN (SELECT id FROM users WHERE auth_subject=$1)`, authSubject)
		_, _ = db.Exec(context.Background(), `DELETE FROM api_tokens WHERE user_id IN (SELECT id FROM users WHERE auth_subject=$1)`, authSubject)
		_, _ = db.Exec(context.Background(), `DELETE FROM users WHERE auth_subject=$1`, authSubject)
	}
	cleanup()
	t.Cleanup(cleanup)

	s := &Server{db: db, cfg: Config{DariAPIBaseURL: upstream.URL, FreeGrantCents: 500}}
	req := httptest.NewRequest(http.MethodPost, "/v1/auth/dari/exchange", nil)
	req.Header.Set("Authorization", "Bearer supabase-token")
	rec := httptest.NewRecorder()
	s.routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 body=%s", rec.Code, rec.Body.String())
	}
	var body struct {
		Token string `json:"token"`
		Email string `json:"email"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Token == "" || body.Email != "cli@example.test" {
		t.Fatalf("body = %#v", body)
	}
	var tokenCount int
	if err := db.QueryRow(context.Background(), `
SELECT count(*)
FROM api_tokens
WHERE kind=$2 AND user_id IN (SELECT id FROM users WHERE auth_subject=$1)
`, authSubject, tokenKindInteractive).Scan(&tokenCount); err != nil {
		t.Fatal(err)
	}
	if tokenCount != 1 {
		t.Fatalf("interactive token count = %d, want 1", tokenCount)
	}
}

func TestUserAuthRejectsInvalidNonJWTWithoutCallingUserinfo(t *testing.T) {
	db := openManagedServiceTestDB(t)
	var upstreamHits int
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamHits++
		http.Error(w, "should not be called", http.StatusInternalServerError)
	}))
	defer upstream.Close()

	s := &Server{db: db, cfg: Config{DariAPIBaseURL: upstream.URL}}
	req := httptest.NewRequest(http.MethodGet, "/v1/me", nil)
	req.Header.Set("Authorization", "Bearer not-a-managed-token")
	rec := httptest.NewRecorder()
	s.routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401 body=%s", rec.Code, rec.Body.String())
	}
	if upstreamHits != 0 {
		t.Fatalf("userinfo hits = %d, want 0", upstreamHits)
	}
}

func TestHandleAuthConfigProxiesDariAuthConfig(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/v1/auth/config" {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"supabase_url":             "https://supabase.example.test/",
			"supabase_publishable_key": "publishable",
			"providers":                []string{"google"},
		})
	}))
	defer upstream.Close()

	req := httptest.NewRequest(http.MethodGet, "/v1/auth/config", nil)
	rec := httptest.NewRecorder()
	(&Server{cfg: Config{DariAPIBaseURL: upstream.URL}}).handleAuthConfig(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["supabase_url"] != "https://supabase.example.test" || body["supabase_publishable_key"] != "publishable" {
		t.Fatalf("body = %#v", body)
	}
}

func TestHandleFrontendServesSPAIndex(t *testing.T) {
	dir := t.TempDir()
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(old)
	if err := os.MkdirAll("web/dist/assets", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile("web/dist/index.html", []byte("<html>Dari Docs</html>"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile("web/dist/assets/app.js", []byte("console.log('ok')"), 0o644); err != nil {
		t.Fatal(err)
	}

	s := &Server{}
	for _, path := range []string{"/", "/runs", "/runs/run_123"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		s.handleFrontend(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s status = %d, want %d", path, rec.Code, http.StatusOK)
		}
		if !strings.Contains(rec.Body.String(), "Dari Docs") {
			t.Fatalf("%s body = %q", path, rec.Body.String())
		}
	}

	apiReq := httptest.NewRequest(http.MethodGet, "/v1/unknown", nil)
	apiRec := httptest.NewRecorder()
	s.handleFrontend(apiRec, apiReq)
	if apiRec.Code != http.StatusNotFound || !strings.Contains(apiRec.Body.String(), "not found") {
		t.Fatalf("api fallback status/body = %d %q", apiRec.Code, apiRec.Body.String())
	}

	req := httptest.NewRequest(http.MethodGet, "/assets/app.js", nil)
	rec := httptest.NewRecorder()
	s.handleFrontend(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "console.log") {
		t.Fatalf("asset status/body = %d %q", rec.Code, rec.Body.String())
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

func TestBrowserSessionHasAllScopes(t *testing.T) {
	u := user{TokenKind: tokenKindBrowserSession}
	for _, scope := range allManagedScopes {
		if !u.hasScope(scope) {
			t.Fatalf("browser session missing scope %s", scope)
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

func TestDefaultAutomationScopesSupportManagedCI(t *testing.T) {
	got := make(map[string]bool, len(defaultAutomationScopes))
	for _, scope := range defaultAutomationScopes {
		got[scope] = true
	}
	for _, scope := range []string{scopeManagedRead, scopeManagedCheck, scopeManagedOptimize} {
		if !got[scope] {
			t.Fatalf("default automation scopes = %#v, missing %s", defaultAutomationScopes, scope)
		}
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

func TestParseRunListParams(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/v1/runs?limit=200&sort=cost&direction=asc&cursor="+encodeRunListCursor(25), nil)
	got, err := parseRunListParams(req)
	if err != nil {
		t.Fatal(err)
	}
	if got.Limit != 100 || got.Offset != 25 || got.Sort != "cost" || got.Direction != "asc" {
		t.Fatalf("params = %#v", got)
	}
}

func TestParseRunListParamsRejectsInvalidCursor(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/v1/runs?cursor=not-base64", nil)
	if _, err := parseRunListParams(req); err == nil {
		t.Fatal("expected invalid cursor error")
	}
}

func TestRunListOrderExprWhitelistsSorts(t *testing.T) {
	for _, sort := range []string{"status", "mode", "type", "task", "cost", "created_at", "completed_at", "llms"} {
		if _, ok := runListOrderExpr(sort); !ok {
			t.Fatalf("sort %q was not accepted", sort)
		}
	}
	order, ok := runListOrderExpr("llms")
	if !ok || !strings.Contains(order.Expr, "tester_llm_ids") || len(order.Args) != 0 {
		t.Fatalf("llms sort order = %#v ok=%v", order, ok)
	}
	if _, ok := runListOrderExpr("created_at; drop table runs"); ok {
		t.Fatal("unsafe sort should not be accepted")
	}
}

func TestRunListResponseSerializesEmptyLLMsAsArray(t *testing.T) {
	body, err := json.Marshal(runListResponse{Runs: []runListItem{
		{ID: "run_test", LLMs: []runLLMSummary{}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(body), `"llms":null`) {
		t.Fatalf("body = %s, llms should not be null", body)
	}
	if !strings.Contains(string(body), `"llms":[]`) {
		t.Fatalf("body = %s, want empty llms array", body)
	}
}

func TestRunStatusResponseSerializesEmptyLLMsAndSessionsAsArrays(t *testing.T) {
	body, err := json.Marshal(runStatusResponse{
		ID:       "run_test",
		LLMs:     []runLLMSummary{},
		Sessions: []runSessionSummary{},
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(body), `"llms":null`) {
		t.Fatalf("body = %s, llms should not be null", body)
	}
	if !strings.Contains(string(body), `"llms":[]`) {
		t.Fatalf("body = %s, want empty llms array", body)
	}
	if !strings.Contains(string(body), `"task_count":0`) {
		t.Fatalf("body = %s, want task_count", body)
	}
	if strings.Contains(string(body), `"sessions":null`) {
		t.Fatalf("body = %s, sessions should not be null", body)
	}
	if !strings.Contains(string(body), `"sessions":[]`) {
		t.Fatalf("body = %s, want empty sessions array", body)
	}
}

func TestAuthTokenListSerializesEmptyTokensAsArray(t *testing.T) {
	body, err := json.Marshal(authTokenListResponse{Tokens: []authTokenResponse{}})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(body), `"tokens":null`) {
		t.Fatalf("body = %s, tokens should not be null", body)
	}
	if !strings.Contains(string(body), `"tokens":[]`) {
		t.Fatalf("body = %s, want empty tokens array", body)
	}
}

func TestCreateAuthTokenDefaultScopesIncludeOptimize(t *testing.T) {
	body := strings.NewReader(`{"name":"github-actions"}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/auth/tokens", body)
	rec := httptest.NewRecorder()

	s := &Server{}
	s.handleCreateAuthToken(rec, req, user{
		ID:          "usr_test",
		TokenKind:   tokenKindAutomation,
		TokenScopes: []string{scopeManagedRead, scopeManagedCheck, scopeManagedTokens},
	})

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusForbidden, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "token cannot grant scope managed:optimize") {
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

func TestHandleRunsRejectsUnknownManagedLLM(t *testing.T) {
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	if err := mw.WriteField("mode", "check"); err != nil {
		t.Fatal(err)
	}
	if err := mw.WriteField("tasks_json", `["check the docs"]`); err != nil {
		t.Fatal(err)
	}
	if err := mw.WriteField("feedback_llm_ids_json", `["unknown-model"]`); err != nil {
		t.Fatal(err)
	}
	if err := mw.Close(); err != nil {
		t.Fatal(err)
	}
	s := &Server{cfg: Config{MaxBundleBytes: 1 << 20, MaxTasksPerRun: 3, MaxTaskBytes: 10000}}
	req := httptest.NewRequest(http.MethodPost, "/v1/runs", &body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	rec := httptest.NewRecorder()
	s.handleRuns(rec, req, user{ID: "usr_test", TokenScopes: []string{scopeManagedCheck}})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "managed mode supports only these LLM IDs") {
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

func TestHandleRunsReadsFieldsAfterBundle(t *testing.T) {
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	part, err := mw.CreateFormFile("bundle", "input-docs-bundle.tar.gz")
	if err != nil {
		t.Fatal(err)
	}
	if err := writeManagedServiceTestBundle(part); err != nil {
		t.Fatal(err)
	}
	if err := mw.WriteField("mode", "check"); err != nil {
		t.Fatal(err)
	}
	if err := mw.WriteField("tasks_json", `["check the docs"]`); err != nil {
		t.Fatal(err)
	}
	if err := mw.WriteField("feedback_llm_ids_json", `["unknown-model"]`); err != nil {
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
	if !strings.Contains(rec.Body.String(), "managed mode supports only these LLM IDs") {
		t.Fatalf("body = %s", rec.Body.String())
	}
}

func writeManagedServiceTestBundle(w io.Writer) error {
	content := []byte("hello docs\n")
	sum := sha256.Sum256(content)
	manifest := bundle.Manifest{
		SchemaVersion: 1,
		CreatedAt:     time.Now().UTC().Format(time.RFC3339),
		RepoRoot:      "repo",
		Files: []bundle.FileRecord{{
			Path:      "README.md",
			SizeBytes: int64(len(content)),
			SHA256:    hex.EncodeToString(sum[:]),
		}},
	}
	gz := gzip.NewWriter(w)
	tw := tar.NewWriter(gz)
	manifestBytes, err := json.Marshal(manifest)
	if err != nil {
		return err
	}
	if err := tw.WriteHeader(&tar.Header{Name: "manifest.json", Mode: 0o644, Size: int64(len(manifestBytes))}); err != nil {
		return err
	}
	if _, err := tw.Write(manifestBytes); err != nil {
		return err
	}
	if err := tw.WriteHeader(&tar.Header{Name: "files/README.md", Mode: 0o644, Size: int64(len(content))}); err != nil {
		return err
	}
	if _, err := tw.Write(content); err != nil {
		return err
	}
	if err := tw.Close(); err != nil {
		return err
	}
	return gz.Close()
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
	t.Setenv("DARI_DOCS_RELEASE_ADMIN_TOKEN", "release-admin-token")
	t.Setenv("DARI_DOCS_SECRET_ENCRYPTION_KEY", testManagedSecretEncryptionKey())
	setRequiredManagedAgentEnv(t)
}

func setRequiredManagedAgentEnv(t *testing.T) {
	t.Helper()
	t.Setenv("MANAGED_TESTER_AGENT_ID", "agt_tester")
	t.Setenv("MANAGED_EDITOR_AGENT_ID", "agt_editor")
}

func testManagedHostedAgentConfig() Config {
	return Config{
		ManagedTesterAgentID:   "agt_tester",
		ManagedTesterVersionID: "ver_tester",
		ManagedEditorAgentID:   "agt_editor",
		ManagedEditorVersionID: "ver_editor",
		ReleaseAdminToken:      "release-admin-token",
	}
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
		"MANAGED_TESTER_VERSION_ID",
		"MANAGED_EDITOR_VERSION_ID",
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
