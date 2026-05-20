package publicdocs

import (
	"strings"
	"testing"

	"github.com/mupt-ai/dari-docs/internal/bundle"
)

func TestSourceFilesWritesPublicDocsSource(t *testing.T) {
	files, summary, err := SourceFiles([]string{"https://docs.dari.dev/llms.txt", "https://docs.dari.dev/llms.txt#ignored"})
	if err != nil {
		t.Fatal(err)
	}
	if len(summary.SeedURLs) != 1 {
		t.Fatalf("seed URLs = %#v, want deduplicated URL", summary.SeedURLs)
	}
	if len(files) != 1 {
		t.Fatalf("files = %#v, want one source file", files)
	}
	if files[0].Path != "public-docs/source.md" {
		t.Fatalf("path = %q", files[0].Path)
	}
	if err := bundle.ValidateRelativePath(files[0].Path); err != nil {
		t.Fatalf("invalid bundle path: %v", err)
	}
	content := string(files[0].Content)
	for _, want := range []string{"https://docs.dari.dev/llms.txt", "Internet access is required", "llms.txt manifest"} {
		if !strings.Contains(content, want) {
			t.Fatalf("content missing %q:\n%s", want, content)
		}
	}
}

func TestSourceFilesRejectsInvalidURL(t *testing.T) {
	_, _, err := SourceFiles([]string{"file:///etc/passwd"})
	if err == nil || !strings.Contains(err.Error(), "invalid public docs URL") {
		t.Fatalf("err = %v, want URL rejection", err)
	}
}
