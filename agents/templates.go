package agents

import "embed"

// FS contains the bundled Flue app templates used by `dari-docs init`.
//
// Keep the patterns explicit so local node_modules/dist directories are never
// embedded into the Go binary while dot-prefixed .flue workflows are included.
//
//go:embed docs-user-tester-agent/README.md docs-user-tester-agent/package.json docs-user-tester-agent/bun.lock docs-user-tester-agent/flue.config.ts docs-user-tester-agent/agents/** docs-user-tester-agent/prompts/** docs-user-tester-agent/skills/** all:docs-user-tester-agent/.flue/workflows
//go:embed docs-editor-agent/README.md docs-editor-agent/package.json docs-editor-agent/bun.lock docs-editor-agent/flue.config.ts docs-editor-agent/agents/** docs-editor-agent/prompts/** docs-editor-agent/skills/** all:docs-editor-agent/.flue/workflows
var FS embed.FS
