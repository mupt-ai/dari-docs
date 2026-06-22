package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPrepareUsesPublicDocsURLWithoutBundlingCWD(t *testing.T) {
	cwd := t.TempDir()
	t.Chdir(cwd)
	if err := os.WriteFile(filepath.Join(cwd, "README.md"), []byte("# Local\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	opts := &checkOptimizeOptions{
		Command:       "check",
		TaskInputs:    []string{"Read docs"},
		PublicDocURLs: []string{"https://docs.dari.dev/llms.txt"},
	}
	if err := opts.prepare(); err != nil {
		t.Fatal(err)
	}
	if !opts.PublicDocsOnly {
		t.Fatal("expected public-docs-only source")
	}
	if len(opts.BundleOptions.ExtraFiles) != 0 {
		t.Fatalf("extra files = %#v, want no synthetic public docs file", opts.BundleOptions.ExtraFiles)
	}
	if len(opts.PublicDocURLs) != 1 || opts.PublicDocURLs[0] != "https://docs.dari.dev/llms.txt" {
		t.Fatalf("public doc URLs = %#v", opts.PublicDocURLs)
	}
	if opts.RepoRoot != cwd {
		t.Fatalf("RepoRoot = %q, want cwd for config lookup", opts.RepoRoot)
	}
}

func TestPrepareRejectsOptimizeWithPublicDocsURL(t *testing.T) {
	repo := t.TempDir()
	opts := &checkOptimizeOptions{
		Command:       "optimize",
		RepoArg:       repo,
		TaskInputs:    []string{"Improve docs"},
		PublicDocURLs: []string{"https://example.com/llms.txt"},
	}
	err := opts.prepare()
	if err == nil || !strings.Contains(err.Error(), "support check only") {
		t.Fatalf("err = %v, want public docs optimize rejection", err)
	}
}

func TestManagedCheckFlagIsUnsupported(t *testing.T) {
	repo := t.TempDir()

	cmd := newCheckOptimizeCommand("check")
	cmd.SetArgs([]string{repo, "--managed", "--task", "Run echo ok"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected unsupported managed mode error")
	}
	if !strings.Contains(err.Error(), "hosted/managed Dari Docs path is not supported") {
		t.Fatalf("error = %q, want unsupported managed mode error", err.Error())
	}
}

func TestWaitFlagIsUnsupported(t *testing.T) {
	repo := t.TempDir()

	cmd := newCheckOptimizeCommand("check")
	cmd.SetArgs([]string{repo, "--wait", "--task", "Run echo ok"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected unsupported --wait error")
	}
	if !strings.Contains(err.Error(), "--wait is not supported for Flue runs") {
		t.Fatalf("error = %q, want --wait unsupported", err.Error())
	}
}

func TestAgentsCommandIsUnsupported(t *testing.T) {
	cmd := newAgentsCommand()
	cmd.SetArgs([]string{"deploy", "--managed", "repo"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected unsupported agents command error")
	}
	if !strings.Contains(err.Error(), "hosted/managed Dari Docs path is not supported") {
		t.Fatalf("error = %q, want unsupported managed mode error", err.Error())
	}
}

func TestCheckHelpShowsModelFlagsAndHidesManagedFlags(t *testing.T) {
	cmd := newCheckOptimizeCommand("check")
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"--help"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	help := out.String()
	for _, hidden := range []string{"--managed", "--wait", "--editor-llm", "--apply", "--editor-url", "--feedback-agent", "--editor-agent", "--api-key", "remote-editor"} {
		if strings.Contains(help, hidden) {
			t.Fatalf("check help should not contain %q:\n%s", hidden, help)
		}
	}
	for _, shown := range []string{"--tester-url", "--task", "--docs-url", "--llm", "--feedback-llm", "--parallel"} {
		if !strings.Contains(help, shown) {
			t.Fatalf("check help missing %q:\n%s", shown, help)
		}
	}
}

func TestInitHelpShowsFlueURLFlags(t *testing.T) {
	cmd := newInitCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"--help"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	help := out.String()
	for _, hidden := range []string{"--deploy", "--api-key", "--anthropic-api-key-secret", "dari deploy"} {
		if strings.Contains(help, hidden) {
			t.Fatalf("init help should not contain %q:\n%s", hidden, help)
		}
	}
	for _, shown := range []string{"--tester-url", "--editor-url", "--agents-dir"} {
		if !strings.Contains(help, shown) {
			t.Fatalf("init help missing %q:\n%s", shown, help)
		}
	}
}

func TestRootHelpShowsOnlyFlueCommands(t *testing.T) {
	cmd := newRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"--help"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	help := out.String()
	for _, hidden := range []string{"auth", "billing", "runs", "--managed", "--wait", "--llm", "--feedback-llm", "--editor-llm"} {
		if strings.Contains(help, hidden) {
			t.Fatalf("root help should not contain %q:\n%s", hidden, help)
		}
	}
	for _, shown := range []string{"check", "init", "optimize"} {
		if !strings.Contains(help, shown) {
			t.Fatalf("root help missing %q:\n%s", shown, help)
		}
	}
}

func TestVersionLine(t *testing.T) {
	original := version
	t.Cleanup(func() { version = original })
	version = "v0.1.0"
	if got, want := versionLine(), "dari-docs v0.1.0"; got != want {
		t.Fatalf("versionLine() = %q, want %q", got, want)
	}
}

func TestReadTasksFileParsesParagraphsAndBullets(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tasks.txt")
	input := strings.Join([]string{
		"- Install the SDK",
		"  and make a first API call",
		"",
		"* Set up authentication",
		"",
		"Review webhook docs",
		"and create a checkout session",
		"",
	}, "\n")
	if err := os.WriteFile(path, []byte(input), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := readTasksFile(path)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"Install the SDK\nand make a first API call",
		"Set up authentication",
		"Review webhook docs\nand create a checkout session",
	}
	if strings.Join(got, "\n---\n") != strings.Join(want, "\n---\n") {
		t.Fatalf("tasks = %#v, want %#v", got, want)
	}
}
