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
	AgentDeployClaimBatchSize  int
	SessionStaleAfter          time.Duration
	SessionStartStaleAfter     time.Duration
	PollErrorStaleAfter        time.Duration
	AgentDeployStaleAfter      time.Duration
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
		AgentDeployClaimBatchSize:  int(managedAgentDeployClaimBatchSize),
		SessionStaleAfter:          time.Duration(managedSessionStaleAfterSeconds) * time.Second,
		SessionStartStaleAfter:     time.Duration(managedSessionStartStaleAfterSeconds) * time.Second,
		PollErrorStaleAfter:        time.Duration(managedPollErrorStaleAfterSeconds) * time.Second,
		AgentDeployStaleAfter:      time.Duration(managedAgentDeployStaleAfterSeconds) * time.Second,
		CostFetchTimeout:           time.Duration(managedCostFetchTimeoutSeconds) * time.Second,
		StripeWebhookTolerance:     time.Duration(managedStripeWebhookToleranceSeconds) * time.Second,
		HTTPReadHeaderTimeout:      time.Duration(managedHTTPReadHeaderTimeoutSeconds) * time.Second,
		HTTPReadTimeout:            time.Duration(managedHTTPReadTimeoutSeconds) * time.Second,
		HTTPWriteTimeout:           time.Duration(managedHTTPWriteTimeoutSeconds) * time.Second,
		HTTPIdleTimeout:            time.Duration(managedHTTPIdleTimeoutSeconds) * time.Second,
		OutboundHTTPTimeout:        time.Duration(managedOutboundHTTPTimeoutSeconds) * time.Second,
		DatabaseURL:                os.Getenv("DATABASE_URL"),
		DariAPIKey:                 os.Getenv("DARI_API_KEY"),
		StripeSecretKey:            os.Getenv("STRIPE_SECRET_KEY"),
		StripeWebhookSecret:        os.Getenv("STRIPE_WEBHOOK_SECRET"),
	}
	if cfg.DatabaseURL == "" {
		return Config{}, errors.New("DATABASE_URL is required")
	}
	if cfg.DariAPIKey == "" {
		return Config{}, errors.New("DARI_API_KEY is required")
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
