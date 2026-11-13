package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/mupt-ai/dari-docs/internal/config"
	"github.com/mupt-ai/dari-docs/internal/setup"
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
		Short: "Set up a repository: write .dari-docs/config.json and the GitHub workflow",
		Long: `init checks the repository is connected to SelfBench, picks a model from SelfBench's catalog, and
writes .dari-docs/config.json and .github/workflows/dari-docs.yml. It uses your self-bench login
(or SELFBENCH_API_KEY and SELFBENCH_ORG); without one it writes the files with placeholders.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error { return runInit(cmd.Context(), o) },
	}
	cmd.Flags().StringVar(&o.Dir, "dir", ".", "repository root")
	cmd.Flags().StringVar(&o.Org, "org", "", "SelfBench workspace (default: SELFBENCH_ORG, then your self-bench login's)")
	cmd.Flags().StringVar(&o.Repo, "repo", "", "repository, owner/name (default: the origin remote)")
	cmd.Flags().StringVar(&o.Model, "model", "", "SelfBench catalog model ID (default: the newest Claude Sonnet in the catalog)")
	cmd.Flags().BoolVar(&o.Force, "force", false, "overwrite existing files")
	return cmd
}

func runInit(ctx context.Context, o initOptions) error {
	configPath := config.Path(o.Dir)
	workflowPath := filepath.Join(o.Dir, ".github", "workflows", "dari-docs.yml")
	for _, p := range []string{configPath, workflowPath} {
		if _, err := os.Stat(p); err == nil && !o.Force {
			return fmt.Errorf("%s exists; pass --force to overwrite it", p)
		} else if err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	if o.Repo == "" {
		o.Repo, _ = repository(ctx, "", config.Config{})
	}

	var todo []string
	var plan *setup.Plan
	sb, err := connection(config.Config{Org: o.Org})
	switch {
	case err != nil:
		todo = append(todo, "Sign in so dari-docs can reach SelfBench: `self-bench login` (or set SELFBENCH_API_KEY and SELFBENCH_ORG), then rerun `dari-docs init --force`.")
		fmt.Println("dari-docs:", err)
	case o.Repo == "":
		todo = append(todo, "Set `repo` in .dari-docs/config.json to owner/name.")
	default:
		o.Org = sb.Org
		connected, err := sb.Connected(ctx, o.Repo)
		switch {
		case err != nil:
			fmt.Println("dari-docs: could not reach SelfBench:", err)
		case !connected:
			todo = append(todo, fmt.Sprintf("Connect %s in SelfBench: %s (workspace %s).", o.Repo, sb.BaseURL, sb.Org))
		default:
			fmt.Printf("%s is connected to SelfBench workspace %s.\n", o.Repo, sb.Org)
			catalog, err := sb.Catalog(ctx, o.Repo)
			if err != nil {
				fmt.Println("dari-docs: reading the model catalog:", err)
				break
			}
			gen, err := sb.GenerationOptions(ctx, o.Repo)
			if err != nil {
				fmt.Println("dari-docs: reading generation options:", err)
				break
			}
			chosen := setup.Choose(catalog, gen, o.Model)
			plan = &chosen
			for _, m := range chosen.Missing {
				todo = append(todo, "Add "+m+", then rerun `dari-docs init --force`.")
			}
		}
	}

	cfg := config.Template(o.Org, o.Repo, o.Model)
	if plan != nil {
		cfg.Models = []config.Model{plan.Model}
		cfg.Sandbox, cfg.SandboxCredentialID, cfg.Generation = plan.Sandbox, plan.SandboxCredentialID, plan.Generation
		fmt.Printf("Checks will run %s on %s", plan.Model.CatalogID, plan.Model.Harnesses[0])
		if plan.Sandbox != "" {
			fmt.Printf(", in %s", plan.Sandbox)
		}
		fmt.Println("; edit `models` in the config to add or swap models.")
	} else if o.Model == "" {
		todo = append(todo, "Set `models[0].catalogId` in .dari-docs/config.json (`self-bench catalog --repo OWNER/NAME` lists them).")
	}
	if err := config.Save(configPath, cfg); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(workflowPath), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(workflowPath, []byte(workflow()), 0o644); err != nil {
		return err
	}
	fmt.Printf("wrote %s\nwrote %s\n", configPath, workflowPath)

	secret := "gh secret set SELFBENCH_API_KEY"
	if o.Repo != "" {
		secret += " --repo " + o.Repo
	}
	todo = append(todo,
		"Add the tasks checks run, then approve them in SelfBench when they're ready:\n       dari-docs tasks add --title \"First API call\" \"A new user follows the quickstart to ...\"\n     or import a file with one task per paragraph or bullet: dari-docs tasks add --file tasks.txt",
		"Give the workflow a SelfBench API key (create one under Settings → API Keys):\n       "+secret,
		"Commit .dari-docs/config.json and .github/workflows/dari-docs.yml. Pull requests that change docs get a comment.",
	)
	fmt.Println("\nNext:")
	for i, t := range todo {
		fmt.Printf("  %d. %s\n", i+1, t)
	}
	return nil
}

// workflowTemplate pins the action to the release this CLI came from (%s).
const workflowTemplate = `name: dari-docs

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
      - uses: mupt-ai/dari-docs@%s
        with:
          selfbench-api-key: ${{ secrets.SELFBENCH_API_KEY }}
`

// workflow is the workflow init writes. A release pins its own tag; a build from source, main.
func workflow() string {
	ref := "main"
	if strings.HasPrefix(version, "v") || strings.Count(version, ".") == 2 {
		ref = "v" + strings.TrimPrefix(version, "v")
	}
	return fmt.Sprintf(workflowTemplate, ref)
}
