package bundle

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type FileRecord struct {
	Path        string `json:"path"`
	SizeBytes   int64  `json:"size_bytes"`
	SHA256      string `json:"sha256"`
	ContentType string `json:"content_type,omitempty"`
}

type Manifest struct {
	SchemaVersion int          `json:"schema_version"`
	CreatedAt     string       `json:"created_at"`
	RepoRoot      string       `json:"repo_root"`
	Files         []FileRecord `json:"files"`
}

type Result struct {
	Path     string
	Manifest Manifest
	SHA256   string
	Bytes    int64
}

var defaultSkipDirs = map[string]bool{
	".git": true, "node_modules": true, ".dari-docs": true, ".next": true,
	"dist": true, "build": true, "coverage": true, ".turbo": true,
}

var defaultExts = map[string]bool{
	".md": true, ".mdx": true, ".txt": true, ".json": true, ".yml": true, ".yaml": true,
	".toml": true, ".css": true, ".js": true, ".jsx": true, ".ts": true, ".tsx": true,
}

var defaultNames = map[string]bool{
	"mint.json": true, "docs.json": true, "openapi.json": true, "openapi.yaml": true,
	"README": true, "README.md": true, "llms.txt": true, "llms-full.txt": true,
}

func Create(repoRoot, outPath string) (Result, error) {
	absRoot, err := filepath.Abs(repoRoot)
	if err != nil {
		return Result{}, err
	}
	var files []FileRecord
	if err := filepath.WalkDir(absRoot, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		name := d.Name()
		if d.IsDir() {
			if path != absRoot && defaultSkipDirs[name] {
				return filepath.SkipDir
			}
			return nil
		}
		if !looksLikeDocsFile(name) {
			return nil
		}
		rel, err := filepath.Rel(absRoot, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if strings.HasPrefix(rel, "../") || strings.HasPrefix(rel, "/") {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		if info.Size() > 2*1024*1024 {
			return nil
		}
		h, err := fileSHA256(path)
		if err != nil {
			return err
		}
		files = append(files, FileRecord{Path: rel, SizeBytes: info.Size(), SHA256: h, ContentType: contentType(name)})
		return nil
	}); err != nil {
		return Result{}, err
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	manifest := Manifest{SchemaVersion: 1, CreatedAt: time.Now().UTC().Format(time.RFC3339), RepoRoot: filepath.Base(absRoot), Files: files}
	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		return Result{}, err
	}
	out, err := os.Create(outPath)
	if err != nil {
		return Result{}, err
	}
	defer out.Close()
	hash := sha256.New()
	mw := io.MultiWriter(out, hash)
	gz := gzip.NewWriter(mw)
	tw := tar.NewWriter(gz)
	if err := writeManifest(tw, manifest); err != nil {
		return Result{}, err
	}
	for _, rec := range files {
		if err := writeFile(tw, absRoot, rec.Path); err != nil {
			return Result{}, err
		}
	}
	if err := tw.Close(); err != nil {
		return Result{}, err
	}
	if err := gz.Close(); err != nil {
		return Result{}, err
	}
	if err := out.Close(); err != nil {
		return Result{}, err
	}
	info, err := os.Stat(outPath)
	if err != nil {
		return Result{}, err
	}
	return Result{Path: outPath, Manifest: manifest, SHA256: hex.EncodeToString(hash.Sum(nil)), Bytes: info.Size()}, nil
}

func looksLikeDocsFile(name string) bool {
	if defaultNames[name] {
		return true
	}
	return defaultExts[strings.ToLower(filepath.Ext(name))]
}

func contentType(name string) string {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".md", ".mdx":
		return "text/markdown"
	case ".json":
		return "application/json"
	case ".yml", ".yaml", ".toml":
		return "text/plain"
	default:
		return "text/plain"
	}
}

func fileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func writeManifest(tw *tar.Writer, manifest Manifest) error {
	b, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')
	hdr := &tar.Header{Name: "manifest.json", Mode: 0o644, Size: int64(len(b)), ModTime: time.Now()}
	if err := tw.WriteHeader(hdr); err != nil {
		return err
	}
	_, err = tw.Write(b)
	return err
}

func writeFile(tw *tar.Writer, root, rel string) error {
	path := filepath.Join(root, filepath.FromSlash(rel))
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	hdr, err := tar.FileInfoHeader(info, "")
	if err != nil {
		return err
	}
	hdr.Name = "files/" + rel
	hdr.Mode = 0o644
	if err := tw.WriteHeader(hdr); err != nil {
		return err
	}
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := io.Copy(tw, f); err != nil {
		return fmt.Errorf("copy %s: %w", rel, err)
	}
	return nil
}
