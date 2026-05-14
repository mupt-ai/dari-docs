package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mupt-ai/dari-docs/internal/managed"
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
	if got := managedRunReserveCents("check", 3, cfg); got != 225 {
		t.Fatalf("check reserve = %d, want 225", got)
	}
	if got := managedRunReserveCents("optimize", 3, cfg); got != 375 {
		t.Fatalf("optimize reserve = %d, want 375", got)
	}
}

func TestManagedSessionSummary(t *testing.T) {
	tests := map[string]struct {
		command string
		tasks   int
		want    string
	}{
		"single check":    {command: "check", tasks: 1, want: "1 tester session"},
		"multi check":     {command: "check", tasks: 3, want: "3 tester sessions"},
		"single optimize": {command: "optimize", tasks: 1, want: "1 tester session + 1 editor session"},
		"multi optimize":  {command: "optimize", tasks: 3, want: "3 tester sessions + 1 editor session"},
	}
	for name, tt := range tests {
		if got := managedSessionSummary(tt.command, tt.tasks); got != tt.want {
			t.Fatalf("%s summary = %q, want %q", name, got, tt.want)
		}
	}
}

func TestDefaultFeedbackLLMIDsIncludesBundledMatrix(t *testing.T) {
	got := defaultFeedbackLLMIDs()
	want := []string{"dumb-claude", "medium-claude", "smart-claude", "dumb-gpt", "medium-gpt", "smart-gpt"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("defaultFeedbackLLMIDs = %#v, want %#v", got, want)
	}
}

func TestExpandCSVListTrimsDeduplicatesAndSplits(t *testing.T) {
	got := expandCSVList([]string{"dumb-claude, medium-claude", "smart-gpt", "medium-claude"})
	want := []string{"dumb-claude", "medium-claude", "smart-gpt"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("expandCSVList = %#v, want %#v", got, want)
	}
}

func TestManagedCheckRequiresLoginBeforeAgentSet(t *testing.T) {
	repo := t.TempDir()
	t.Setenv("HOME", filepath.Join(t.TempDir(), "home"))

	err := runCheckOrOptimize("check", []string{repo, "--managed", "--task", "Run echo ok"})
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

func TestManagedAgentDeployRequiresLoginBeforeInit(t *testing.T) {
	repo := t.TempDir()
	t.Setenv("HOME", filepath.Join(t.TempDir(), "home"))

	err := runAgents([]string{"deploy", "--managed", repo})
	if err == nil {
		t.Fatal("expected missing login error")
	}
	if !strings.Contains(err.Error(), "not logged in to managed service") {
		t.Fatalf("error = %q, want login error", err.Error())
	}
	if strings.Contains(err.Error(), "missing local agents") {
		t.Fatalf("error = %q, should not mention missing local agents before login", err.Error())
	}
}

func TestAuthLogoutWithoutTokenSucceeds(t *testing.T) {
	home := filepath.Join(t.TempDir(), "home")
	t.Setenv("HOME", home)

	if err := runAuthLogout(nil); err != nil {
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
  default: medium-claude
  options:
    medium-claude:
      provider: anthropic
      model: claude-sonnet-4-6
    smart-gpt:
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
  default: medium-claude
  options:
    medium-claude:
      provider: anthropic
      model: claude-sonnet-4-6
    smart-gpt:
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
  default: medium-claude
  options:
    medium-claude:
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
