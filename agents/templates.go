package agents

import "embed"

// FS contains the bundled Flue agent folder templates used by `dari-docs init`.
//
// Keep the patterns explicit so local generated directories are never embedded
// into the Go binary.
//
//go:embed modal_app.py
//go:embed docs-user-tester-agent/README.md docs-user-tester-agent/package.json docs-user-tester-agent/bun.lock docs-user-tester-agent/flue.config.ts docs-user-tester-agent/app.ts docs-user-tester-agent/agents/** docs-user-tester-agent/workflows/** docs-user-tester-agent/prompts/** docs-user-tester-agent/skills/**
//go:embed docs-editor-agent/README.md docs-editor-agent/package.json docs-editor-agent/bun.lock docs-editor-agent/flue.config.ts docs-editor-agent/app.ts docs-editor-agent/agents/** docs-editor-agent/workflows/** docs-editor-agent/prompts/** docs-editor-agent/skills/**
var FS embed.FS
