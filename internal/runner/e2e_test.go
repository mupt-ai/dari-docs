package runner

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestRunCheckE2EDefaultFeedbackLLMMatrix(t *testing.T) {
	repo := t.TempDir()
	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("# Test docs\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	outDir := filepath.Join(repo, ".dari-docs")

	var mu sync.Mutex
	var llmIDs []string
	var uploadedBundleSawReadme bool
	sessions := map[string]string{}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer dari_test" {
			t.Fatalf("Authorization = %q", got)
		}
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1/files":
			if !uploadedTarGzContains(t, r, "files/README.md") {
				t.Fatalf("uploaded docs bundle did not contain README.md")
			}
			mu.Lock()
			uploadedBundleSawReadme = true
			mu.Unlock()
			writeJSON(t, w, map[string]any{"id": "file_docs", "filename": "input-docs-bundle.tar.gz", "size_bytes": 123})

		case r.Method == http.MethodPost && r.URL.Path == "/v1/agents/agt_tester/sessions":
			var body struct {
				LLMID string `json:"llm_id"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			mu.Lock()
			llmIDs = append(llmIDs, body.LLMID)
			sessionID := fmt.Sprintf("sess_%02d", len(llmIDs))
			sessions[sessionID] = body.LLMID
			mu.Unlock()
			writeJSON(t, w, map[string]any{"id": sessionID, "agent_id": "agt_tester", "version_id": "ver_tester", "llm_id": body.LLMID, "status": "active"})

		case r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/v1/sessions/") && strings.HasSuffix(r.URL.Path, "/events"):
			sessionID := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/v1/sessions/"), "/events")
			writeJSON(t, w, map[string]any{
				"message": map[string]any{"id": "msg_" + sessionID, "status": "completed"},
				"session": map[string]any{"id": sessionID, "agent_id": "agt_tester", "version_id": "ver_tester", "status": "active", "last_message_id": "msg_" + sessionID, "last_message_status": "completed"},
			})

		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/v1/sessions/") && strings.HasSuffix(r.URL.Path, "/transcript"):
			sessionID := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/v1/sessions/"), "/transcript")
			mu.Lock()
			llmID := sessions[sessionID]
			mu.Unlock()
			writeJSON(t, w, map[string]any{
				"timeline": map[string]any{"items": []any{map[string]any{
					"type": "assistant_message", "status": "completed",
					"content": []any{map[string]any{"type": "text", "text": "feedback from " + llmID}},
				}}},
			})

		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/v1/sessions/"):
			sessionID := strings.TrimPrefix(r.URL.Path, "/v1/sessions/")
			mu.Lock()
			llmID := sessions[sessionID]
			mu.Unlock()
			writeJSON(t, w, map[string]any{"id": sessionID, "agent_id": "agt_tester", "version_id": "ver_tester", "llm_id": llmID, "status": "active", "last_message_id": "msg_" + sessionID, "last_message_status": "completed"})

		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	res, err := Run(context.Background(), Config{
		RepoRoot:      repo,
		OutDir:        outDir,
		APIKey:        "dari_test",
		APIBaseURL:    server.URL,
		FeedbackAgent: "agt_tester",
		Tasks:         []string{"do task"},
		SkipEditor:    true,
		Parallel:      1,
		Timeout:       0,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.FeedbackReports) != 6 {
		t.Fatalf("feedback reports = %d, want 6", len(res.FeedbackReports))
	}

	mu.Lock()
	gotLLMs := append([]string(nil), llmIDs...)
	sawReadme := uploadedBundleSawReadme
	mu.Unlock()
	wantLLMs := []string{"dumb-claude", "medium-claude", "smart-claude", "dumb-gpt", "medium-gpt", "smart-gpt"}
	if strings.Join(gotLLMs, ",") != strings.Join(wantLLMs, ",") {
		t.Fatalf("session llm_id sequence = %#v, want %#v", gotLLMs, wantLLMs)
	}
	if !sawReadme {
		t.Fatal("server did not receive docs bundle with README.md")
	}
	aggregate, err := os.ReadFile(filepath.Join(outDir, "aggregate-feedback.md"))
	if err != nil {
		t.Fatal(err)
	}
	for _, llmID := range wantLLMs {
		if !strings.Contains(string(aggregate), "Tester LLM: "+llmID) {
			t.Fatalf("aggregate missing tester LLM %s:\n%s", llmID, aggregate)
		}
	}
}

func writeJSON(t *testing.T, w http.ResponseWriter, v any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		t.Fatal(err)
	}
}

func uploadedTarGzContains(t *testing.T, r *http.Request, name string) bool {
	t.Helper()
	mr, err := r.MultipartReader()
	if err != nil {
		t.Fatal(err)
	}
	for {
		part, err := mr.NextPart()
		if err == io.EOF {
			return false
		}
		if err != nil {
			t.Fatal(err)
		}
		if part.FormName() != "file" {
			continue
		}
		return tarGzReaderContains(t, part, name)
	}
}

func tarGzReaderContains(t *testing.T, r io.Reader, name string) bool {
	t.Helper()
	gz, err := gzip.NewReader(r)
	if err != nil {
		t.Fatal(err)
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	for {
		h, err := tr.Next()
		if err == io.EOF {
			return false
		}
		if err != nil {
			t.Fatal(err)
		}
		if h.Name == name {
			return true
		}
	}
}
