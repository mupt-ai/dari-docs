# Managed Mode

Managed mode runs `dari-docs` through the hosted Dari Docs service. The hosted tester and editor agents are ordinary dari.dev agents deployed with `sandbox.provider: modal`, so each managed session runs in a Modal sandbox without requiring users to deploy agent infrastructure.

## Set Up

Log in from the repo that contains your docs:

```bash
dari-docs auth login
```

For CI, create an API key instead and store it as `DARI_DOCS_API_KEY`:

```bash
dari-docs auth api-key create --name github-actions
```

## Run A Check

```bash
dari-docs check . \
  --managed \
  --wait \
  --feedback-llm claude-haiku-4-5 \
  --task "Install the SDK and make a first API call"
```

Without `--wait`, the command submits the run and prints the run ID. With `--wait`, it polls until completion and writes feedback under `.dari-docs/`.

## Optimize Docs

```bash
dari-docs optimize . \
  --managed \
  --wait \
  --task "Install the SDK and make a first API call"
```

Completed optimize runs download proposed edits into `.dari-docs/updated/`. Add `--apply` with `--wait` only when you want the CLI to copy those edited files back into the repo.

## Runs

Use the run ID from a submitted run:

```bash
dari-docs runs status run_...
dari-docs runs wait run_...
dari-docs runs download run_...
```

## Models And Secrets

Managed mode supports the hosted Claude and GPT model options returned by the managed service. `--llm`, `--feedback-llm`, and `--editor-llm` select those hosted options; common groups such as `claude`, `gpt`, and `all` expand for tester runs.

Runtime product/API credentials are opt-in per run:

```bash
dari-docs check . \
  --managed \
  --live-verify \
  --secret-env STRIPE_TEST_SECRET_KEY \
  --task "Create a checkout session"
```

`--secret-env` sends the named local environment variables to that managed run only. The hosted manifests omit provider-specific LLM API key secrets, so model calls use platform-managed LLM credentials. The manifests also omit a Modal `provider_api_key_secret`, so sandboxes use platform-managed Modal credentials.

## Self-Managed

Use [Flue app setup](self-managed.md) when you want to deploy and operate your own tester/editor URLs. Self-managed users run the bundled Flue projects with `uvx modal deploy .dari-docs/agents/modal_app.py`; managed users do not deploy those templates.
