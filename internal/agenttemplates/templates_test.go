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

	for _, name := range []string{"docs-user-tester-agent", "docs-editor-agent"} {
		agentDir := filepath.Join(dir, name)
		manifest := readText(t, filepath.Join(agentDir, "dari.yml"))
		if !strings.Contains(manifest, "name: "+name) {
			t.Fatalf("%s manifest missing name:\n%s", name, manifest)
		}
		for _, legacy := range []string{"harness:", "llm:", "built_in_tools:", "internet_access:"} {
			if strings.Contains(manifest, legacy) {
				t.Fatalf("%s manifest still contains legacy field %q:\n%s", name, legacy, manifest)
			}
		}
		if !strings.Contains(manifest, "sandbox:") || !strings.Contains(manifest, "secrets:") {
			t.Fatalf("%s manifest missing Flue sandbox secret declaration:\n%s", name, manifest)
		}
		entry := filepath.Join(agentDir, "agents", name+".ts")
		if _, err := os.Stat(entry); err != nil {
			t.Fatalf("%s missing Flue entrypoint: %v", name, err)
		}
		pkg := readText(t, filepath.Join(agentDir, "package.json"))
		if !strings.Contains(pkg, "@flue/runtime") || !strings.Contains(pkg, "@flue/cli") {
			t.Fatalf("%s package.json missing Flue dependencies:\n%s", name, pkg)
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
