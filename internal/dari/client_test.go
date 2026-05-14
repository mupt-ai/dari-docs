package dari

import (
	"archive/zip"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

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
