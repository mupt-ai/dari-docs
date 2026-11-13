package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/mupt-ai/dari-docs/internal/config"
)

// A self-bench login's key only goes to the deployment it was made for.
func TestConnectionUsesTheLoginOnlyForItsOwnDeployment(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("SELFBENCH_API_KEY", "")
	t.Setenv("SELFBENCH_ORG", "")
	t.Setenv("SELFBENCH_API_URL", "")
	if err := os.MkdirAll(filepath.Join(dir, "self-bench"), 0o700); err != nil {
		t.Fatal(err)
	}
	login := `{"apiUrl":"https://app.selfbench.dev","apiKey":"sb-login","org":"acme"}`
	if err := os.WriteFile(filepath.Join(dir, "self-bench", "config.json"), []byte(login), 0o600); err != nil {
		t.Fatal(err)
	}

	c, err := connection(config.Config{})
	if err != nil || c.APIKey != "sb-login" || c.Org != "acme" || c.BaseURL != "https://app.selfbench.dev" {
		t.Fatalf("hosted: %+v, %v; want the login's key and workspace", c, err)
	}
	if c, err := connection(config.Config{APIURL: "https://selfbench.internal.example"}); err == nil {
		t.Fatalf("self-hosted config got the hosted login's key: %+v", c)
	}
	t.Setenv("SELFBENCH_API_KEY", "sb-env")
	t.Setenv("SELFBENCH_ORG", "other")
	if c, _ := connection(config.Config{Org: "configured"}); c.APIKey != "sb-env" || c.Org != "other" {
		t.Errorf("environment: %+v; SELFBENCH_API_KEY and SELFBENCH_ORG come first", c)
	}
}
