package ablation

import (
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mupt-ai/dari-docs/internal/docsdiff"
)

// apply runs a script the way SelfBench does, with /app pointed at dir.
func apply(t *testing.T, script, dir string) {
	t.Helper()
	cmd := exec.Command("bash", "-c", strings.ReplaceAll(script, "/app", dir))
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("script failed: %v\n%s", err, out)
	}
}

func TestScriptLeavesTheDocsAsTheSnapshotHasThem(t *testing.T) {
	dir := t.TempDir()
	for path, content := range map[string]string{
		"docs/old.md":         "remove me",
		"docs/it's quoted.md": "remove me too",
		"docs/guide.md":       "stale",
		"src/main.go":         "package main",
		"docs/untouched.md":   "keep",
	} {
		full := filepath.Join(dir, path)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	snap := docsdiff.Snapshot{Files: []docsdiff.File{
		{Path: "docs/old.md", Absent: true},
		{Path: "docs/it's quoted.md", Absent: true},
		{Path: "docs/guide.md", Content: []byte("fresh")},
		{Path: "docs/new/deep/page.mdx", Content: []byte("# New")},
		{Path: "docs/run.sh", Executable: true, Content: []byte("#!/bin/sh\n")},
	}}
	script, err := Script(snap)
	if err != nil {
		t.Fatal(err)
	}
	apply(t, script, dir)

	for _, gone := range []string{"docs/old.md", "docs/it's quoted.md"} {
		if _, err := os.Stat(filepath.Join(dir, gone)); !os.IsNotExist(err) {
			t.Errorf("%s still exists", gone)
		}
	}
	for path, want := range map[string]string{
		"docs/guide.md":          "fresh",
		"docs/new/deep/page.mdx": "# New",
		"src/main.go":            "package main",
		"docs/untouched.md":      "keep",
	} {
		got, err := os.ReadFile(filepath.Join(dir, path))
		if err != nil || string(got) != want {
			t.Errorf("%s = %q, %v; want %q", path, got, err, want)
		}
	}
	if info, err := os.Stat(filepath.Join(dir, "docs/run.sh")); err != nil || info.Mode()&0o100 == 0 {
		t.Errorf("docs/run.sh lost its executable bit: %v %v", info, err)
	}
}

func TestSameDocsMakeTheSameScriptAndName(t *testing.T) {
	a := docsdiff.Snapshot{Revision: "aaaa", Files: []docsdiff.File{{Path: "b.md", Content: []byte("b")}, {Path: "a.md", Content: []byte("a")}}}
	b := docsdiff.Snapshot{Revision: "bbbb", Files: []docsdiff.File{{Path: "a.md", Content: []byte("a")}, {Path: "b.md", Content: []byte("b")}}}
	sa, _ := Script(a)
	sb, _ := Script(b)
	if sa != sb {
		t.Fatal("the same docs at different revisions made different scripts, so reruns would rebuild")
	}
	if got := Name(12, "after", sa); !strings.HasPrefix(got, PRPrefix(12)) || len(got) > MaxNameLength {
		t.Errorf("Name = %q: cleanup finds environments by %q and SelfBench caps names at %d", got, PRPrefix(12), MaxNameLength)
	}
}

func TestScriptRefusesWhatSelfBenchWouldOrShouldNot(t *testing.T) {
	big := make([]byte, 200*1024)
	_, _ = rand.New(rand.NewSource(1)).Read(big) // random bytes do not compress
	for name, snap := range map[string]docsdiff.Snapshot{
		"too large":     {Files: []docsdiff.File{{Path: "docs/big.md", Content: big}}},
		"escapes /app":  {Files: []docsdiff.File{{Path: "../etc/passwd", Content: []byte("x")}}},
		"absolute":      {Files: []docsdiff.File{{Path: "/etc/passwd", Content: []byte("x")}}},
		"git internals": {Files: []docsdiff.File{{Path: "docs/.git/config", Content: []byte("x")}}},
	} {
		if _, err := Script(snap); err == nil {
			t.Errorf("%s: no error", name)
		}
	}
}
