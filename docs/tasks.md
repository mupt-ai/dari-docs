# Tasks

Tasks tell `dari-docs` what the simulated users should try to do with your documentation. A good task is concrete and outcome-oriented, such as “Install the SDK and make a first API call.”

## Pass tasks on the command line

Pass one or more `--task` values:

```bash
dari-docs check . \
  --managed \
  --task "Install the SDK" \
  --task "Set up authentication"
```

## Keep tasks in a file

For repeated checks, keep tasks in a file. Write one task per paragraph or bullet:

```bash
dari-docs check . \
  --managed \
  --tasks-file docs-test-tasks.txt
```

## Choose an output directory

When you wait for a run or download artifacts, local files are written under `.dari-docs/` by default, and later runs overwrite the previous local outputs. Use `--out` to keep separate run directories:

```bash
dari-docs optimize . \
  --managed \
  --wait \
  --out .dari-docs/runs/install-sdk \
  --task "Install the SDK and make a first API call"
```

When `--out` is used, downloaded revisions are written under that output directory for review.
