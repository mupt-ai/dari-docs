package agents

import "embed"

// FS contains the bundled Dari agent templates used by `dari-docs init`.
//
//go:embed docs-user-tester-agent/** docs-editor-agent/**
var FS embed.FS
