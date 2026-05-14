package agentbundle

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBuildSkipsSymlinks(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "outside.txt")
	if err := os.WriteFile(filepath.Join(root, "dari.yml"), []byte("name: test\nllm:\n  model: anthropic/claude-sonnet-4.6\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(outside, []byte("secret"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "linked-secret.txt")); err != nil {
		t.Skipf("symlinks are unavailable: %v", err)
	}

	b, err := Build(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range b.Files {
		if file == "linked-secret.txt" {
			t.Fatalf("symlink was included in source bundle file list: %v", b.Files)
		}
	}
}
