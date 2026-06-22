package agenttemplates

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExtractedAgentsAreFlueProjects(t *testing.T) {
	dir := t.TempDir()
	if err := Extract(dir); err != nil {
		t.Fatal(err)
	}
	modalApp := readText(t, filepath.Join(dir, "modal_app.py"))
	if !strings.Contains(modalApp, "modal.App") || !strings.Contains(modalApp, "modal.Sandbox.create") || !strings.Contains(modalApp, "@modal.asgi_app") || !strings.Contains(modalApp, "encrypted_ports=[PORT]") {
		t.Fatalf("modal_app.py missing Modal sandbox gateway setup:\n%s", modalApp)
	}

	for _, tt := range []struct {
		name     string
		workflow string
	}{
		{name: "docs-user-tester-agent", workflow: "test.ts"},
		{name: "docs-editor-agent", workflow: "edit.ts"},
	} {
		agentDir := filepath.Join(dir, tt.name)
		if _, err := os.Stat(filepath.Join(agentDir, "dari.yml")); !os.IsNotExist(err) {
			t.Fatalf("%s should not include a Dari deploy manifest, stat err=%v", tt.name, err)
		}
		config := readText(t, filepath.Join(agentDir, "flue.config.ts"))
		if !strings.Contains(config, "defineConfig") || !strings.Contains(config, "target: 'node'") {
			t.Fatalf("%s missing node Flue config:\n%s", tt.name, config)
		}
		entry := filepath.Join(agentDir, "agents", tt.name+".ts")
		if _, err := os.Stat(entry); err != nil {
			t.Fatalf("%s missing Flue entrypoint: %v", tt.name, err)
		}
		if _, err := os.Stat(filepath.Join(agentDir, ".flue", "app.ts")); err != nil {
			t.Fatalf("%s missing Flue app entrypoint: %v", tt.name, err)
		}
		workflow := filepath.Join(agentDir, ".flue", "workflows", tt.workflow)
		if _, err := os.Stat(workflow); err != nil {
			t.Fatalf("%s missing Flue workflow %s: %v", tt.name, tt.workflow, err)
		}
		pkg := readText(t, filepath.Join(agentDir, "package.json"))
		if !strings.Contains(pkg, "@flue/runtime") || !strings.Contains(pkg, "@flue/cli") || !strings.Contains(pkg, "valibot") || !strings.Contains(pkg, "bun@") {
			t.Fatalf("%s package.json missing Flue/Bun dependencies:\n%s", tt.name, pkg)
		}
		if _, err := os.Stat(filepath.Join(agentDir, "bun.lock")); err != nil {
			t.Fatalf("%s missing Bun lockfile: %v", tt.name, err)
		}
		if _, err := os.Stat(filepath.Join(agentDir, "package-lock.json")); !os.IsNotExist(err) {
			t.Fatalf("%s should not embed npm package-lock.json, stat err=%v", tt.name, err)
		}
		if _, err := os.Stat(filepath.Join(agentDir, "node_modules")); !os.IsNotExist(err) {
			t.Fatalf("%s should not embed node_modules, stat err=%v", tt.name, err)
		}
	}
}

func readText(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
