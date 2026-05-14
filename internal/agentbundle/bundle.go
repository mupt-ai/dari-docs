package agentbundle

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"time"
)

type Bundle struct {
	Content []byte
	SHA256  string
	Files   []string
}

var excludedDirs = map[string]struct{}{
	".dari":         {},
	".git":          {},
	".pytest_cache": {},
	".venv":         {},
	"__pycache__":   {},
	"node_modules":  {},
}

func Build(root string) (Bundle, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return Bundle{}, err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return Bundle{}, err
	}
	if !info.IsDir() {
		return Bundle{}, fmt.Errorf("%s is not a directory", root)
	}
	files, err := selectedFiles(abs)
	if err != nil {
		return Bundle{}, err
	}
	if !contains(files, "dari.yml") {
		return Bundle{}, errors.New("agent project must contain a top-level dari.yml file")
	}
	var b bytes.Buffer
	gzw, err := gzip.NewWriterLevel(&b, gzip.BestCompression)
	if err != nil {
		return Bundle{}, err
	}
	gzw.ModTime = time.Time{}
	gzw.Name = ""
	gzw.Comment = ""
	gzw.OS = 0xff
	tw := tar.NewWriter(gzw)
	for _, rel := range files {
		if err := addFile(tw, abs, rel); err != nil {
			_ = tw.Close()
			_ = gzw.Close()
			return Bundle{}, err
		}
	}
	if err := tw.Close(); err != nil {
		return Bundle{}, err
	}
	if err := gzw.Close(); err != nil {
		return Bundle{}, err
	}
	content := b.Bytes()
	sum := sha256.Sum256(content)
	return Bundle{Content: content, SHA256: hex.EncodeToString(sum[:]), Files: files}, nil
}

func selectedFiles(root string) ([]string, error) {
	var out []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == root {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if d.IsDir() {
			if _, ok := excludedDirs[d.Name()]; ok {
				return filepath.SkipDir
			}
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		if d.Name() == ".DS_Store" {
			return nil
		}
		out = append(out, rel)
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(out)
	return out, nil
}

func addFile(tw *tar.Writer, root, rel string) error {
	path := filepath.Join(root, filepath.FromSlash(rel))
	lstat, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !lstat.Mode().IsRegular() {
		return fmt.Errorf("refusing to bundle non-regular file %s", rel)
	}
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("refusing to bundle non-regular file %s", rel)
	}
	hdr, err := tar.FileInfoHeader(info, "")
	if err != nil {
		return err
	}
	hdr.Name = rel
	hdr.ModTime = time.Time{}
	hdr.AccessTime = time.Time{}
	hdr.ChangeTime = time.Time{}
	hdr.Uid = 0
	hdr.Gid = 0
	hdr.Uname = ""
	hdr.Gname = ""
	if err := tw.WriteHeader(hdr); err != nil {
		return err
	}
	_, err = io.Copy(tw, f)
	return err
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
