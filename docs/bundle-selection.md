# Bundle Selection

Before a local-docs run starts, `dari-docs` creates `.dari-docs/input-docs-bundle.tar.gz`. The bundle is the selected set of docs files for one run. The archive is kept locally for inspection, and the CLI sends the bundle contents to your Flue workflow, which writes them under `input-docs/files/` in the agent workspace.

By default, `dari-docs` includes files with docs-friendly extensions such as `.md`, `.mdx`, `.txt`, `.json`, `.yml`, `.yaml`, `.toml`, `.css`, `.js`, `.jsx`, `.ts`, and `.tsx`. It also includes common docs entrypoints such as `README.md`, `README`, `mint.json`, `docs.json`, `openapi.json`, `openapi.yaml`, `llms.txt`, and `llms-full.txt`.

It skips `.git`, `node_modules`, `.dari-docs`, `.next`, `dist`, `build`, `coverage`, and `.turbo` directories by default. Individual files larger than 5 MB are skipped. Environment variables are not bundled, but secrets committed to matching files are included unless you exclude those paths. Pay particular attention to docs examples, OpenAPI fixtures, JSON/YAML config, and generated TypeScript files; if they contain tokens, exclude them.

## Include Or Exclude Files

Use repo-relative globs when your docs need extra inputs or when generated paths should be excluded:

```bash
dari-docs check . \
  --tester-url https://your-tester-url.modal.run \
  --bundle-include "schemas/*.proto" \
  --bundle-exclude "docs/generated/**" \
  --task "Create an API key"
```

`--bundle-include` adds files in addition to the defaults. `--bundle-exclude` wins over both defaults and include patterns.

## Public Docs URLs

Use `--docs-url` with `check` to test public docs without checking out a repo. The URL is passed to the tester workflow as a public docs source, and internet access is required. If you pass both a local repo and `--docs-url`, both are sent: local files are written under `input-docs/files/`, and the URL is included in the task prompt. For a normal page, the tester uses that page as the starting point and may follow relevant same-site links as needed for the task. For `llms.txt` or `llms-full.txt`, the tester treats the file as an LLM docs manifest: a plain-text or Markdown index that lists documentation pages intended for agents to read, usually as headings plus links.

```bash
dari-docs check \
  --tester-url https://your-tester-url.modal.run \
  --docs-url https://docs.example.com/llms.txt \
  --task "Create a browser session"
```

You can also pass the URL as the source argument:

```bash
dari-docs check https://docs.example.com/llms.txt \
  --tester-url https://your-tester-url.modal.run \
  --task "Create a browser session"
```

Public docs URLs support `check` only. Use local docs files for `optimize`, because the editor needs concrete files to modify and download.
