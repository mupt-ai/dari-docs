package workspace

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

func CopyTree(src, dst string) error {
	absSrc, err := filepath.Abs(src)
	if err != nil {
		return err
	}
	absDst, err := filepath.Abs(dst)
	if err != nil {
		return err
	}
	return filepath.WalkDir(absSrc, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(absSrc, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		if strings.HasPrefix(rel, "../") || filepath.IsAbs(rel) {
			return fmt.Errorf("unsafe path %q", rel)
		}
		out := filepath.Join(absDst, rel)
		if d.IsDir() {
			if err := ensureNoDestinationSymlink(absDst, out); err != nil {
				return err
			}
			return os.MkdirAll(out, 0o755)
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		if err := ensureNoDestinationSymlink(absDst, filepath.Dir(out)); err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
			return err
		}
		if err := ensureNoDestinationSymlink(absDst, out); err != nil {
			return err
		}
		return copyFile(path, out, info.Mode())
	})
}

func ensureNoDestinationSymlink(root, target string) error {
	rel, err := filepath.Rel(root, target)
	if err != nil {
		return err
	}
	if rel == "." {
		return nil
	}
	if strings.HasPrefix(rel, "../") || filepath.IsAbs(rel) {
		return fmt.Errorf("unsafe destination path %q", rel)
	}

	current := root
	for _, part := range strings.Split(rel, string(os.PathSeparator)) {
		if part == "" || part == "." {
			continue
		}
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("refusing to apply through destination symlink %q", current)
		}
	}
	return nil
}

func copyFile(src, dst string, mode os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(out, in)
	closeErr := out.Close()
	if copyErr != nil {
		return copyErr
	}
	return closeErr
}

func UpdatedRoot(extractDir string) (string, error) {
	candidates := []string{
		filepath.Join(extractDir, "updated-docs", "files"),
		filepath.Join(extractDir, "workspace", "updated-docs", "files"),
	}
	for _, c := range candidates {
		if st, err := os.Stat(c); err == nil && st.IsDir() {
			return c, nil
		}
	}
	return "", fmt.Errorf("updated docs archive missing expected updated-docs/files directory")
}
