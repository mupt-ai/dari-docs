package agenttemplates

import (
	"embed"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

//go:embed templates/**
var templates embed.FS

func Extract(dest string) error {
	return fs.WalkDir(templates, "templates", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == "templates" {
			return nil
		}
		rel := strings.TrimPrefix(path, "templates/")
		out := filepath.Join(dest, filepath.FromSlash(rel))
		if d.IsDir() {
			return os.MkdirAll(out, 0o755)
		}
		b, err := templates.ReadFile(path)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
			return err
		}
		mode := os.FileMode(0o644)
		if strings.HasSuffix(out, ".sh") {
			mode = 0o755
		}
		return os.WriteFile(out, b, mode)
	})
}
