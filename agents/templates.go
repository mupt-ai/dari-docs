package agents

import "embed"

// FS contains the bundled Harbor runner, deployment and workflow instructions.
//
//go:embed modal_app.py harbor_runner.py runner_api.py requirements.txt Dockerfile
//go:embed docs-user-tester-agent/README.md docs-user-tester-agent/prompts/** docs-user-tester-agent/skills/**
//go:embed docs-editor-agent/README.md docs-editor-agent/prompts/** docs-editor-agent/skills/**
var FS embed.FS
