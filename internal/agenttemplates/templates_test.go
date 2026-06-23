package agenttemplates

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExtractedAgentsAreSimpleFlueProjects(t *testing.T) {
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
		config := readText(t, filepath.Join(agentDir, "flue.config.ts"))
		if !strings.Contains(config, "defineConfig") || !strings.Contains(config, "target: 'node'") {
			t.Fatalf("%s missing node Flue config:\n%s", tt.name, config)
		}
		if _, err := os.Stat(filepath.Join(agentDir, "app.ts")); err != nil {
			t.Fatalf("%s missing visible Flue app entrypoint: %v", tt.name, err)
		}
		entry := filepath.Join(agentDir, "agents", tt.name+".ts")
		if _, err := os.Stat(entry); err != nil {
			t.Fatalf("%s missing Flue entrypoint: %v", tt.name, err)
		}
		workflow := filepath.Join(agentDir, "workflows", tt.workflow)
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
		assertNoGeneratedTemplateJunk(t, agentDir)
	}
}

func TestExtractRemovesStaleGeneratedJunk(t *testing.T) {
	dir := t.TempDir()
	stalePaths := []string{
		filepath.Join("docs-user-tester-agent", ".flue", "workflows", "test.ts"),
		filepath.Join("docs-user-tester-agent", "node_modules", "pkg", "index.js"),
		filepath.Join("docs-user-tester-agent", "dist", "server.mjs"),
		filepath.Join("docs-editor-agent", ".flue", "workflows", "edit.ts"),
		filepath.Join("docs-editor-agent", ".flue-vite", "_entry.ts"),
		filepath.Join("docs-editor-agent", "package-lock.json"),
	}
	for _, rel := range stalePaths {
		path := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("stale\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	if err := Extract(dir); err != nil {
		t.Fatal(err)
	}

	for _, rel := range stalePaths {
		if _, err := os.Stat(filepath.Join(dir, rel)); !os.IsNotExist(err) {
			t.Fatalf("stale generated path %s still exists, stat err=%v", rel, err)
		}
	}
	for _, rel := range []string{
		filepath.Join("docs-user-tester-agent", "workflows", "test.ts"),
		filepath.Join("docs-editor-agent", "workflows", "edit.ts"),
	} {
		if _, err := os.Stat(filepath.Join(dir, rel)); err != nil {
			t.Fatalf("missing extracted source %s: %v", rel, err)
		}
	}
}

func TestValidateTemplatePathRejectsGeneratedJunk(t *testing.T) {
	tests := []struct {
		name    string
		path    string
		wantErr bool
	}{
		{name: "source file", path: "docs-user-tester-agent/workflows/test.ts"},
		{name: "hidden flue directory", path: "docs-user-tester-agent/.flue/workflows/test.ts", wantErr: true},
		{name: "node modules", path: "docs-user-tester-agent/node_modules/pkg/index.js", wantErr: true},
		{name: "build output", path: "docs-user-tester-agent/dist/server.mjs", wantErr: true},
		{name: "flue vite output", path: "docs-user-tester-agent/.flue-vite/_entry.ts", wantErr: true},
		{name: "npm lock", path: "docs-user-tester-agent/package-lock.json", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateTemplatePath(tt.path)
			if tt.wantErr && err == nil {
				t.Fatal("expected error")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func assertNoGeneratedTemplateJunk(t *testing.T, root string) {
	t.Helper()
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil || rel == "." {
			return err
		}
		return validateTemplatePath(filepath.ToSlash(rel))
	})
	if err != nil {
		t.Fatal(err)
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
