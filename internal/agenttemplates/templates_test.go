package agenttemplates

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExtractedAgentsAreHarborTemplates(t *testing.T) {
	dir := t.TempDir()
	if err := Extract(dir); err != nil {
		t.Fatal(err)
	}
	for _, rel := range []string{
		"modal_app.py", "harbor_runner.py", "runner_api.py", "requirements.txt", "Dockerfile",
		"docs-user-tester-agent/README.md", "docs-user-tester-agent/prompts/system.md", "docs-user-tester-agent/skills/docs-user-test/SKILL.md",
		"docs-editor-agent/README.md", "docs-editor-agent/prompts/system.md", "docs-editor-agent/skills/documentation-editor/SKILL.md",
	} {
		if _, err := os.Stat(filepath.Join(dir, filepath.FromSlash(rel))); err != nil {
			t.Fatalf("missing extracted template %s: %v", rel, err)
		}
	}
	if requirements := readText(t, filepath.Join(dir, "requirements.txt")); !strings.Contains(requirements, "harbor[modal]==0.24.0") {
		t.Fatalf("Harbor must be pinned: %s", requirements)
	}
	modalApp := readText(t, filepath.Join(dir, "modal_app.py"))
	for _, marker := range []string{"modal.App", "modal.Dict.from_name", "@modal.asgi_app", "execute.spawn.aio"} {
		if !strings.Contains(modalApp, marker) {
			t.Fatalf("Modal template missing %s", marker)
		}
	}
	if !strings.Contains(readText(t, filepath.Join(dir, "harbor_runner.py")), "Trial.create") {
		t.Fatal("runner must execute Harbor trials")
	}
	assertNoGeneratedTemplateJunk(t, dir)
}

func TestExtractRemovesLegacyRuntimePreservesCustomFiles(t *testing.T) {
	dir := t.TempDir()
	legacy := []string{
		"docs-user-tester-agent/.flue/workflows/test.ts",
		"docs-user-tester-agent/node_modules/pkg/index.js",
		"docs-user-tester-agent/dist/server.mjs",
		"docs-user-tester-agent/workflows/test.ts",
		"docs-user-tester-agent/agents/docs-user-tester-agent.ts",
		"docs-editor-agent/package.json", "docs-editor-agent/bun.lock", "docs-editor-agent/flue.config.ts",
		"docs-editor-agent/app.ts", "docs-editor-agent/workflows/edit.ts",
	}
	custom := "docs-editor-agent/skills/custom/SKILL.md"
	prompt := "docs-editor-agent/prompts/system.md"
	for _, rel := range append(legacy, custom, prompt) {
		path := filepath.Join(dir, filepath.FromSlash(rel))
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
	for _, rel := range legacy {
		if _, err := os.Stat(filepath.Join(dir, filepath.FromSlash(rel))); !os.IsNotExist(err) {
			t.Fatalf("legacy path %s still exists: %v", rel, err)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, filepath.FromSlash(custom))); err != nil {
		t.Fatalf("custom skill should be preserved: %v", err)
	}
	if got := readText(t, filepath.Join(dir, filepath.FromSlash(prompt))); got != "stale\n" {
		t.Fatalf("customized bundled prompt overwritten: %q", got)
	}
}

func TestValidateTemplatePathRejectsGeneratedJunk(t *testing.T) {
	for _, path := range []string{"__pycache__/runner.pyc", ".venv/bin/python", "node_modules/index.js", ".env", "dist/server.py", ".flue/test.ts"} {
		if err := validateTemplatePath(path); err == nil {
			t.Fatalf("expected rejection of %s", path)
		}
	}
	if err := validateTemplatePath("harbor_runner.py"); err != nil {
		t.Fatal(err)
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
