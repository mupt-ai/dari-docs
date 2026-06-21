# Tasks

Tasks tell `dari-docs` what the tester agents should try to do with your documentation. A good task is concrete and outcome-oriented, such as “Install the SDK and make a first API call.”

## Pass tasks on the command line

Pass one or more `--task` values:

```bash
dari-docs check . \
  --task "Install the SDK" \
  --task "Set up authentication"
```

Each task is run independently by the deployed Flue tester agent.

## Keep tasks in a file

For repeated checks, keep tasks in a file. Write one task per paragraph or bullet:

```bash
dari-docs check . \
  --tasks-file docs-test-tasks.txt
```

## Choose an output directory

Local files are written under `.dari-docs/` by default, and later runs overwrite the previous local outputs. Use `--out` to keep separate run directories:

```bash
dari-docs optimize . \
  --out .dari-docs/runs/install-sdk \
  --task "Install the SDK and make a first API call"
```

When `--out` is used, feedback and proposed revisions are written under that output directory for review.
