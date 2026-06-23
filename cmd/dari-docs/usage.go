package main

import "fmt"

func usage() {
	fmt.Print(`dari-docs runs managed or self-managed tester and editor agents against your documentation.

Usage:
  dari-docs --version
  dari-docs init [repo]
  dari-docs check [repo|docs-url] --managed --wait --task "Implement auth" [flags]
  dari-docs check [repo|docs-url] --tester-url URL --task "Implement auth" [flags]
  dari-docs optimize [repo] --managed --wait --task "Implement auth" [flags]
  dari-docs optimize [repo] --tester-url URL --editor-url URL --task "Implement auth" [flags]

Managed setup:
  dari-docs auth login
  dari-docs check . --managed --wait --task "Install the SDK"

Self-managed setup:
  dari-docs init
  cd .dari-docs/agents/docs-user-tester-agent && bun install --frozen-lockfile && bun run build && bun run start
  cd ../docs-editor-agent && bun install --frozen-lockfile && bun run build && bun run start

Important flags:
  --managed                   use the hosted Dari Docs service
  --wait                      wait for managed run completion
  --tester-url URL            base URL of the deployed tester Flue app
  --editor-url URL            base URL of the deployed editor Flue app
  --task TEXT                 task/prompt to test; repeatable
  --tasks-file PATH           tasks file; repeatable
  --llm MODEL                 request a model for tester/editor workflows
  --feedback-llm MODEL        request tester model(s); repeatable or comma-separated
  --editor-llm MODEL          request editor model for optimize
  --live-verify               permit safe credential-dependent checks
  --secret-env NAME           pass runtime product/API key to managed run or Flue workflow payload; repeatable
  --bundle-include GLOB       include extra repo-relative docs bundle paths; repeatable
  --bundle-exclude GLOB       exclude repo-relative docs bundle paths; repeatable
  --docs-url URL              give tester agents a public docs URL to use with internet access; repeatable
  --apply                     copy downloaded updated docs back into repo after optimize

Init outputs:
  .dari-docs/config.json
  .dari-docs/agents/

Run outputs:
  .dari-docs/input-docs-bundle.tar.gz
  .dari-docs/runs/feedback-*.md
  .dari-docs/aggregate-feedback.md
  .dari-docs/updated/
`)
}
