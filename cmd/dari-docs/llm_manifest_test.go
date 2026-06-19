package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSetAgentProviderSecretsUpdatesFlueManifest(t *testing.T) {
	path := filepath.Join(t.TempDir(), "dari.yml")
	if err := os.WriteFile(path, []byte(strings.Join([]string{
		"name: docs-user-tester-agent",
		"sandbox:",
		"  env:",
		"    DARI_DOCS_DEFAULT_MODEL: anthropic/claude-sonnet-4-6",
		"    DARI_DOCS_ANTHROPIC_API_KEY_SECRET_NAME: ANTHROPIC_API_KEY",
		"  secrets:",
		"    - ANTHROPIC_API_KEY",
		"",
	}, "\n")), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := setAgentProviderSecrets(path, map[string]string{"anthropic": "TEAM_ANTHROPIC_KEY", "openai": "TEAM_OPENAI_KEY"}); err != nil {
		t.Fatal(err)
	}

	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	got := string(b)
	for _, want := range []string{
		"DARI_DOCS_ANTHROPIC_API_KEY_SECRET_NAME: TEAM_ANTHROPIC_KEY",
		"DARI_DOCS_OPENAI_API_KEY_SECRET_NAME: TEAM_OPENAI_KEY",
		"- TEAM_ANTHROPIC_KEY",
		"- TEAM_OPENAI_KEY",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("manifest missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "- ANTHROPIC_API_KEY") {
		t.Fatalf("manifest kept old default secret:\n%s", got)
	}
}
