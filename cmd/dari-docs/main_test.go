package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

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

func TestSetLLMAPIKeySecretUpdatesLLMOptions(t *testing.T) {
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
	if err := setLLMAPIKeySecret(path, "MY_KEY"); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	got := string(b)
	if strings.Count(got, "api_key_secret: MY_KEY") != 2 {
		t.Fatalf("api_key_secret was not inserted for each option:\n%s", got)
	}
}

func TestSetLLMAPIKeySecretPreservesModel(t *testing.T) {
	path := filepath.Join(t.TempDir(), "dari.yml")
	original := "name: test\nllm:\n  provider: anthropic\n  model: claude-sonnet-4-6\n"
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
	if !strings.Contains(got, "model: claude-sonnet-4-6") {
		t.Fatalf("model was not preserved:\n%s", got)
	}
	if !strings.Contains(got, "api_key_secret: MY_KEY") {
		t.Fatalf("api_key_secret was not inserted:\n%s", got)
	}
}
