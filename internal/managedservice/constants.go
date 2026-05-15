package managedservice

const (
	defaultPort           = "8080"
	defaultPublicBaseURL  = "http://localhost:8080"
	defaultDariAPIBaseURL = "https://api.dari.dev"

	managedFreeGrantCents     int64 = 500
	managedTesterReserveCents int64 = 75
	managedEditorReserveCents int64 = 150
	managedServiceFeeCents    int64 = 0

	stripeCheckoutMinCents int64 = 500
	stripeCheckoutMaxCents int64 = 50000

	managedMaxBundleBytes             int64 = 25 * 1024 * 1024
	managedBundleMaxUncompressedBytes int64 = 100 * 1024 * 1024
	managedBundleMaxFileBytes         int64 = 5 * 1024 * 1024
	managedMaxUpdatedZipBytes         int64 = 25 * 1024 * 1024
	managedMaxRuntimeSecretsBytes     int64 = 64 * 1024
	managedMaxTasksPerRun             int64 = 3
	managedMaxTaskBytes               int64 = 10000
	managedMaxActiveRunsPerUser       int64 = 3

	managedSessionStaleAfterSeconds      int64 = 24 * 60 * 60
	managedSessionStartStaleAfterSeconds int64 = 2 * 60
	managedPollErrorStaleAfterSeconds    int64 = 60 * 60
	managedCostFetchTimeoutSeconds       int64 = 5 * 60
	managedStripeWebhookToleranceSeconds int64 = 5 * 60

	managedHTTPReadHeaderTimeoutSeconds int64 = 10
	managedHTTPReadTimeoutSeconds       int64 = 120
	managedHTTPWriteTimeoutSeconds      int64 = 600
	managedHTTPIdleTimeoutSeconds       int64 = 120
	managedOutboundHTTPTimeoutSeconds   int64 = 30
)
