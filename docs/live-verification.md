# Live Verification Secrets

By default, agents can inspect docs and try no-credential smoke tests, but they do not receive your product or API secrets. To let tester and editor agents use safe test-mode product credentials, pass `--live-verify` and repeat `--secret-env NAME` for each local environment variable to send.

```bash
export STRIPE_TEST_SECRET_KEY=sk_test_...

dari-docs optimize . \
  --live-verify \
  --secret-env STRIPE_TEST_SECRET_KEY \
  --task "Create a checkout session"
```

Secret values are passed to each remote Flue tester/editor session started for that run. The agents receive them as environment variables with the same names you passed to `--secret-env`, so they can use those values in shell commands, SDK calls, or API requests needed for the task.

Use test-mode, read-only, or otherwise safe credentials, such as a Stripe test key, a sandbox API token, or a read-only token for a disposable test account. Avoid production admin keys, write-capable production keys, and credentials that can access customer data unless the task explicitly requires production access and you are comfortable with the agent using them. The agents are instructed not to print secret values, but you should still treat every value passed with `--secret-env` as available to the remote session.
