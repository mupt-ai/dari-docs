package flueclient

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWriteUpdatedFilesNormalizesWorkspacePath(t *testing.T) {
	dir := t.TempDir()
	err := writeUpdatedFiles(dir, editorResult{Files: []docFile{{Path: "input-docs/files/README.md", Content: "updated\n"}}})
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(dir, "README.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "updated\n" {
		t.Fatalf("README = %q", got)
	}
	if _, err := os.Stat(filepath.Join(dir, "input-docs")); !os.IsNotExist(err) {
		t.Fatalf("workspace prefix should not be written, stat err=%v", err)
	}
}
