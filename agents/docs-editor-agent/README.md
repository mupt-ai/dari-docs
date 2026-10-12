# Documentation editor

The shared Python `harbor_runner.py` loads `prompts/system.md`, stages the source docs and aggregate tester feedback, and runs a Harbor trial. The agent writes complete proposed files under `/app/output/updated-docs/files/` and a changelog at `/app/output/changelog.md`. The controller returns validated `{"result":{"changelog":"...","files":[{"path":"README.md","content":"..."}]}}`.

Local development, from the parent agents directory:

```bash
python harbor_runner.py --service editor --port 8001
```

Paths must remain repo-relative; symlinks and special files are rejected. Unchanged files are filtered. Missing output triggers one retry in the same session when the harness supports resume, otherwise a fresh prompt in the same workspace. A run with original docs but no changed files fails rather than returning an empty proposal.

Select the editor separately with `--editor-harness`; model/provider and sandbox compatibility depend on the selected Harbor adapters. Provider credentials belong in the controller environment or Modal secret. The prompt and skill content are included in task instructions, including for custom harnesses without native skill support.
