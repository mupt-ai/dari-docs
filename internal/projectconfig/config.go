// Package projectconfig reads and writes the per-repository .dari-docs/config.json file.
//
// That file is created by `dari-docs init` and stores local project metadata,
// such as where bundled Flue agent folders were extracted and the deployment URLs to use
// by default. Commands like `dari-docs check` and `dari-docs optimize` use it so
// users do not have to pass URLs on every run.
package projectconfig

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// Config is the persisted contents of .dari-docs/config.json.
type Config struct {
	TesterAgentID    string            `json:"tester_agent_id,omitempty"`
	EditorAgentID    string            `json:"editor_agent_id,omitempty"`
	TesterURL        string            `json:"tester_url,omitempty"`
	EditorURL        string            `json:"editor_url,omitempty"`
	AgentsDir        string            `json:"agents_dir"`
	AgentRuntime     string            `json:"agent_runtime,omitempty"`
	LLMMode          string            `json:"llm_mode,omitempty"`
	LLMAPIKeySecret  string            `json:"llm_api_key_secret,omitempty"`
	LLMAPIKeySecrets map[string]string `json:"llm_api_key_secrets,omitempty"`
}

// Path returns the .dari-docs/config.json path for repoRoot.
func Path(repoRoot string) string { return filepath.Join(repoRoot, ".dari-docs", "config.json") }

func Load(repoRoot string) (Config, bool, error) {
	p := Path(repoRoot)
	b, err := os.ReadFile(p)
	if os.IsNotExist(err) {
		return Config{}, false, nil
	}
	if err != nil {
		return Config{}, false, err
	}
	var c Config
	if err := json.Unmarshal(b, &c); err != nil {
		return Config{}, false, err
	}
	return c, true, nil
}

func Save(repoRoot string, c Config) error {
	p := Path(repoRoot)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')
	return os.WriteFile(p, b, 0o644)
}
