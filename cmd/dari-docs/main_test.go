package main

import (
	"archive/zip"
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mupt-ai/dari-docs/internal/managed"
	"github.com/mupt-ai/dari-docs/internal/runner"
)

func TestParseDollarsToCents(t *testing.T) {
	tests := map[string]int64{
		"5":     500,
		"20.00": 2000,
		"0.99":  99,
		"12.3":  1230,
	}
	for in, want := range tests {
		got, err := parseDollarsToCents(in)
		if err != nil {
			t.Fatal(err)
		}
		if got != want {
			t.Fatalf("parseDollarsToCents(%q) = %d, want %d", in, got, want)
		}
	}
	if _, err := parseDollarsToCents("1.234"); err == nil {
		t.Fatal("expected too many decimal places error")
	}
}

func TestParseExpiresIn(t *testing.T) {
	got, err := parseExpiresIn("2d")
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || time.Until(*got) < 47*time.Hour || time.Until(*got) > 49*time.Hour {
		t.Fatalf("expires in 2d parsed to %v", got)
	}
	got, err = parseExpiresIn("24h")
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || time.Until(*got) < 23*time.Hour || time.Until(*got) > 25*time.Hour {
		t.Fatalf("expires in 24h parsed to %v", got)
	}
	if got, err := parseExpiresIn(""); err != nil || got != nil {
		t.Fatalf("empty expires = %v, %v; want nil nil", got, err)
	}
	if _, err := parseExpiresIn("0d"); err == nil {
		t.Fatal("expected error for zero duration")
	}
}

func TestManagedRunReserveCents(t *testing.T) {
	cfg := managed.RunConfig{TesterSessionReserveCents: 75, EditorSessionReserveCents: 150}
	if got := managedRunReserveCents("check", 3, 3, cfg); got != 675 {
		t.Fatalf("check reserve = %d, want 675", got)
	}
	if got := managedRunReserveCents("optimize", 3, 3, cfg); got != 825 {
		t.Fatalf("optimize reserve = %d, want 825", got)
	}
}

func TestManagedSessionSummary(t *testing.T) {
	tests := map[string]struct {
		command string
		tasks   int
		llms    int
		want    string
	}{
		"single check":    {command: "check", tasks: 1, llms: 1, want: "1 tester session"},
		"multi check":     {command: "check", tasks: 3, llms: 1, want: "3 tester sessions"},
		"matrix check":    {command: "check", tasks: 3, llms: 3, want: "9 tester sessions (3 tasks x 3 LLMs)"},
		"single optimize": {command: "optimize", tasks: 1, llms: 1, want: "1 tester session + 1 editor session"},
		"multi optimize":  {command: "optimize", tasks: 3, llms: 3, want: "9 tester sessions (3 tasks x 3 LLMs) + 1 editor session"},
	}
	for name, tt := range tests {
		if got := managedSessionSummary(tt.command, tt.tasks, tt.llms); got != tt.want {
			t.Fatalf("%s summary = %q, want %q", name, got, tt.want)
		}
	}
}

func TestManagedLLMSelectionDefaultsToAllowedClaudeMatrix(t *testing.T) {
	cfg := managed.RunConfig{
		DefaultLLMID:          "claude-sonnet-4-6",
		DefaultFeedbackLLMIDs: []string{"claude-haiku-4-5", "claude-sonnet-4-6", "claude-opus-4-7"},
		AllowedLLMIDs:         []string{"claude-haiku-4-5", "claude-sonnet-4-6", "claude-opus-4-7", "gpt-5-mini", "gpt-5.1", "gpt-5.5"},
	}
	feedback, editor, err := managedLLMSelection(nil, "", cfg)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(feedback, ",") != "claude-haiku-4-5,claude-sonnet-4-6,claude-opus-4-7" {
		t.Fatalf("feedback = %#v", feedback)
	}
	if editor != "claude-sonnet-4-6" {
		t.Fatalf("editor = %q", editor)
	}
}

func TestManagedLLMSelectionAllowsGPT(t *testing.T) {
	cfg := managed.RunConfig{
		DefaultLLMID:          "claude-sonnet-4-6",
		DefaultFeedbackLLMIDs: []string{"claude-haiku-4-5", "claude-sonnet-4-6", "claude-opus-4-7"},
		AllowedLLMIDs:         []string{"claude-haiku-4-5", "claude-sonnet-4-6", "claude-opus-4-7", "gpt-5-mini", "gpt-5.1", "gpt-5.5"},
	}
	feedback, editor, err := managedLLMSelection([]string{"gpt-5.5"}, "gpt-5.1", cfg)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(feedback, ",") != "gpt-5.5" || editor != "gpt-5.1" {
		t.Fatalf("feedback/editor = %#v/%q", feedback, editor)
	}
}

func TestDefaultFeedbackLLMIDsIncludesBundledMatrix(t *testing.T) {
	got := runner.DefaultFeedbackLLMIDs()
	want := []string{"claude-haiku-4-5", "claude-sonnet-4-6", "claude-opus-4-7", "gpt-5-mini", "gpt-5.1", "gpt-5.5"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("defaultFeedbackLLMIDs = %#v, want %#v", got, want)
	}
}

func TestUniqueTrimmedListDeduplicates(t *testing.T) {
	got := uniqueTrimmedList([]string{"claude-haiku-4-5", " claude-sonnet-4-6", "gpt-5.5", "claude-sonnet-4-6"})
	want := []string{"claude-haiku-4-5", "claude-sonnet-4-6", "gpt-5.5"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("uniqueTrimmedList = %#v, want %#v", got, want)
	}
}

func TestExpandFeedbackLLMListSupportsGroups(t *testing.T) {
	got := expandFeedbackLLMList([]string{"claude", "gpt-5.1", "gpt", "claude-opus-4-7"})
	want := []string{"claude-haiku-4-5", "claude-sonnet-4-6", "claude-opus-4-7", "gpt-5.1", "gpt-5-mini", "gpt-5.5"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("expandFeedbackLLMList = %#v, want %#v", got, want)
	}
}

func TestManagedRunFeedbackOutputPrintsCanonicalAggregate(t *testing.T) {
	status := managed.RunStatus{
		ID:                "run_123",
		Mode:              "check",
		Status:            "completed",
		AggregateFeedback: "# Dari Docs Feedback\n\n## Task 1\n\nfeedback",
	}
	got, err := managedRunFeedbackOutput(status)
	if err != nil {
		t.Fatal(err)
	}
	want := "# Dari Docs Feedback\n\n## Task 1\n\nfeedback\n"
	if got != want {
		t.Fatalf("feedback markdown = %q, want %q", got, want)
	}
}

func TestDownloadManagedRunArtifactsForCheckWritesFeedback(t *testing.T) {
	outDir := t.TempDir()
	client := managed.New("http://127.0.0.1:1", "token")
	status := managed.RunStatus{
		ID:                "run_check",
		Mode:              "check",
		Status:            "completed",
		FeedbackReports:   []string{"feedback one"},
		AggregateFeedback: "# aggregate\n",
	}
	updatedDir, err := downloadManagedRunArtifacts(context.Background(), client, status, outDir)
	if err != nil {
		t.Fatal(err)
	}
	if updatedDir != "" {
		t.Fatalf("updatedDir = %q, want empty for check", updatedDir)
	}
	aggregate, err := os.ReadFile(filepath.Join(outDir, "aggregate-feedback.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(aggregate) != "# aggregate\n" {
		t.Fatalf("aggregate = %q", aggregate)
	}
	report, err := os.ReadFile(filepath.Join(outDir, "runs", "feedback-001.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(report) != "feedback one\n" {
		t.Fatalf("report = %q", report)
	}
}

func TestManagedRunFeedbackMarkdownGroupsInterleavedSessionsByTask(t *testing.T) {
	status := managed.RunStatus{
		Tasks:           []string{"First task", "Second task"},
		FeedbackReports: []string{"task 1 from a", "task 2 from b", "task 1 from c"},
		Sessions: []managed.RunSessionSummary{
			{Kind: "tester", Status: "completed", TaskIndex: 1, LLMID: "llm-a"},
			{Kind: "tester", Status: "completed", TaskIndex: 2, LLMID: "llm-b"},
			{Kind: "tester", Status: "completed", TaskIndex: 1, LLMID: "llm-c"},
		},
	}
	got := managedRunFeedbackMarkdown(status)
	if count := strings.Count(got, "## Task 1"); count != 1 {
		t.Fatalf("Task 1 heading count = %d, want 1:\n%s", count, got)
	}
	if count := strings.Count(got, "## Task 2"); count != 1 {
		t.Fatalf("Task 2 heading count = %d, want 1:\n%s", count, got)
	}
	if strings.Index(got, "task 1 from a") > strings.Index(got, "## Task 2") || strings.Index(got, "task 1 from c") > strings.Index(got, "## Task 2") {
		t.Fatalf("Task 1 reports were not grouped before Task 2:\n%s", got)
	}
}

func TestDownloadManagedRunArtifactsRejectsActiveRun(t *testing.T) {
	client := managed.New("http://127.0.0.1:1", "token")
	status := managed.RunStatus{ID: "run_running", Mode: "check", Status: "running"}
	if _, err := downloadManagedRunArtifacts(context.Background(), client, status, t.TempDir()); err == nil {
		t.Fatal("expected running run download to fail")
	}
}

func TestDownloadManagedRunArtifactsForFailedRunWritesFeedback(t *testing.T) {
	outDir := t.TempDir()
	client := managed.New("http://127.0.0.1:1", "token")
	status := managed.RunStatus{
		ID:              "run_failed",
		Mode:            "check",
		Status:          "failed",
		FeedbackReports: []string{"partial feedback"},
	}
	updatedDir, err := downloadManagedRunArtifacts(context.Background(), client, status, outDir)
	if err != nil {
		t.Fatal(err)
	}
	if updatedDir != "" {
		t.Fatalf("updatedDir = %q, want empty for failed run", updatedDir)
	}
	report, err := os.ReadFile(filepath.Join(outDir, "runs", "feedback-001.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(report) != "partial feedback\n" {
		t.Fatalf("report = %q", report)
	}
}

func TestDownloadManagedRunArtifactsDownloadsOptimizeOutput(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/runs/run_opt/updated-docs.zip" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/zip")
		if err := writeUpdatedDocsZip(w, map[string]string{"updated-docs/files/README.md": "updated docs\n"}); err != nil {
			t.Fatal(err)
		}
	}))
	defer server.Close()

	outDir := t.TempDir()
	client := managed.New(server.URL, "token")
	status := managed.RunStatus{
		ID:                   "run_opt",
		Mode:                 "optimize",
		Status:               "completed",
		UpdatedDocsAvailable: true,
		FeedbackReports:      []string{"feedback"},
	}
	updatedDir, err := downloadManagedRunArtifacts(context.Background(), client, status, outDir)
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(updatedDir, "README.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "updated docs\n" {
		t.Fatalf("downloaded README = %q", got)
	}
}

func writeUpdatedDocsZip(w http.ResponseWriter, files map[string]string) error {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, contents := range files {
		f, err := zw.Create(name)
		if err != nil {
			return err
		}
		if _, err := f.Write([]byte(contents)); err != nil {
			return err
		}
	}
	if err := zw.Close(); err != nil {
		return err
	}
	_, err := w.Write(buf.Bytes())
	return err
}

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

func TestManagedCheckRequiresLoginBeforeRunConfig(t *testing.T) {
	repo := t.TempDir()
	t.Setenv("HOME", filepath.Join(t.TempDir(), "home"))

	cmd := newCheckOptimizeCommand("check")
	cmd.SetArgs([]string{repo, "--managed", "--task", "Run echo ok"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected missing login error")
	}
	if !strings.Contains(err.Error(), "not logged in to managed service") {
		t.Fatalf("error = %q, want login error", err.Error())
	}
	if strings.Contains(err.Error(), "missing managed agent set") {
		t.Fatalf("error = %q, should not mention missing agent set before login", err.Error())
	}
}

func TestManagedApplyRequiresWait(t *testing.T) {
	err := runManagedCheckOrOptimizeFromOptions(context.Background(), checkOptimizeOptions{
		Command: "optimize",
		Managed: true,
		Apply:   true,
	})
	if err == nil {
		t.Fatal("expected --apply without --wait error")
	}
	if !strings.Contains(err.Error(), "--apply requires --wait") {
		t.Fatalf("error = %q, want --apply requires --wait", err.Error())
	}
}

func TestManagedAgentDeployManagedNoops(t *testing.T) {
	repo := t.TempDir()
	t.Setenv("HOME", filepath.Join(t.TempDir(), "home"))

	cmd := newAgentsCommand()
	cmd.SetArgs([]string{"deploy", "--managed", repo})
	err := cmd.Execute()
	if err != nil {
		t.Fatalf("managed agent deploy should be a no-op: %v", err)
	}
}

func TestAuthLogoutWithoutTokenSucceeds(t *testing.T) {
	home := filepath.Join(t.TempDir(), "home")
	t.Setenv("HOME", home)

	cmd := newAuthLogoutCommand()
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(home, ".dari-docs", "credentials.json")); !os.IsNotExist(err) {
		t.Fatalf("credentials file should not be created on no-op logout, stat err=%v", err)
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

func TestSetLLMAPIKeySecretRejectsMultipleProviders(t *testing.T) {
	path := filepath.Join(t.TempDir(), "dari.yml")
	original := `name: test
llm:
  default: claude-sonnet-4-6
  options:
    claude-sonnet-4-6:
      provider: anthropic
      model: claude-sonnet-4-6
    gpt-5.5:
      provider: openai
      model: gpt-5.5
`
	if err := os.WriteFile(path, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}
	err := setLLMAPIKeySecret(path, "MY_KEY")
	if err == nil || !strings.Contains(err.Error(), "multiple LLM providers") {
		t.Fatalf("err = %v, want multiple-provider rejection", err)
	}
}

func TestSetLLMAPIKeySecretsByProviderUpdatesMatchingOptions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "dari.yml")
	original := `name: test
llm:
  default: claude-sonnet-4-6
  options:
    claude-sonnet-4-6:
      provider: anthropic
      model: claude-sonnet-4-6
    gpt-5.5:
      provider: openai
      model: gpt-5.5
`
	if err := os.WriteFile(path, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := setLLMAPIKeySecretsByProvider(path, map[string]string{"anthropic": "ANTHROPIC_KEY", "openai": "OPENAI_KEY"}); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	got := string(b)
	if strings.Count(got, "api_key_secret: ANTHROPIC_KEY") != 1 || strings.Count(got, "api_key_secret: OPENAI_KEY") != 1 {
		t.Fatalf("provider-specific api_key_secret values were not inserted:\n%s", got)
	}
}

func TestSetLLMAPIKeySecretReplacesExistingSecret(t *testing.T) {
	path := filepath.Join(t.TempDir(), "dari.yml")
	original := `name: test
llm:
  default: claude-sonnet-4-6
  options:
    claude-sonnet-4-6:
      provider: anthropic
      model: claude-sonnet-4-6
      api_key_secret: OLD_KEY
`
	if err := os.WriteFile(path, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := setLLMAPIKeySecret(path, "MY_KEY"); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	got := string(b)
	if strings.Contains(got, "OLD_KEY") {
		t.Fatalf("old api_key_secret was preserved:\n%s", got)
	}
	if strings.Count(got, "api_key_secret: MY_KEY") != 1 {
		t.Fatalf("api_key_secret was not replaced:\n%s", got)
	}
}

func TestSetLLMAPIKeySecretPreservesModel(t *testing.T) {
	path := filepath.Join(t.TempDir(), "dari.yml")
	original := "name: test\nllm:\n  model: anthropic/claude-sonnet-4.6\n"
	if err := os.WriteFile(path, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := setLLMAPIKeySecret(path, "MY_KEY"); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	got := string(b)
	if !strings.Contains(got, "model: anthropic/claude-sonnet-4.6") {
		t.Fatalf("model was not preserved:\n%s", got)
	}
	if !strings.Contains(got, "api_key_secret: MY_KEY") {
		t.Fatalf("api_key_secret was not inserted:\n%s", got)
	}
}
