# Bundle Selection

Before a run starts, `dari-docs` creates `.dari-docs/input-docs-bundle.tar.gz`. By default, the bundle includes likely docs and docs-adjacent source files, while skipping common generated, dependency, build, and local output directories.

The CLI prints a bundle summary before starting the run.

## Include or exclude files

Use repo-relative globs when your docs need extra inputs or when generated paths should be excluded:

```bash
dari-docs check . \
  --managed \
  --bundle-include "schemas/*.proto" \
  --bundle-exclude "docs/generated/**" \
  --task "Create an API key"
```

`--bundle-include` adds files in addition to the defaults. `--bundle-exclude` wins over both defaults and include patterns.

## Public Docs URLs

Use `--docs-url` to test public docs without checking out a repo. The URL is passed to the tester agent as a public docs source, and internet access is required. For `llms.txt` or `llms-full.txt`, the agent reads the manifest and decides which linked docs are relevant to the task.

```bash
dari-docs check \
  --managed \
  --docs-url https://www.kernel.sh/docs/llms.txt \
  --task "Create a browser session"
```

You can also pass the URL as the source argument:

```bash
dari-docs check https://www.kernel.sh/docs/llms.txt \
  --managed \
  --task "Create a browser session"
```

Public docs URLs support `check` only. Use local docs files for `optimize`, because the editor needs concrete files to modify and download.
