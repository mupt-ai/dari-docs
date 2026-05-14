package workspace

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCopyTreeRejectsDestinationSymlinkParent(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()
	outside := t.TempDir()

	if err := os.MkdirAll(filepath.Join(src, "docs"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "docs", "guide.md"), []byte("updated"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(dst, "docs")); err != nil {
		t.Fatal(err)
	}

	err := CopyTree(src, dst)
	if err == nil || !strings.Contains(err.Error(), "destination symlink") {
		t.Fatalf("expected destination symlink error, got %v", err)
	}
	if _, err := os.Stat(filepath.Join(outside, "guide.md")); !os.IsNotExist(err) {
		t.Fatalf("expected outside file not to be written, got %v", err)
	}
}

func TestCopyTreeRejectsDestinationSymlinkFile(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()
	outside := filepath.Join(t.TempDir(), "outside.md")

	if err := os.WriteFile(filepath.Join(src, "guide.md"), []byte("updated"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(outside, []byte("original"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(dst, "guide.md")); err != nil {
		t.Fatal(err)
	}

	err := CopyTree(src, dst)
	if err == nil || !strings.Contains(err.Error(), "destination symlink") {
		t.Fatalf("expected destination symlink error, got %v", err)
	}
	got, err := os.ReadFile(outside)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "original" {
		t.Fatalf("expected outside file to remain unchanged, got %q", got)
	}
}

func TestUpdatedRootRequiresExpectedFilesDirectory(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "README.md"), []byte("bad root fallback"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := UpdatedRoot(root)
	if err == nil || !strings.Contains(err.Error(), "updated-docs/files") {
		t.Fatalf("expected missing updated-docs/files error, got %v", err)
	}
}

func TestUpdatedRootUsesUpdatedDocsFiles(t *testing.T) {
	root := t.TempDir()
	want := filepath.Join(root, "updated-docs", "files")
	if err := os.MkdirAll(want, 0o755); err != nil {
		t.Fatal(err)
	}

	got, err := UpdatedRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("UpdatedRoot = %q, want %q", got, want)
	}
}

func TestUpdatedRootUsesWorkspaceUpdatedDocsFiles(t *testing.T) {
	root := t.TempDir()
	want := filepath.Join(root, "workspace", "updated-docs", "files")
	if err := os.MkdirAll(want, 0o755); err != nil {
		t.Fatal(err)
	}

	got, err := UpdatedRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("UpdatedRoot = %q, want %q", got, want)
	}
}
