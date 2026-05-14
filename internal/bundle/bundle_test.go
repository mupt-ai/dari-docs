package bundle

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestReadReturnsManifestAndArchiveHash(t *testing.T) {
	repo := t.TempDir()
	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("# Test\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(t.TempDir(), "bundle.tar.gz")
	created, err := Create(repo, out)
	if err != nil {
		t.Fatal(err)
	}
	read, err := Read(out)
	if err != nil {
		t.Fatal(err)
	}
	if read.SHA256 != created.SHA256 {
		t.Fatalf("sha mismatch: got %s want %s", read.SHA256, created.SHA256)
	}
	if len(read.Manifest.Files) != 1 {
		t.Fatalf("file count = %d, want 1", len(read.Manifest.Files))
	}
	if read.Manifest.Files[0].Path != "README.md" {
		t.Fatalf("path = %q, want README.md", read.Manifest.Files[0].Path)
	}
}

func TestCreateWithOptionsReportsSkipsAndAppliesGlobs(t *testing.T) {
	repo := t.TempDir()
	writeFileForTest(t, repo, "docs/guide.md", "# Guide\n")
	writeFileForTest(t, repo, "docs/private.md", "# Private\n")
	writeFileForTest(t, repo, "docs/large.md", strings.Repeat("x", 64))
	writeFileForTest(t, repo, "schemas/example.proto", "syntax = \"proto3\";\n")
	writeFileForTest(t, repo, "src/main.go", "package main\n")
	writeFileForTest(t, repo, "node_modules/pkg/README.md", "# Dependency\n")

	out := filepath.Join(t.TempDir(), "bundle.tar.gz")
	created, err := CreateWithOptions(repo, out, CreateOptions{
		Include:      []string{"schemas/*.proto"},
		Exclude:      []string{"docs/private.md"},
		MaxFileBytes: 32,
	})
	if err != nil {
		t.Fatal(err)
	}

	var paths []string
	for _, rec := range created.Manifest.Files {
		paths = append(paths, rec.Path)
	}
	wantPaths := []string{"docs/guide.md", "schemas/example.proto"}
	if strings.Join(paths, ",") != strings.Join(wantPaths, ",") {
		t.Fatalf("paths = %v, want %v", paths, wantPaths)
	}
	if created.Skipped.IgnoredDirs != 1 {
		t.Fatalf("ignored dirs = %d, want 1", created.Skipped.IgnoredDirs)
	}
	if created.Skipped.UnsupportedFiles != 1 {
		t.Fatalf("unsupported files = %d, want 1", created.Skipped.UnsupportedFiles)
	}
	if created.Skipped.ExcludedFiles != 1 {
		t.Fatalf("excluded files = %d, want 1", created.Skipped.ExcludedFiles)
	}
	if len(created.Skipped.OversizedFiles) != 1 || created.Skipped.OversizedFiles[0].Path != "docs/large.md" {
		t.Fatalf("oversized files = %#v, want docs/large.md", created.Skipped.OversizedFiles)
	}
}

func TestCreateSkipsSymlinks(t *testing.T) {
	repo := t.TempDir()
	outside := filepath.Join(t.TempDir(), "secret.md")
	if err := os.WriteFile(outside, []byte("do not bundle\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(repo, "linked.md")); err != nil {
		t.Skipf("symlink not supported on this platform: %v", err)
	}

	out := filepath.Join(t.TempDir(), "bundle.tar.gz")
	created, err := Create(repo, out)
	if err != nil {
		t.Fatal(err)
	}
	if len(created.Manifest.Files) != 0 {
		t.Fatalf("files = %#v, want symlink skipped", created.Manifest.Files)
	}
	if created.Skipped.UnsupportedFiles != 1 {
		t.Fatalf("unsupported files = %d, want 1", created.Skipped.UnsupportedFiles)
	}
}

func TestWriteSummaryIncludesSkippedCategories(t *testing.T) {
	res := Result{
		Path:         "bundle.tar.gz",
		Bytes:        1536,
		MaxFileBytes: 32,
		Manifest: Manifest{Files: []FileRecord{
			{Path: "README.md"},
		}},
		Skipped: SkipSummary{
			IgnoredDirs:      1,
			UnsupportedFiles: 2,
			ExcludedFiles:    3,
			OversizedFiles:   []SkippedFile{{Path: "docs/large.md", SizeBytes: 64}},
		},
	}
	var out strings.Builder
	WriteSummary(&out, res)
	got := out.String()
	for _, want := range []string{
		"Included: 1 file, 1.5 KiB compressed",
		"Skipped: 1 ignored directory, 2 unsupported files, 3 excluded paths, 1 file over the 32 B per-file limit",
		"Large skipped files were not uploaded:",
		"docs/large.md (64 B)",
		"include patterns do not bypass the per-file limit",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("summary = %q, want substring %q", got, want)
		}
	}
}

func TestReadRejectsTraversalEntry(t *testing.T) {
	bundlePath := filepath.Join(t.TempDir(), "bundle.tar.gz")
	manifest := Manifest{SchemaVersion: 1, CreatedAt: time.Now().UTC().Format(time.RFC3339), RepoRoot: "repo"}
	if err := writeTestBundle(bundlePath, manifest, map[string]string{"files/../evil.md": "bad"}); err != nil {
		t.Fatal(err)
	}
	_, err := Read(bundlePath)
	if err == nil || !strings.Contains(err.Error(), "invalid tar entry path") {
		t.Fatalf("err = %v, want invalid tar entry path", err)
	}
}

func TestReadRejectsFileNotListedInManifest(t *testing.T) {
	bundlePath := filepath.Join(t.TempDir(), "bundle.tar.gz")
	manifest := Manifest{SchemaVersion: 1, CreatedAt: time.Now().UTC().Format(time.RFC3339), RepoRoot: "repo"}
	if err := writeTestBundle(bundlePath, manifest, map[string]string{"files/README.md": "# Test\n"}); err != nil {
		t.Fatal(err)
	}
	_, err := Read(bundlePath)
	if err == nil || !strings.Contains(err.Error(), "not listed in manifest") {
		t.Fatalf("err = %v, want not listed in manifest", err)
	}
}

func TestReadRejectsManifestHashMismatch(t *testing.T) {
	bundlePath := filepath.Join(t.TempDir(), "bundle.tar.gz")
	manifest := Manifest{
		SchemaVersion: 1,
		CreatedAt:     time.Now().UTC().Format(time.RFC3339),
		RepoRoot:      "repo",
		Files: []FileRecord{{
			Path:      "README.md",
			SizeBytes: int64(len("# Test\n")),
			SHA256:    strings.Repeat("0", sha256.Size*2),
		}},
	}
	if err := writeTestBundle(bundlePath, manifest, map[string]string{"files/README.md": "# Test\n"}); err != nil {
		t.Fatal(err)
	}
	_, err := Read(bundlePath)
	if err == nil || !strings.Contains(err.Error(), "sha256 does not match manifest") {
		t.Fatalf("err = %v, want sha256 mismatch", err)
	}
}

func TestReadRejectsUncompressedLimit(t *testing.T) {
	bundlePath := filepath.Join(t.TempDir(), "bundle.tar.gz")
	content := strings.Repeat("x", 64)
	sum := sha256.Sum256([]byte(content))
	manifest := Manifest{
		SchemaVersion: 1,
		CreatedAt:     time.Now().UTC().Format(time.RFC3339),
		RepoRoot:      "repo",
		Files: []FileRecord{{
			Path:      "README.md",
			SizeBytes: int64(len(content)),
			SHA256:    hex.EncodeToString(sum[:]),
		}},
	}
	if err := writeTestBundle(bundlePath, manifest, map[string]string{"files/README.md": content}); err != nil {
		t.Fatal(err)
	}
	_, err := ReadWithOptions(bundlePath, ReadOptions{MaxUncompressedBytes: 32, MaxFileBytes: 1024})
	if err == nil || !strings.Contains(err.Error(), "uncompressed size limit") {
		t.Fatalf("err = %v, want uncompressed size limit", err)
	}
}

func TestReadRejectsFileSizeLimit(t *testing.T) {
	bundlePath := filepath.Join(t.TempDir(), "bundle.tar.gz")
	content := strings.Repeat("x", 64)
	sum := sha256.Sum256([]byte(content))
	manifest := Manifest{
		SchemaVersion: 1,
		CreatedAt:     time.Now().UTC().Format(time.RFC3339),
		RepoRoot:      "repo",
		Files: []FileRecord{{
			Path:      "README.md",
			SizeBytes: int64(len(content)),
			SHA256:    hex.EncodeToString(sum[:]),
		}},
	}
	if err := writeTestBundle(bundlePath, manifest, map[string]string{"files/README.md": content}); err != nil {
		t.Fatal(err)
	}
	_, err := ReadWithOptions(bundlePath, ReadOptions{MaxUncompressedBytes: 1024, MaxFileBytes: 32})
	if err == nil || !strings.Contains(err.Error(), "exceeds file size limit") {
		t.Fatalf("err = %v, want file size limit", err)
	}
}

func writeTestBundle(path string, manifest Manifest, files map[string]string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	gz := gzip.NewWriter(f)
	defer gz.Close()
	tw := tar.NewWriter(gz)
	defer tw.Close()
	manifestBytes, err := json.Marshal(manifest)
	if err != nil {
		return err
	}
	if err := tw.WriteHeader(&tar.Header{Name: "manifest.json", Mode: 0o644, Size: int64(len(manifestBytes))}); err != nil {
		return err
	}
	if _, err := tw.Write(manifestBytes); err != nil {
		return err
	}
	for name, content := range files {
		if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o644, Size: int64(len(content))}); err != nil {
			return err
		}
		if _, err := tw.Write([]byte(content)); err != nil {
			return err
		}
	}
	return nil
}

func writeFileForTest(t *testing.T, root, rel, content string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
