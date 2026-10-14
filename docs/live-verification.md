# Live Verification Secrets

By default, agents can inspect docs and try no-credential smoke tests, but they do not receive your product or API secrets. To let the tester and editor workflows use safe test-mode product credentials, pass `--live-verify` and repeat `--secret-env NAME` for each local environment variable to send.

```bash
export STRIPE_TEST_SECRET_KEY=sk_test_...

dari-docs optimize . \
  --tester-url https://your-tester-url.modal.run \
  --editor-url https://your-editor-url.modal.run \
  --live-verify \
  --secret-env STRIPE_TEST_SECRET_KEY \
  --task "Create a checkout session"
```

`dari-docs` reads each named environment variable locally and includes the value in the workflow payload for that run. The runner rejects `runtimeSecrets` unless `--live-verify` is set. The bundled runner exposes those values to the agent in its sandbox as environment variables with the same names, so the agent can use them in shell commands, SDK calls, or API requests needed for the task.

Use test-mode, read-only, or otherwise safe credentials, such as a Stripe test key, a sandbox API token, or a read-only token for a disposable test account. Avoid production admin keys, write-capable production keys, and credentials that can access customer data unless the task explicitly requires production access and you are comfortable with the agent using them. The agents are instructed not to print secret values, but you should still treat every value passed with `--secret-env` as available to the runner and the agent's sandbox.

Model provider keys are different. Set `ANTHROPIC_API_KEY`, `OPENAI_API_KEY`, or another provider key in the Modal secret or the runner's environment. Do not pass model provider keys with `--secret-env`.

Values are used literally; a value containing a `${...}` template is rejected. Runtime secrets can't override runner configuration. The runner rejects names that are model provider variables, start with `MODAL_` or `DARI_DOCS_`, or are `PATH`, `HOME`, or `PYTHONPATH`. It also rejects a result that contains any runtime secret value, so a run fails rather than returning a leaked secret.

`--agent-env NAME=VALUE` is for non-secret harness settings only. Names that look like credentials, such as those containing `KEY`, `TOKEN`, or `SECRET`, are rejected; use `--secret-env` for product credentials.
