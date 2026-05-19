package workspace

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

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
