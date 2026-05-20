package agenttemplates

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/mupt-ai/dari-docs/agents"
)

const (
	dirMode        os.FileMode = 0o755
	templateMode   os.FileMode = 0o644
	executableMode os.FileMode = 0o755
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
			return os.MkdirAll(out, dirMode)
		}

		b, err := agents.FS.ReadFile(path)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(out), dirMode); err != nil {
			return err
		}
		return os.WriteFile(out, b, templateFileMode(path))
	})
}

func templateFileMode(path string) os.FileMode {
	if strings.HasSuffix(path, ".sh") {
		return executableMode
	}
	return templateMode
}
