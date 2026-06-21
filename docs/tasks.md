# Tasks

Tasks tell `dari-docs` what the tester app should try to do with your documentation. A good task is concrete and outcome-oriented, such as “Install the SDK and make a first API call.”

## Pass Tasks On The Command Line

Pass one or more `--task` values:

```bash
dari-docs check . \
  --tester-url https://docs-tester.example.com \
  --task "Install the SDK" \
  --task "Set up authentication"
```

Each task is run independently by the deployed Flue tester workflow.

## Keep Tasks In A File

For repeated checks, keep tasks in a file. Write one task per paragraph or bullet:

```text
- Install the SDK and make a first API call

- Set up authentication

Create a webhook endpoint and verify a test event.
```

Run it with:

```bash
dari-docs check . \
  --tester-url https://docs-tester.example.com \
  --tasks-file docs-test-tasks.txt
```

## Choose An Output Directory

Local files are written under `.dari-docs/` by default, and later runs overwrite the previous local outputs. Use `--out` to keep separate run directories:

```bash
dari-docs optimize . \
  --tester-url https://docs-tester.example.com \
  --editor-url https://docs-editor.example.com \
  --out .dari-docs/runs/install-sdk \
  --task "Install the SDK and make a first API call"
```

When `--out` is used, feedback and proposed revisions are written under that output directory for review. The main paths are `<out>/runs/feedback-001.md`, `<out>/aggregate-feedback.md`, and for `optimize`, `<out>/updated/`.
