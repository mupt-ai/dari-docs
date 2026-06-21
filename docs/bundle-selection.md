# Bundle Selection

Before a local-docs run starts, `dari-docs` creates `.dari-docs/input-docs-bundle.tar.gz`. The bundle is what the tester and editor agents can read for that run.

By default, `dari-docs` includes files with docs-friendly extensions such as `.md`, `.mdx`, `.txt`, `.json`, `.yml`, `.yaml`, `.toml`, `.css`, `.js`, `.jsx`, `.ts`, and `.tsx`. It also includes common docs entrypoints such as `README.md`, `README`, `mint.json`, `docs.json`, `openapi.json`, `openapi.yaml`, `llms.txt`, and `llms-full.txt`.

It skips `.git`, `node_modules`, `.dari-docs`, `.next`, `dist`, `build`, `coverage`, and `.turbo` directories by default. Individual files larger than 5 MB are skipped. The CLI prints a bundle summary before starting the tester sessions, and you can inspect the archive at `.dari-docs/input-docs-bundle.tar.gz` after the run starts. Environment variables are not bundled, but secrets committed to matching files are included unless you exclude those paths.

## Include or exclude files

Use repo-relative globs when your docs need extra inputs or when generated paths should be excluded:

```bash
dari-docs check . \
  --bundle-include "schemas/*.proto" \
  --bundle-exclude "docs/generated/**" \
  --task "Create an API key"
```

`--bundle-include` adds files in addition to the defaults. `--bundle-exclude` wins over both defaults and include patterns.

## Public docs URLs

Use `--docs-url` with `check` to test public docs without checking out a repo. The URL is passed to the tester agent as a public docs source, and internet access is required. For a normal page, the tester uses that page as the starting point and may follow links that look relevant to the task. For `llms.txt` or `llms-full.txt`, the tester reads the file as an LLM docs manifest and follows the relevant links listed there.

```bash
dari-docs check \
  --docs-url https://docs.dari.dev/llms.txt \
  --task "Create a browser session"
```

You can also pass the URL as the source argument:

```bash
dari-docs check https://docs.dari.dev/llms.txt \
  --task "Create a browser session"
```

Public docs URLs support `check` only. Use local docs files for `optimize`, because the editor needs concrete files to modify and download.
