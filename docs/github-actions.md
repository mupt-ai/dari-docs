# GitHub Actions

Managed checks can run in CI with a named automation token. The CLI waits until the managed run finishes, so the Actions job status reflects the docs check result.

## Create an automation token

Create the token locally after logging in:

```bash
dari-docs auth login
dari-docs auth token create --name github-actions
```

Add the token to your repository or environment secrets as `DARI_DOCS_TOKEN`.

By default, automation tokens can read managed account and run state and create managed checks. Add scopes explicitly for broader workflows, for example `--scope managed:read --scope managed:optimize` if CI should generate proposed revisions.

Automation tokens do not expire by default. To set an expiration, pass `--expires-in 90d` or `--expires-in 24h`.

## Manage automation tokens

List and revoke tokens with:

```bash
dari-docs auth token list
dari-docs auth token revoke tok_...
```
