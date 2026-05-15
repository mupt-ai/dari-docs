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
