package managedservice

import (
	"strings"
	"testing"

	"github.com/mupt-ai/dari-docs/internal/dari"
)

func TestNormalizeManagedLLMIDsDefaultsToClaudeMatrix(t *testing.T) {
	got, err := normalizeManagedLLMIDs(nil)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(got, ",") != "dumb-claude,medium-claude,smart-claude" {
		t.Fatalf("managed LLM IDs = %#v", got)
	}
}

func TestNormalizeManagedLLMIDsAllowsGPT(t *testing.T) {
	got, err := normalizeManagedLLMIDs([]string{"smart-gpt"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(got, ",") != "smart-gpt" {
		t.Fatalf("managed LLM IDs = %#v", got)
	}
}

func TestNormalizeManagedLLMIDsRejectsUnknownModel(t *testing.T) {
	if _, err := normalizeManagedLLMIDs([]string{"unknown-model"}); err == nil {
		t.Fatal("expected unknown model rejection")
	}
}

func TestTesterBatchItemsExpandsTasksAcrossManagedLLMs(t *testing.T) {
	run := queuedRun{
		Tasks:        []string{"task one", "task two"},
		TesterLLMIDs: []string{"dumb-claude", "smart-claude"},
	}
	items := testerBatchItems(run)
	if len(items) != 4 {
		t.Fatalf("items = %d, want 4", len(items))
	}
	got := []string{
		items[0].task + ":" + items[0].llmID,
		items[1].task + ":" + items[1].llmID,
		items[2].task + ":" + items[2].llmID,
		items[3].task + ":" + items[3].llmID,
	}
	want := "task one:dumb-claude,task one:smart-claude,task two:dumb-claude,task two:smart-claude"
	if strings.Join(got, ",") != want {
		t.Fatalf("items = %#v", got)
	}
}

func TestMissingTesterBatchItemsReturnsOnlyUnstartedMatrixItems(t *testing.T) {
	run := queuedRun{
		Tasks:        []string{"task one", "task two"},
		TesterLLMIDs: []string{"dumb-claude", "medium-claude"},
	}
	sessions := []runSessionRecord{
		{Kind: "tester", TaskIndex: 1, LLMID: "dumb-claude", Status: statusCompleted},
		{Kind: "tester", TaskIndex: 2, LLMID: "medium-claude", Status: statusRunning},
	}
	items := missingTesterBatchItems(run, sessions)
	got := make([]string, 0, len(items))
	for _, item := range items {
		got = append(got, item.task+":"+item.llmID)
	}
	want := "task one:medium-claude,task two:dumb-claude"
	if strings.Join(got, ",") != want {
		t.Fatalf("missing tester items = %#v, want %s", got, want)
	}
}

func TestTesterBatchIdempotencyKeyKeepsOriginalKeyForFullMatrix(t *testing.T) {
	run := queuedRun{
		ID:           "run_test",
		Tasks:        []string{"task one"},
		TesterLLMIDs: []string{"dumb-claude", "medium-claude"},
	}
	full := testerBatchItems(run)
	if got := testerBatchIdempotencyKey(run, full); got != "dari-docs-managed-run_test-testers" {
		t.Fatalf("full batch key = %q", got)
	}
	partial := full[:1]
	if got := testerBatchIdempotencyKey(run, partial); got == "dari-docs-managed-run_test-testers" {
		t.Fatalf("partial batch reused full batch key")
	}
}

func TestReserveCentsForRunCountsTesterLLMMatrix(t *testing.T) {
	cfg := Config{TesterReserveCents: 75, EditorReserveCents: 150}
	if got := reserveCentsForRun("check", 2, 3, cfg); got != 450 {
		t.Fatalf("check reserve = %d, want 450", got)
	}
	if got := reserveCentsForRun("optimize", 2, 3, cfg); got != 600 {
		t.Fatalf("optimize reserve = %d, want 600", got)
	}
}

func TestSessionLLMIDPreservesStoredRequestedID(t *testing.T) {
	remoteLLMID := "unexpected-remote-llm"
	got := sessionLLMID("smart-claude", dari.Session{LLMID: &remoteLLMID})
	if got != "smart-claude" {
		t.Fatalf("session LLM ID = %q, want stored requested ID", got)
	}
}
