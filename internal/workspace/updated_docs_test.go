package workspace

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFindUpdatedDocsFilesDirRequiresExpectedFilesDirectory(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "README.md"), []byte("bad root fallback"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := FindUpdatedDocsFilesDir(root)
	if err == nil || !strings.Contains(err.Error(), "updated-docs/files") {
		t.Fatalf("expected missing updated-docs/files error, got %v", err)
	}
}

func TestFindUpdatedDocsFilesDirUsesUpdatedDocsFiles(t *testing.T) {
	root := t.TempDir()
	want := filepath.Join(root, "updated-docs", "files")
	if err := os.MkdirAll(want, 0o755); err != nil {
		t.Fatal(err)
	}

	got, err := FindUpdatedDocsFilesDir(root)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("FindUpdatedDocsFilesDir = %q, want %q", got, want)
	}
}

func TestFindUpdatedDocsFilesDirUsesWorkspaceUpdatedDocsFiles(t *testing.T) {
	root := t.TempDir()
	want := filepath.Join(root, "workspace", "updated-docs", "files")
	if err := os.MkdirAll(want, 0o755); err != nil {
		t.Fatal(err)
	}

	got, err := FindUpdatedDocsFilesDir(root)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("FindUpdatedDocsFilesDir = %q, want %q", got, want)
	}
}
