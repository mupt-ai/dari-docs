# Live Verification Secrets

Static testing is the default. To let agents use test-mode product or API keys for safe checks, pass `--live-verify` and repeat `--secret-env NAME` for each local environment variable to send.

## Managed example

```bash
export STRIPE_TEST_SECRET_KEY=sk_test_...

dari-docs optimize . \
  --managed \
  --live-verify \
  --secret-env STRIPE_TEST_SECRET_KEY \
  --task "Create a checkout session"
```

Secret values are passed to the remote sessions only for that run. In managed mode, runtime secrets are encrypted while the run is active and cleared after use.
