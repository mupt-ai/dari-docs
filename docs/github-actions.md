# GitHub Actions

Managed checks can run in CI with a named API key. The CLI waits until the managed run finishes, so the Actions job status reflects the docs check result.

## Create an API key

Create the API key locally after logging in:

```bash
dari-docs auth login
dari-docs auth api-key create --name github-actions
```

Add the API key to your repository or environment secrets as `DARI_DOCS_API_KEY`.

By default, API keys can read managed account and run state and create managed checks. Add scopes explicitly for broader workflows, for example `--scope managed:read --scope managed:optimize` if CI should generate proposed revisions.

API keys do not expire by default. To set an expiration, pass `--expires-in 90d` or `--expires-in 24h`.

## Manage API keys

List and revoke API keys with:

```bash
dari-docs auth api-key list
dari-docs auth api-key revoke tok_...
```
