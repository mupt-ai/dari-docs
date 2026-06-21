package flueclient

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunSendsTesterModelMatrix(t *testing.T) {
	repo := t.TempDir()
	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("# Docs\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var gotModels []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/workflows/test" || r.URL.Query().Get("wait") != "result" {
			t.Fatalf("unexpected request %s?%s", r.URL.Path, r.URL.RawQuery)
		}
		var payload testerPayload
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		gotModels = append(gotModels, payload.Model)
		_ = json.NewEncoder(w).Encode(workflowResponse[testerResult]{Result: testerResult{Feedback: "ok " + payload.Model}})
	}))
	defer server.Close()

	_, err := Run(context.Background(), Config{
		RepoRoot:       repo,
		OutDir:         filepath.Join(repo, ".dari-docs"),
		TesterURL:      server.URL,
		Tasks:          []string{"Install"},
		FeedbackModels: []string{"gpt-5.5", "claude-opus-4-8"},
		SkipEditor:     true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(gotModels, ",") != "gpt-5.5,claude-opus-4-8" {
		t.Fatalf("models = %#v", gotModels)
	}
	if _, err := os.Stat(filepath.Join(repo, ".dari-docs", "runs", "feedback-001-gpt-5.5.md")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(repo, ".dari-docs", "runs", "feedback-002-claude-opus-4-8.md")); err != nil {
		t.Fatal(err)
	}
}

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

func TestFormatFeedbackReportIncludesTesterModel(t *testing.T) {
	report := formatFeedbackReport(0, 1, "gpt-5.5", "feedback")
	if !strings.Contains(report, "Tester model: gpt-5.5") {
		t.Fatalf("report missing model header:\n%s", report)
	}
}

func TestFeedbackModelsOrDefaultDeduplicates(t *testing.T) {
	got := feedbackModelsOrDefault([]string{"gpt-5.5", " ", "gpt-5.5", "claude-sonnet-4-6"})
	if strings.Join(got, ",") != "gpt-5.5,claude-sonnet-4-6" {
		t.Fatalf("models = %#v", got)
	}
	if got := feedbackModelsOrDefault(nil); len(got) != 1 || got[0] != "" {
		t.Fatalf("default models = %#v", got)
	}
}
