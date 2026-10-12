package main

import "fmt"

func usage() {
	fmt.Print(`dari-docs runs deployed Harbor tester and editor apps against your documentation.

Usage:
  dari-docs --version
  dari-docs init [repo]
  dari-docs check [repo|docs-url] --tester-url URL --task "Implement auth" [flags]
  dari-docs optimize [repo] --tester-url URL --editor-url URL --task "Implement auth" [flags]

Setup:
  dari-docs init
  uvx modal deploy .dari-docs/agents/modal_app.py

Important flags:
  --tester-url URL            base URL of the deployed tester Harbor app
  --editor-url URL            base URL of the deployed editor Harbor app
  --harness NAME              Harbor harness name or module:Class; server default when unset
  --editor-harness NAME       editor harness override
  --sandbox NAME              Harbor sandbox name or module:Class; server default when unset
  --agent-kwarg KEY=VALUE      agent argument as JSON or string; repeatable
  --agent-env NAME=VALUE      nonsecret agent environment; repeatable
  --sandbox-kwarg KEY=VALUE    sandbox argument as JSON or string; repeatable
  --runner-token-env NAME     bearer token env (default: DARI_DOCS_RUNNER_TOKEN if present)
  --task TEXT                 task/prompt to test; repeatable
  --tasks-file PATH           tasks file; repeatable
  --llm MODEL                 request a model for tester/editor workflows
  --feedback-llm MODEL        request tester model(s); repeatable or comma-separated
  --editor-llm MODEL          request editor model for optimize
  --live-verify               permit safe credential-dependent checks
  --secret-env NAME           pass runtime product/API key to Harbor workflow payload; repeatable
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
