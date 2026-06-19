package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

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
	AgentsDir             string
}

func newInitCommand() *cobra.Command {
	opts := initOptions{APIKeyEnv: "DARI_API_KEY", APIBaseURL: os.Getenv("DARI_API_URL")}
	cmd := &cobra.Command{
		Use:           "init [repo]",
		Short:         "Extract or deploy bundled agents",
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
	cmd.Flags().BoolVar(&opts.Deploy, "deploy", false, "deploy bundled agents into the current Dari org")
	cmd.Flags().StringVar(&opts.APIKeyEnv, "api-key-env", opts.APIKeyEnv, "env var containing Dari API key for deploy")
	cmd.Flags().StringVar(&opts.APIKey, "api-key", "", "Dari API key for deploy (prefer --api-key-env)")
	cmd.Flags().StringVar(&opts.APIBaseURL, "api-base-url", opts.APIBaseURL, "Dari API base URL for deploy (defaults to production or $DARI_API_URL)")
	cmd.Flags().StringVar(&opts.LLMAPIKeySecret, "llm-api-key-secret", "", "optional stored Dari credential name for the default Anthropic provider at agent publish time")
	cmd.Flags().StringVar(&opts.AnthropicAPIKeySecret, "anthropic-api-key-secret", "", "optional stored Dari credential name for Anthropic BYOK LLM at agent publish time")
	cmd.Flags().StringVar(&opts.OpenAIAPIKeySecret, "openai-api-key-secret", "", "optional stored Dari credential name for OpenAI BYOK LLM at agent publish time")
	cmd.Flags().StringVar(&opts.AgentsDir, "agents-dir", "", "where to extract agent templates (default: <repo>/.dari-docs/agents)")
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
	agentsDir := opts.AgentsDir
	if agentsDir == "" {
		agentsDir = filepath.Join(absRepo, ".dari-docs", "agents")
	}
	if err := agenttemplates.Extract(agentsDir); err != nil {
		return err
	}
	fmt.Printf("Extracted bundled agents to %s\n", agentsDir)

	providerSecrets := map[string]string{}
	if opts.AnthropicAPIKeySecret != "" {
		providerSecrets["anthropic"] = opts.AnthropicAPIKeySecret
	}
	if opts.OpenAIAPIKeySecret != "" {
		providerSecrets["openai"] = opts.OpenAIAPIKeySecret
	}
	if opts.LLMAPIKeySecret != "" && len(providerSecrets) > 0 {
		return fmt.Errorf("--llm-api-key-secret cannot be combined with provider-specific LLM key secret flags")
	}

	cfg := projectconfig.Config{AgentsDir: agentsDir, AgentRuntime: "flue", LLMMode: "flue-provider-secrets", LLMAPIKeySecret: opts.LLMAPIKeySecret}
	if opts.LLMAPIKeySecret != "" {
		cfg.LLMMode = "byok-publish-time"
		if err := setAgentDefaultProviderSecret(filepath.Join(agentsDir, "docs-user-tester-agent", "dari.yml"), opts.LLMAPIKeySecret); err != nil {
			return err
		}
		if err := setAgentDefaultProviderSecret(filepath.Join(agentsDir, "docs-editor-agent", "dari.yml"), opts.LLMAPIKeySecret); err != nil {
			return err
		}
	}
	if len(providerSecrets) > 0 {
		cfg.LLMMode = "byok-publish-time"
		cfg.LLMAPIKeySecrets = providerSecrets
		if err := setAgentProviderSecrets(filepath.Join(agentsDir, "docs-user-tester-agent", "dari.yml"), providerSecrets); err != nil {
			return err
		}
		if err := setAgentProviderSecrets(filepath.Join(agentsDir, "docs-editor-agent", "dari.yml"), providerSecrets); err != nil {
			return err
		}
	}
	if opts.Deploy {
		apiKey := opts.APIKey
		if apiKey == "" && opts.APIKeyEnv != "" {
			apiKey = os.Getenv(opts.APIKeyEnv)
		}
		if apiKey == "" {
			return fmt.Errorf("missing Dari API key for deploy; set %s or pass --api-key", opts.APIKeyEnv)
		}
		apiBaseURL := strings.TrimSpace(opts.APIBaseURL)
		if apiBaseURL == "" {
			apiBaseURL = "https://api.dari.dev"
		}
		env := append(os.Environ(), "DARI_API_URL="+apiBaseURL, "DARI_API_KEY="+apiKey)
		testerID, err := deployAgent(env, filepath.Join(agentsDir, "docs-user-tester-agent"))
		if err != nil {
			return err
		}
		editorID, err := deployAgent(env, filepath.Join(agentsDir, "docs-editor-agent"))
		if err != nil {
			return err
		}
		cfg.TesterAgentID = testerID
		cfg.EditorAgentID = editorID
		fmt.Printf("Deployed tester agent: %s\n", testerID)
		fmt.Printf("Deployed editor agent: %s\n", editorID)
	}
	if err := projectconfig.Save(absRepo, cfg); err != nil {
		return err
	}
	fmt.Printf("Wrote %s\n", projectconfig.Path(absRepo))
	if !opts.Deploy {
		fmt.Println("Run `dari-docs init --deploy` to deploy these agents into your Dari org.")
	}
	return nil
}

func deployAgent(env []string, dir string) (string, error) {
	cmd := exec.Command("dari", "deploy", "--quiet", ".")
	cmd.Dir = dir
	cmd.Env = env
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("deploy %s: %w\n%s", dir, err, stderr.String()+stdout.String())
	}
	var resp struct {
		AgentID string `json:"agent_id"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &resp); err != nil || resp.AgentID == "" {
		return "", fmt.Errorf("deploy %s: could not parse agent_id from output: %s", dir, stdout.String())
	}
	return resp.AgentID, nil
}
