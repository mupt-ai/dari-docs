package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/mupt-ai/dari-docs/internal/agenttemplates"
	appconfig "github.com/mupt-ai/dari-docs/internal/config"
)

func runInit(args []string) error {
	fs := flag.NewFlagSet("dari-docs init", flag.ExitOnError)
	var deploy bool
	var apiKeyEnv, apiKey, llmAPIKeySecret, anthropicAPIKeySecret, openAIAPIKeySecret, agentsDir string
	fs.BoolVar(&deploy, "deploy", false, "deploy bundled agents into the current Dari org")
	fs.StringVar(&apiKeyEnv, "api-key-env", "DARI_API_KEY", "env var containing Dari API key for deploy")
	fs.StringVar(&apiKey, "api-key", "", "Dari API key for deploy (prefer --api-key-env)")
	fs.StringVar(&llmAPIKeySecret, "llm-api-key-secret", "", "optional stored Dari credential name for BYOK LLM at agent publish time; only valid when all LLM options use one provider")
	fs.StringVar(&anthropicAPIKeySecret, "anthropic-api-key-secret", "", "optional stored Dari credential name for Anthropic BYOK LLM at agent publish time")
	fs.StringVar(&openAIAPIKeySecret, "openai-api-key-secret", "", "optional stored Dari credential name for OpenAI BYOK LLM at agent publish time")
	fs.StringVar(&agentsDir, "agents-dir", "", "where to extract agent templates (default: <repo>/.dari-docs/agents)")
	repoArg := "."
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		repoArg = args[0]
		args = args[1:]
	}
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() > 0 {
		repoArg = fs.Arg(0)
	}
	absRepo, err := filepath.Abs(repoArg)
	if err != nil {
		return err
	}
	if agentsDir == "" {
		agentsDir = filepath.Join(absRepo, ".dari-docs", "agents")
	}
	if err := agenttemplates.Extract(agentsDir); err != nil {
		return err
	}
	fmt.Printf("Extracted bundled agents to %s\n", agentsDir)

	providerSecrets := map[string]string{}
	if anthropicAPIKeySecret != "" {
		providerSecrets["anthropic"] = anthropicAPIKeySecret
	}
	if openAIAPIKeySecret != "" {
		providerSecrets["openai"] = openAIAPIKeySecret
	}
	if llmAPIKeySecret != "" && len(providerSecrets) > 0 {
		return fmt.Errorf("--llm-api-key-secret cannot be combined with provider-specific LLM key secret flags")
	}

	cfg := appconfig.Config{AgentsDir: agentsDir, LLMMode: "platform-managed", LLMAPIKeySecret: llmAPIKeySecret}
	if llmAPIKeySecret != "" {
		cfg.LLMMode = "byok-publish-time"
		if err := setLLMAPIKeySecret(filepath.Join(agentsDir, "docs-user-tester-agent", "dari.yml"), llmAPIKeySecret); err != nil {
			return err
		}
		if err := setLLMAPIKeySecret(filepath.Join(agentsDir, "docs-editor-agent", "dari.yml"), llmAPIKeySecret); err != nil {
			return err
		}
	}
	if len(providerSecrets) > 0 {
		cfg.LLMMode = "byok-publish-time"
		cfg.LLMAPIKeySecrets = providerSecrets
		if err := setLLMAPIKeySecretsByProvider(filepath.Join(agentsDir, "docs-user-tester-agent", "dari.yml"), providerSecrets); err != nil {
			return err
		}
		if err := setLLMAPIKeySecretsByProvider(filepath.Join(agentsDir, "docs-editor-agent", "dari.yml"), providerSecrets); err != nil {
			return err
		}
	}
	if deploy {
		if apiKey == "" && apiKeyEnv != "" {
			apiKey = os.Getenv(apiKeyEnv)
		}
		if apiKey == "" {
			return fmt.Errorf("missing Dari API key for deploy; set %s or pass --api-key", apiKeyEnv)
		}
		env := append(os.Environ(), "DARI_API_URL=https://api.dari.dev", "DARI_API_KEY="+apiKey)
		ensureCredential(env, "DARI_DOCS_RUNTIME_SECRETS_JSON", "{}")
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
	if err := appconfig.Save(absRepo, cfg); err != nil {
		return err
	}
	fmt.Printf("Wrote %s\n", appconfig.Path(absRepo))
	if !deploy {
		fmt.Println("Run `dari-docs init --deploy` to deploy these agents into your Dari org.")
	}
	return nil
}

func ensureCredential(env []string, name, value string) {
	cmd := exec.Command("dari", "credentials", "add", name, value)
	cmd.Env = env
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "warning: could not create credential %s (it may already exist): %s\n", name, strings.TrimSpace(out.String()))
	}
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
