package main

import "fmt"

func usage() {
	fmt.Print(`dari-docs runs lightweight user-test sessions, feeds the results into a hosted editor, and pulls updated docs back to your repo.

Usage:
  dari-docs --version
  dari-docs auth login
  dari-docs auth status
  dari-docs auth api-key create --name github-actions
  dari-docs auth api-key list
  dari-docs auth api-key revoke <api-key-id>
  dari-docs auth logout [--all] [--interactive-only|--automation-only]
  dari-docs init [repo]
  dari-docs billing balance
  dari-docs runs status <run-id>
  dari-docs runs wait <run-id>
  dari-docs runs feedback <run-id>
  dari-docs runs download <run-id> [repo]
  dari-docs runs apply <run-id> [repo]
  dari-docs optimize [repo|docs-url] --task "Implement auth" [--task "Set up webhooks"] [flags]
  dari-docs check [repo|docs-url] --task "Implement auth" [flags]

Managed setup:
  dari-docs auth login

Self-managed setup:
  export DARI_API_KEY=...
  dari-docs init --deploy

Important flags:
  --task TEXT                 task/prompt to test; repeatable
  --tasks-file PATH           tasks file; repeatable
  --live-verify               permit safe credential-dependent checks
  --secret-env NAME           pass runtime product/API key from env var; repeatable
  --managed                   use the managed dari-docs service instead of your Dari org
  --wait                      wait for a managed run to finish before exiting
  --bundle-include GLOB       include extra repo-relative docs bundle paths; repeatable
  --bundle-exclude GLOB       exclude repo-relative docs bundle paths; repeatable
  --docs-url URL              give agents a public docs URL to use with internet access; repeatable
  --apply                     copy downloaded updated docs back into repo
  --api-base-url URL          Dari API base URL; self-managed only
  --parallel N                tester sessions per batch; self-managed only
  --llm ID                    select an LLM option for all sessions
  --feedback-llm ID           select tester LLM option(s); repeat or comma-separate; supports all, claude, gpt
  --editor-llm ID             select a manifest LLM option for the editor session
  --anthropic-api-key-secret  stored Dari credential name for Anthropic BYOK deploys
  --openai-api-key-secret     stored Dari credential name for OpenAI BYOK deploys

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
