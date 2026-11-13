package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/mupt-ai/dari-docs/internal/config"
	"github.com/spf13/cobra"
)

type initOptions struct {
	Dir   string
	Org   string
	Repo  string
	Model string
	Force bool
}

func newInitCommand() *cobra.Command {
	o := initOptions{}
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Write .dari-docs/config.json and a GitHub workflow that checks docs pull requests",
		Args:  cobra.NoArgs,
		RunE:  func(cmd *cobra.Command, _ []string) error { return runInit(cmd.Context(), o) },
	}
	cmd.Flags().StringVar(&o.Dir, "dir", ".", "repository root")
	cmd.Flags().StringVar(&o.Org, "org", os.Getenv("SELFBENCH_ORG"), "SelfBench workspace")
	cmd.Flags().StringVar(&o.Repo, "repo", "", "connected repository, owner/name (default: the origin remote)")
	cmd.Flags().StringVar(&o.Model, "model", "", "a SelfBench catalog model ID to check with")
	cmd.Flags().BoolVar(&o.Force, "force", false, "overwrite existing files")
	return cmd
}

func runInit(ctx context.Context, o initOptions) error {
	if o.Repo == "" {
		o.Repo, _ = repository(ctx, "", config.Config{})
	}
	cfg := config.Template(o.Org, o.Repo, o.Model)
	b, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	files := []struct {
		path    string
		content []byte
	}{
		{config.Path(o.Dir), append(b, '\n')},
		{filepath.Join(o.Dir, ".github", "workflows", "dari-docs.yml"), []byte(workflow)},
	}
	for _, f := range files {
		if _, err := os.Stat(f.path); err == nil && !o.Force {
			return fmt.Errorf("%s exists; pass --force to overwrite it", f.path)
		} else if err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	for _, f := range files {
		if err := os.MkdirAll(filepath.Dir(f.path), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(f.path, f.content, 0o644); err != nil {
			return err
		}
		fmt.Println("wrote", f.path)
	}
	fmt.Print(`
Next:
  1. Connect the repository in SelfBench and approve some tasks. Tasks about using the docs work best:
       dari-docs tasks add --title "Make a first API call" "A new user follows the quickstart to ..."
  2. Set the model in .dari-docs/config.json (list them with: self-bench catalog --repo OWNER/NAME).
  3. Add a SELFBENCH_API_KEY secret to the GitHub repository (SelfBench → Settings → API Keys).
`)
	if cfg.Org == "" {
		fmt.Println("  4. Set `org` in .dari-docs/config.json to your SelfBench workspace.")
	}
	return nil
}

const workflow = `name: dari-docs

on:
  pull_request:
    types: [opened, synchronize, reopened, ready_for_review, closed]

permissions:
  contents: read
  pull-requests: write

concurrency:
  group: dari-docs-${{ github.event.pull_request.number }}
  cancel-in-progress: true

jobs:
  docs:
    if: github.event.pull_request.draft == false || github.event.action == 'closed'
    runs-on: ubuntu-latest
    timeout-minutes: 300
    steps:
      - uses: actions/checkout@v4
        with:
          fetch-depth: 0
      - uses: mupt-ai/dari-docs@main
        with:
          selfbench-api-key: ${{ secrets.SELFBENCH_API_KEY }}
`
