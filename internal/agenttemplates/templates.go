package agenttemplates

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/mupt-ai/dari-docs/agents"
)

func Extract(dest string) error {
	return fs.WalkDir(agents.FS, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == "." {
			return nil
		}
		out := filepath.Join(dest, filepath.FromSlash(path))
		if d.IsDir() {
			return os.MkdirAll(out, 0o755)
		}
		b, err := agents.FS.ReadFile(path)
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
