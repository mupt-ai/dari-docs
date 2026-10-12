package harborclient

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"testing"

	"github.com/mupt-ai/dari-docs/internal/projectconfig"
)

func TestWorkflowExecutionAndAuthorization(t *testing.T) {
	for _, tt := range []struct {
		name      string
		execution *projectconfig.Execution
		editor    *projectconfig.Execution
		token     string
	}{
		{name: "server defaults"},
		{name: "custom harness and editor override", execution: &projectconfig.Execution{Harness: "custom.agent:Agent", Sandbox: "custom.env:Environment", AgentKwargs: map[string]any{"count": float64(2)}, AgentEnv: map[string]string{"MODE": "test"}, EnvironmentKwargs: map[string]any{"region": "east"}}, editor: &projectconfig.Execution{Harness: "custom.editor:Agent"}, token: "runner-token"},
		{name: "empty execution omitted", execution: &projectconfig.Execution{}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			repo := t.TempDir()
			if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("docs"), 0o644); err != nil {
				t.Fatal(err)
			}
			var mu sync.Mutex
			seen := map[string]map[string]any{}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost || r.URL.Query().Get("wait") != "result" {
					t.Errorf("unexpected request %s %s", r.Method, r.URL)
				}
				wantAuth := ""
				if tt.token != "" {
					wantAuth = "Bearer " + tt.token
				}
				if got := r.Header.Get("Authorization"); got != wantAuth {
					t.Errorf("authorization = %q, want %q", got, wantAuth)
				}
				var payload map[string]any
				if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
					t.Error(err)
				}
				mu.Lock()
				seen[r.URL.Path] = payload
				mu.Unlock()
				w.Header().Set("Content-Type", "application/json")
				if r.URL.Path == "/workflows/test" {
					_, _ = w.Write([]byte(`{"result":{"feedback":"feedback"}}`))
				} else if r.URL.Path == "/workflows/edit" {
					_, _ = w.Write([]byte(`{"result":{"files":[{"path":"README.md","content":"updated"}]}}`))
				} else {
					t.Errorf("unexpected path %s", r.URL.Path)
					w.WriteHeader(404)
				}
			}))
			defer server.Close()
			_, err := Run(context.Background(), Config{RepoRoot: repo, OutDir: filepath.Join(repo, "out"), TesterURL: server.URL, EditorURL: server.URL, Tasks: []string{"task"}, Execution: tt.execution, EditorExecution: tt.editor, RunnerToken: tt.token})
			if err != nil {
				t.Fatal(err)
			}
			for path, payload := range seen {
				want := projectconfig.MergeExecution(nil, tt.execution)
				if path == "/workflows/edit" {
					want = projectconfig.MergeExecution(want, tt.editor)
				}
				actual, exists := payload["execution"]
				if want == nil {
					if exists {
						t.Fatalf("execution not omitted: %#v", actual)
					}
					continue
				}
				b, _ := json.Marshal(want)
				var expected any
				if err := json.Unmarshal(b, &expected); err != nil {
					t.Fatal(err)
				}
				if !reflect.DeepEqual(actual, expected) {
					t.Fatalf("%s execution = %#v, want %#v", path, actual, expected)
				}
			}
			if len(seen) != 2 {
				t.Fatalf("workflow count = %d", len(seen))
			}
		})
	}
}

func TestRunRejectsSecretAgentEnvBeforeRequest(t *testing.T) {
	_, err := Run(context.Background(), Config{Execution: &projectconfig.Execution{AgentEnv: map[string]string{"API_TOKEN": "secret-value"}}})
	if err == nil {
		t.Fatal("expected secret rejection")
	}
}
