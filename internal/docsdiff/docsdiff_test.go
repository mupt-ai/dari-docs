package docsdiff

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"
)

func git(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func write(t *testing.T, dir, path, content string) {
	t.Helper()
	full := filepath.Join(dir, path)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestDiffIsExactlyTheBranchsDocsChange(t *testing.T) {
	dir := t.TempDir()
	git(t, dir, "init", "-q", "-b", "main")
	write(t, dir, "docs/guide.md", "v1")
	write(t, dir, "docs/gone.md", "bye")
	write(t, dir, "docs/main-only.md", "m1")
	write(t, dir, "src/app.go", "package app")
	git(t, dir, "add", ".")
	git(t, dir, "commit", "-qm", "base")

	git(t, dir, "checkout", "-qb", "feature")
	write(t, dir, "docs/guide.md", "v2")
	write(t, dir, "docs/new.md", "hello")
	write(t, dir, "src/app.go", "package app // changed code, not docs")
	git(t, dir, "rm", "-q", "docs/gone.md")
	if err := os.Symlink("guide.md", filepath.Join(dir, "docs/link.md")); err != nil {
		t.Fatal(err)
	}
	git(t, dir, "add", ".")
	git(t, dir, "commit", "-qm", "feature")

	// main moves on after the branch: its change is not the pull request's.
	git(t, dir, "checkout", "-q", "main")
	write(t, dir, "docs/main-only.md", "m2")
	git(t, dir, "commit", "-qam", "main moves")

	change, err := Git{Dir: dir}.Diff(context.Background(), "main", "feature", []string{"docs"})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := change.Paths(), []string{"docs/gone.md", "docs/guide.md", "docs/new.md"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("paths = %v, want %v", got, want)
	}
	if !reflect.DeepEqual(change.Skipped, []string{"docs/link.md"}) {
		t.Errorf("skipped = %v, want the symlink", change.Skipped)
	}
	want := map[string][2]string{ // before, after; "-" is absent
		"docs/gone.md":  {"bye", "-"},
		"docs/guide.md": {"v1", "v2"},
		"docs/new.md":   {"-", "hello"},
	}
	for i, after := range change.After.Files {
		before := change.Before.Files[i]
		got := [2]string{content(before), content(after)}
		if got != want[after.Path] {
			t.Errorf("%s: before/after = %v, want %v", after.Path, got, want[after.Path])
		}
	}
}

func content(f File) string {
	if f.Absent {
		return "-"
	}
	return string(f.Content)
}
