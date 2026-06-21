package main

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/mupt-ai/dari-docs/internal/agenttemplates"
	"github.com/mupt-ai/dari-docs/internal/projectconfig"
	"github.com/spf13/cobra"
)

type initOptions struct {
	RepoArg               string
	Deploy                bool
	APIKeyEnv             string
	APIKey                string
	APIBaseURL            string
	LLMAPIKeySecret       string
	AnthropicAPIKeySecret string
	OpenAIAPIKeySecret    string
	TesterURL             string
	EditorURL             string
	AgentsDir             string
}

func newInitCommand() *cobra.Command {
	opts := initOptions{}
	cmd := &cobra.Command{
		Use:           "init [repo]",
		Short:         "Extract bundled Flue apps",
		Args:          cobra.ArbitraryArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				opts.RepoArg = args[0]
			}
			return runInitWithOptions(cmd.Context(), opts)
		},
	}
	cmd.Flags().BoolVar(&opts.Deploy, "deploy", false, "unsupported; deploy the Flue apps with `flue build` and your host")
	cmd.Flags().StringVar(&opts.APIKeyEnv, "api-key-env", opts.APIKeyEnv, "unsupported; Dari API keys are not used by Flue deployments")
	cmd.Flags().StringVar(&opts.APIKey, "api-key", "", "unsupported; Dari API keys are not used by Flue deployments")
	cmd.Flags().StringVar(&opts.APIBaseURL, "api-base-url", opts.APIBaseURL, "unsupported; Dari API URLs are not used by Flue deployments")
	cmd.Flags().StringVar(&opts.LLMAPIKeySecret, "llm-api-key-secret", "", "unsupported; configure provider env vars on the Flue deployment")
	cmd.Flags().StringVar(&opts.AnthropicAPIKeySecret, "anthropic-api-key-secret", "", "unsupported; configure ANTHROPIC_API_KEY on the Flue deployment")
	cmd.Flags().StringVar(&opts.OpenAIAPIKeySecret, "openai-api-key-secret", "", "unsupported; configure OPENAI_API_KEY on the Flue deployment")
	cmd.Flags().StringVar(&opts.TesterURL, "tester-url", "", "base URL of a deployed tester Flue app to save in config")
	cmd.Flags().StringVar(&opts.EditorURL, "editor-url", "", "base URL of a deployed editor Flue app to save in config")
	cmd.Flags().StringVar(&opts.AgentsDir, "agents-dir", "", "where to extract Flue apps (default: <repo>/.dari-docs/agents)")
	_ = cmd.Flags().MarkHidden("deploy")
	_ = cmd.Flags().MarkHidden("api-key-env")
	_ = cmd.Flags().MarkHidden("api-key")
	_ = cmd.Flags().MarkHidden("api-base-url")
	_ = cmd.Flags().MarkHidden("llm-api-key-secret")
	_ = cmd.Flags().MarkHidden("anthropic-api-key-secret")
	_ = cmd.Flags().MarkHidden("openai-api-key-secret")
	return cmd
}

func runInitWithOptions(ctx context.Context, opts initOptions) error {
	_ = ctx
	repoArg := opts.RepoArg
	if repoArg == "" {
		repoArg = "."
	}
	absRepo, err := filepath.Abs(repoArg)
	if err != nil {
		return err
	}
	if opts.Deploy || opts.APIKeyEnv != "" || opts.APIKey != "" || opts.APIBaseURL != "" || opts.LLMAPIKeySecret != "" || opts.AnthropicAPIKeySecret != "" || opts.OpenAIAPIKeySecret != "" {
		return fmt.Errorf("Dari deploy/credential flags are not supported; deploy the extracted Flue apps with `flue build` and configure provider env vars on that deployment")
	}

	agentsDir := opts.AgentsDir
	if agentsDir == "" {
		agentsDir = filepath.Join(absRepo, ".dari-docs", "agents")
	}
	if err := agenttemplates.Extract(agentsDir); err != nil {
		return err
	}
	fmt.Printf("Extracted bundled Flue apps to %s\n", agentsDir)

	cfg := projectconfig.Config{AgentsDir: agentsDir, AgentRuntime: "flue", LLMMode: "flue-env", TesterURL: opts.TesterURL, EditorURL: opts.EditorURL}
	if err := projectconfig.Save(absRepo, cfg); err != nil {
		return err
	}
	fmt.Printf("Wrote %s\n", projectconfig.Path(absRepo))
	fmt.Println("Deploy the Flue apps with `flue build --target node` and your Node host, then run `dari-docs check --tester-url <url>`.")
	return nil
}
