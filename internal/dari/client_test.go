package dari

import (
	"archive/zip"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCreateSessionBatchSendsItems(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/session-batches" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		var got CreateSessionBatchRequest
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Fatal(err)
		}
		if got.IdempotencyKey != "batch-key" || len(got.Items) != 1 {
			t.Fatalf("batch request = %#v", got)
		}
		item := got.Items[0]
		if item.AgentID != "agt_test" || item.LLMID != "smart-claude" || item.Metadata["kind"] != "tester" || len(item.Message.Content) != 1 {
			t.Fatalf("batch item = %#v", item)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"batch_id":"batch_test","status":"queued","sessions":[{"index":0,"session_id":"sess_test","status":"queued","last_message_status":"queued","agent_id":"agt_test","version_id":"ver_test","llm_id":"smart-claude","metadata":{"kind":"tester"}}]}`))
	}))
	defer server.Close()

	batch, err := New(server.URL, "dari_test").CreateSessionBatch(context.Background(), CreateSessionBatchRequest{
		IdempotencyKey: "batch-key",
		Items: []CreateSessionBatchItem{{
			AgentID:  "agt_test",
			LLMID:    "smart-claude",
			Metadata: map[string]string{"kind": "tester"},
			Message:  CreateSessionBatchMessage{Content: []ContentBlock{TextBlock("hello")}},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if batch.BatchID != "batch_test" || len(batch.Sessions) != 1 || batch.Sessions[0].SessionID != "sess_test" {
		t.Fatalf("batch response = %#v", batch)
	}
}

func TestGetAgentVersionParsesVersionDetail(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/agents/agt_test/versions/ver_test" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"agent":{"id":"agt_test","active_version_id":"ver_test"},"version":{"id":"ver_test","agent_id":"agt_test"}}`))
	}))
	defer server.Close()

	detail, err := New(server.URL, "dari_test").GetAgentVersion(context.Background(), "agt_test", "ver_test")
	if err != nil {
		t.Fatal(err)
	}
	if detail.Agent.ID != "agt_test" || detail.Version.ID != "ver_test" || detail.Version.AgentID != "agt_test" {
		t.Fatalf("detail = %#v", detail)
	}
}

func TestGetAgentVersionReturnsHTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "missing version", http.StatusNotFound)
	}))
	defer server.Close()

	_, err := New(server.URL, "dari_test").GetAgentVersion(context.Background(), "agt_test", "ver_missing")
	var httpErr *HTTPError
	if !errors.As(err, &httpErr) {
		t.Fatalf("err = %T %[1]v, want HTTPError", err)
	}
	if httpErr.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", httpErr.StatusCode, http.StatusNotFound)
	}
}

func TestDownloadWorkspaceZipWithLimitRejectsOversizedResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/sessions/sess_test/workspace.zip" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		if got := r.URL.Query()["path"]; len(got) != 1 || got[0] != "updated-docs" {
			t.Fatalf("path query = %#v", got)
		}
		_, _ = w.Write([]byte(strings.Repeat("x", 6)))
	}))
	defer server.Close()

	outPath := filepath.Join(t.TempDir(), "workspace.zip")
	err := New(server.URL, "dari_test").DownloadWorkspaceZipWithLimit(context.Background(), "sess_test", []string{"updated-docs"}, outPath, 5)
	if err == nil || !strings.Contains(err.Error(), "exceeds size limit") {
		t.Fatalf("err = %v, want size limit error", err)
	}
	if _, statErr := os.Stat(outPath); !os.IsNotExist(statErr) {
		t.Fatalf("downloaded file exists after oversized response: stat err = %v", statErr)
	}
}

func TestExtractZipWithLimitRejectsZipBomb(t *testing.T) {
	zipPath := filepath.Join(t.TempDir(), "workspace.zip")
	if err := writeZipForTest(zipPath, map[string]string{"updated-docs/files/README.md": strings.Repeat("x", 6)}); err != nil {
		t.Fatal(err)
	}
	dest := t.TempDir()
	err := ExtractZipWithLimit(zipPath, dest, 5)
	if err == nil || !strings.Contains(err.Error(), "uncompressed size limit") {
		t.Fatalf("err = %v, want uncompressed size limit", err)
	}
	if _, statErr := os.Stat(filepath.Join(dest, "updated-docs", "files", "README.md")); !os.IsNotExist(statErr) {
		t.Fatalf("extracted oversized file exists: stat err = %v", statErr)
	}
}

func TestExtractZipRejectsTraversal(t *testing.T) {
	zipPath := filepath.Join(t.TempDir(), "workspace.zip")
	if err := writeZipForTest(zipPath, map[string]string{"../escape.md": "bad"}); err != nil {
		t.Fatal(err)
	}
	err := ExtractZipWithLimit(zipPath, t.TempDir(), 1024)
	if err == nil || !strings.Contains(err.Error(), "unsafe zip path") {
		t.Fatalf("err = %v, want unsafe zip path", err)
	}
}

func writeZipForTest(path string, files map[string]string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	zw := zip.NewWriter(f)
	for name, content := range files {
		w, err := zw.Create(name)
		if err != nil {
			_ = zw.Close()
			_ = f.Close()
			return err
		}
		if _, err := io.WriteString(w, content); err != nil {
			_ = zw.Close()
			_ = f.Close()
			return err
		}
	}
	if err := zw.Close(); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}
