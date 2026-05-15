package config

import (
	"encoding/json"
	"os"
	"path/filepath"
)

type Config struct {
	TesterAgentID    string            `json:"tester_agent_id"`
	EditorAgentID    string            `json:"editor_agent_id"`
	AgentsDir        string            `json:"agents_dir"`
	LLMMode          string            `json:"llm_mode,omitempty"`
	LLMAPIKeySecret  string            `json:"llm_api_key_secret,omitempty"`
	LLMAPIKeySecrets map[string]string `json:"llm_api_key_secrets,omitempty"`
}

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
