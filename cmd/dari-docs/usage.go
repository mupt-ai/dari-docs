package main

import "fmt"

func usage() {
	fmt.Print(`dari-docs runs Flue tester and editor agents against your documentation.

Usage:
  dari-docs --version
  dari-docs init [repo] [--deploy]
  dari-docs check [repo|docs-url] --task "Implement auth" [flags]
  dari-docs optimize [repo] --task "Implement auth" [flags]

Setup:
  dari auth login
  dari credentials add ANTHROPIC_API_KEY
  export DARI_API_KEY=...
  dari-docs init --deploy

Important flags:
  --task TEXT                 task/prompt to test; repeatable
  --tasks-file PATH           tasks file; repeatable
  --live-verify               permit safe credential-dependent checks
  --secret-env NAME           pass runtime product/API key from env var; repeatable
  --bundle-include GLOB       include extra repo-relative docs bundle paths; repeatable
  --bundle-exclude GLOB       exclude repo-relative docs bundle paths; repeatable
  --docs-url URL              give tester agents a public docs URL to use with internet access; repeatable
  --apply                     copy downloaded updated docs back into repo after optimize
  --api-base-url URL          Dari API base URL
  --parallel N                tester sessions per batch
  --anthropic-api-key-secret  stored Dari credential name for Anthropic deploys
  --openai-api-key-secret     stored Dari credential name for OpenAI deploys

Init outputs:
  .dari-docs/config.json
  .dari-docs/agents/

Run outputs:
  .dari-docs/input-docs-bundle.tar.gz
  .dari-docs/runs/feedback-*.md
  .dari-docs/aggregate-feedback.md
  .dari-docs/updated-docs-workspace.zip
  .dari-docs/updated/
`)
}
