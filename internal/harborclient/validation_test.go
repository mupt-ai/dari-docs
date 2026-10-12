package harborclient

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestWorkflowResponseValidation(t *testing.T) {
	tests := []struct {
		name, workflow, body string
		valid                bool
	}{
		{"missing envelope", "test", `{}`, false},
		{"null envelope", "test", `{"result":null}`, false},
		{"empty body", "test", ``, false},
		{"array envelope", "test", `[]`, false},
		{"array result", "test", `{"result":[]}`, false},
		{"missing feedback", "test", `{"result":{}}`, false},
		{"null feedback", "test", `{"result":{"feedback":null}}`, false},
		{"empty feedback", "test", `{"result":{"feedback":"  "}}`, false},
		{"wrong feedback type", "test", `{"result":{"feedback":42}}`, false},
		{"valid feedback", "test", `{"result":{"feedback":"feedback"}}`, true},
		{"missing changelog", "edit", `{"result":{"files":[]}}`, false},
		{"empty changelog", "edit", `{"result":{"changelog":" ","files":[]}}`, false},
		{"missing files", "edit", `{"result":{"changelog":"changes"}}`, false},
		{"null files", "edit", `{"result":{"changelog":"changes","files":null}}`, false},
		{"missing content", "edit", `{"result":{"changelog":"changes","files":[{"path":"README.md"}]}}`, false},
		{"null content", "edit", `{"result":{"changelog":"changes","files":[{"path":"README.md","content":null}]}}`, false},
		{"unsafe path", "edit", `{"result":{"changelog":"changes","files":[{"path":"../escape","content":"x"}]}}`, false},
		{"duplicate paths", "edit", `{"result":{"changelog":"changes","files":[{"path":"README.md","content":"x"},{"path":"./README.md","content":"y"}]}}`, false},
		{"empty content is valid", "edit", `{"result":{"changelog":"changes","files":[{"path":"README.md","content":""}]}}`, true},
		{"no input docs permits empty files", "edit", `{"result":{"changelog":"changes","files":[]}}`, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(tt.body)) }))
			defer server.Close()
			var err error
			if tt.workflow == "test" {
				_, err = callWorkflow[testerResult](context.Background(), server.Client(), server.URL, tt.workflow, "", testerPayload{})
			} else {
				_, err = callWorkflow[editorResult](context.Background(), server.Client(), server.URL, tt.workflow, "", editorPayload{})
			}
			if (err == nil) != tt.valid {
				t.Fatalf("err = %v, valid = %v", err, tt.valid)
			}
		})
	}
}

func TestRunDefaultsTimeoutToSixtyMinutes(t *testing.T) {
	if defaultWorkflowTimeout != 60*time.Minute {
		t.Fatalf("default timeout = %s, want 60m", defaultWorkflowTimeout)
	}
}

func TestRunRejectsNegativeTimeout(t *testing.T) {
	_, err := Run(context.Background(), Config{Timeout: -time.Minute})
	if err == nil || !strings.Contains(err.Error(), "timeout") {
		t.Fatalf("err = %v", err)
	}
}

func TestInvalidEditorResponsePreservesPreviousOutput(t *testing.T) {
	// Validation must happen before removing existing downloaded docs.
	repo := t.TempDir()
	out := t.TempDir()
	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("original"), 0o644); err != nil {
		t.Fatal(err)
	}
	updated := filepath.Join(out, "updated")
	if err := os.MkdirAll(updated, 0o755); err != nil {
		t.Fatal(err)
	}
	previous := filepath.Join(updated, "README.md")
	if err := os.WriteFile(previous, []byte("previous"), 0o644); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/test") {
			_, _ = w.Write([]byte(`{"result":{"feedback":"feedback"}}`))
		} else {
			_, _ = w.Write([]byte(`{"result":{"changelog":"nothing","files":[]}}`))
		}
	}))
	defer server.Close()
	_, err := Run(context.Background(), Config{RepoRoot: repo, OutDir: out, TesterURL: server.URL, EditorURL: server.URL, Tasks: []string{"task"}})
	if err == nil {
		t.Fatal("expected empty editor output error")
	}
	b, err := os.ReadFile(previous)
	if err != nil || string(b) != "previous" {
		t.Fatalf("previous output: %q, %v", b, err)
	}
}
