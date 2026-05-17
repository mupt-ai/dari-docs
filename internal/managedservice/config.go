package managedservice

import (
	"errors"
	"os"
	"time"
)

type Config struct {
	Addr                       string
	DatabaseURL                string
	PublicBaseURL              string
	DariAPIBaseURL             string
	DariAPIKey                 string
	ManagedTesterAgentID       string
	ManagedTesterVersionID     string
	ManagedEditorAgentID       string
	ManagedEditorVersionID     string
	ReleaseAdminToken          string
	RuntimeSecretsKey          []byte
	FreeGrantCents             int64
	TesterReserveCents         int64
	EditorReserveCents         int64
	ServiceFeeCents            int64
	MaxBundleBytes             int64
	BundleMaxUncompressedBytes int64
	BundleMaxFileBytes         int64
	MaxTasksPerRun             int
	MaxTaskBytes               int64
	MaxActiveRunsPerUser       int
	SessionStaleAfter          time.Duration
	SessionStartStaleAfter     time.Duration
	PollErrorStaleAfter        time.Duration
	CostFetchTimeout           time.Duration
	StripeSecretKey            string
	StripeWebhookSecret        string
	StripeWebhookTolerance     time.Duration
	HTTPReadHeaderTimeout      time.Duration
	HTTPReadTimeout            time.Duration
	HTTPWriteTimeout           time.Duration
	HTTPIdleTimeout            time.Duration
	OutboundHTTPTimeout        time.Duration
}

func ConfigFromEnv() (Config, error) {
	cfg := Config{
		Addr:                       env("ADDR", ":"+env("PORT", defaultPort)),
		PublicBaseURL:              env("PUBLIC_BASE_URL", defaultPublicBaseURL),
		DariAPIBaseURL:             env("DARI_API_BASE_URL", defaultDariAPIBaseURL),
		FreeGrantCents:             managedFreeGrantCents,
		TesterReserveCents:         managedTesterReserveCents,
		EditorReserveCents:         managedEditorReserveCents,
		ServiceFeeCents:            managedServiceFeeCents,
		MaxBundleBytes:             managedMaxBundleBytes,
		BundleMaxUncompressedBytes: managedBundleMaxUncompressedBytes,
		BundleMaxFileBytes:         managedBundleMaxFileBytes,
		MaxTasksPerRun:             int(managedMaxTasksPerRun),
		MaxTaskBytes:               managedMaxTaskBytes,
		MaxActiveRunsPerUser:       int(managedMaxActiveRunsPerUser),
		SessionStaleAfter:          time.Duration(managedSessionStaleAfterSeconds) * time.Second,
		SessionStartStaleAfter:     time.Duration(managedSessionStartStaleAfterSeconds) * time.Second,
		PollErrorStaleAfter:        time.Duration(managedPollErrorStaleAfterSeconds) * time.Second,
		CostFetchTimeout:           time.Duration(managedCostFetchTimeoutSeconds) * time.Second,
		StripeWebhookTolerance:     time.Duration(managedStripeWebhookToleranceSeconds) * time.Second,
		HTTPReadHeaderTimeout:      time.Duration(managedHTTPReadHeaderTimeoutSeconds) * time.Second,
		HTTPReadTimeout:            time.Duration(managedHTTPReadTimeoutSeconds) * time.Second,
		HTTPWriteTimeout:           time.Duration(managedHTTPWriteTimeoutSeconds) * time.Second,
		HTTPIdleTimeout:            time.Duration(managedHTTPIdleTimeoutSeconds) * time.Second,
		OutboundHTTPTimeout:        time.Duration(managedOutboundHTTPTimeoutSeconds) * time.Second,
		DatabaseURL:                os.Getenv("DATABASE_URL"),
		DariAPIKey:                 os.Getenv("DARI_API_KEY"),
		ManagedTesterAgentID:       os.Getenv("MANAGED_TESTER_AGENT_ID"),
		ManagedTesterVersionID:     os.Getenv("MANAGED_TESTER_VERSION_ID"),
		ManagedEditorAgentID:       os.Getenv("MANAGED_EDITOR_AGENT_ID"),
		ManagedEditorVersionID:     os.Getenv("MANAGED_EDITOR_VERSION_ID"),
		ReleaseAdminToken:          os.Getenv("DARI_DOCS_RELEASE_ADMIN_TOKEN"),
		StripeSecretKey:            os.Getenv("STRIPE_SECRET_KEY"),
		StripeWebhookSecret:        os.Getenv("STRIPE_WEBHOOK_SECRET"),
	}
	if cfg.DatabaseURL == "" {
		return Config{}, errors.New("DATABASE_URL is required")
	}
	if cfg.DariAPIKey == "" {
		return Config{}, errors.New("DARI_API_KEY is required")
	}
	if cfg.ManagedTesterAgentID == "" {
		return Config{}, errors.New("MANAGED_TESTER_AGENT_ID is required")
	}
	if cfg.ManagedEditorAgentID == "" {
		return Config{}, errors.New("MANAGED_EDITOR_AGENT_ID is required")
	}
	if cfg.ReleaseAdminToken == "" {
		return Config{}, errors.New("DARI_DOCS_RELEASE_ADMIN_TOKEN is required")
	}
	key, err := decodeRuntimeSecretsKey(os.Getenv("DARI_DOCS_SECRET_ENCRYPTION_KEY"))
	if err != nil {
		return Config{}, err
	}
	cfg.RuntimeSecretsKey = key
	return cfg, nil
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
