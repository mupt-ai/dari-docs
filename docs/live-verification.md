# Live Verification Secrets

By default, agents can inspect docs and try no-credential smoke tests, but they do not receive your product or API secrets. To let the tester and editor workflows use safe test-mode product credentials, pass `--live-verify` and repeat `--secret-env NAME` for each local environment variable to send.

```bash
export STRIPE_TEST_SECRET_KEY=sk_test_...

dari-docs optimize . \
  --tester-url https://docs-tester.example.com \
  --editor-url https://docs-editor.example.com \
  --live-verify \
  --secret-env STRIPE_TEST_SECRET_KEY \
  --task "Create a checkout session"
```

`dari-docs` reads each named environment variable locally and includes the value in the Flue workflow payload for that run. The bundled workflows expose those values to the agent sandbox as environment variables with the same names, so the agent can use them in shell commands, SDK calls, or API requests needed for the task.

Use test-mode, read-only, or otherwise safe credentials, such as a Stripe test key, a sandbox API token, or a read-only token for a disposable test account. Avoid production admin keys, write-capable production keys, and credentials that can access customer data unless the task explicitly requires production access and you are comfortable with the agent using them. The agents are instructed not to print secret values, but you should still treat every value passed with `--secret-env` as available to the Flue app and its agent workspace.

Model provider keys are different. Set `ANTHROPIC_API_KEY`, `OPENAI_API_KEY`, or another provider key in the Flue app deployment environment. Do not pass model provider keys with `--secret-env`.
