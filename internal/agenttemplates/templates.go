package agenttemplates

import (
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/mupt-ai/dari-docs/agents"
)

const (
	dirMode        os.FileMode = 0o755
	templateMode   os.FileMode = 0o644
	executableMode os.FileMode = 0o755
)

var bundledAgentDirs = []string{
	"docs-user-tester-agent",
	"docs-editor-agent",
}

var generatedTemplateParts = map[string]struct{}{
	".dari":             {},
	".dari-docs":        {},
	".flue":             {},
	".flue-vite":        {},
	".next":             {},
	".turbo":            {},
	"build":             {},
	"coverage":          {},
	"dist":              {},
	"node_modules":      {},
	"package-lock.json": {},
	"bun.lockb":         {},
	"__pycache__":       {},
	".pytest_cache":     {},
	".venv":             {},
}

func Extract(dest string) error {
	if err := cleanGeneratedTemplateJunk(dest); err != nil {
		return err
	}
	return fs.WalkDir(agents.FS, ".", func(templatePath string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if templatePath == "." {
			return nil
		}
		if err := validateTemplatePath(templatePath); err != nil {
			return err
		}

		out := filepath.Join(dest, filepath.FromSlash(templatePath))
		if d.IsDir() {
			return os.MkdirAll(out, dirMode)
		}

		// User instructions survive runtime upgrades and repeated initialization.
		if strings.Contains(templatePath, "/prompts/") || strings.Contains(templatePath, "/skills/") {
			if _, err := os.Stat(out); err == nil {
				return nil
			} else if !os.IsNotExist(err) {
				return err
			}
		}
		b, err := agents.FS.ReadFile(templatePath)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(out), dirMode); err != nil {
			return err
		}
		return os.WriteFile(out, b, templateFileMode(templatePath))
	})
}

func cleanGeneratedTemplateJunk(dest string) error {
	for _, agentDir := range bundledAgentDirs {
		root := filepath.Join(dest, agentDir)
		if _, err := os.Stat(root); os.IsNotExist(err) {
			continue
		} else if err != nil {
			return err
		}
		for part := range generatedTemplateParts {
			if err := os.RemoveAll(filepath.Join(root, part)); err != nil {
				return err
			}
		}
		// Remove only known former runtime files, preserving custom prompts/skills.
		for _, rel := range []string{"package.json", "bun.lock", "flue.config.ts", "app.ts", "agents/" + agentDir + ".ts", "workflows/test.ts", "workflows/edit.ts"} {
			if err := os.RemoveAll(filepath.Join(root, filepath.FromSlash(rel))); err != nil {
				return err
			}
		}
	}
	return nil
}

func validateTemplatePath(templatePath string) error {
	for _, part := range strings.Split(templatePath, "/") {
		if _, ok := generatedTemplateParts[part]; ok {
			return fmt.Errorf("bundled agent template includes generated or hidden runtime path %q", templatePath)
		}
	}
	base := path.Base(templatePath)
	if base == ".env" || base == ".env.local" {
		return fmt.Errorf("bundled agent template includes environment file %q", templatePath)
	}
	return nil
}

func templateFileMode(path string) os.FileMode {
	if strings.HasSuffix(path, ".sh") {
		return executableMode
	}
	return templateMode
}
