# Configuration

## `.dari-docs/config.json`

`dari-docs init` writes it. Only `models` is required.

| Field | Default | Does |
| --- | --- | --- |
| `org` | your `self-bench` login's workspace | SelfBench workspace. `SELFBENCH_ORG` overrides it. |
| `repo` | `GITHUB_REPOSITORY`, else the `origin` remote | Connected repository, `owner/name` |
| `apiUrl` | your `self-bench` login's, else `https://app.selfbench.dev` | A self-hosted SelfBench. `SELFBENCH_API_URL` overrides it. |
| `docs` | `["docs", "README.md", ":(glob)**/*.md", ":(glob)**/*.mdx"]` | [Git pathspecs](https://git-scm.com/docs/gitglossary#Documentation/gitglossary.txt-aiddefpathspecapathspec) for the files a check compares. `:(exclude)CHANGELOG.md` leaves one out. |
| `models` | required | Up to 12 `{ catalogId, credentialId?, harnesses?, thinking? }`. `credentialId` defaults to `managed-model`, `harnesses` to `["claude-code"]`. List models with `self-bench catalog --repo OWNER/NAME`. |
| `tasks` | every approved task | Tasks checks run, as `runId/taskId`. `dari-docs tasks add` appends the ones it creates. |
| `sandbox`, `sandboxCredentialId` | `managed`, `managed-sandbox` | Where trials run |
| `judgeCredentialId` | none | Needed when a task is graded by an LLM judge |
| `generation` | the deployment's managed settings | Passed to SelfBench when it builds the ablations and writes tasks (its `generation` body) |
| `failOnRegression` | `false` | Exit 2 when a task passes before the change and fails after |
| `timeoutMinutes` | `240` | How long a check waits for builds and trials |

## Credentials

dari-docs calls SelfBench as the owner of an API key. It takes, in order:

1. `SELFBENCH_API_KEY`, `SELFBENCH_ORG`, `SELFBENCH_API_URL`
2. `org` and `apiUrl` in the config
3. what `self-bench login` saved (`$XDG_CONFIG_HOME/self-bench/config.json`), used only for the deployment it was made for

In GitHub Actions only the first two apply: store the key as the `SELFBENCH_API_KEY` secret.

## The action

```yaml
- uses: actions/checkout@v4
  with:
    fetch-depth: 0          # the check diffs against the merge base
- uses: mupt-ai/dari-docs@v1
  with:
    selfbench-api-key: ${{ secrets.SELFBENCH_API_KEY }}
```

| Input | Default |
| --- | --- |
| `selfbench-api-key` | required |
| `selfbench-org` | `org` in the config |
| `selfbench-api-url` | `apiUrl` in the config |
| `config` | `.dari-docs/config.json` |
| `github-token` | the job's token; needs `pull-requests: write` to comment |

Run it on `pull_request` with `closed` among the types, so closing a pull request cleans up. The workflow `init` writes does, skips drafts, and cancels a running check when a newer push arrives.
