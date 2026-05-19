package managed

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSaveAndLoadToken(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	baseURL := "https://service.example.com/"
	if err := SaveToken(baseURL, "tok_test"); err != nil {
		t.Fatal(err)
	}
	got, err := LoadToken("https://service.example.com")
	if err != nil {
		t.Fatal(err)
	}
	if got != "tok_test" {
		t.Fatalf("token = %q, want tok_test", got)
	}
	info, err := os.Stat(filepath.Join(home, ".dari-docs", "credentials.json"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("credentials mode = %o, want 600", info.Mode().Perm())
	}
}

func TestLoadAuthTokenPrefersEnvToken(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv(EnvTokenName, "env-token")
	if err := SaveToken("https://service.example.com", "local-token"); err != nil {
		t.Fatal(err)
	}
	got, err := LoadAuthToken("https://service.example.com")
	if err != nil {
		t.Fatal(err)
	}
	if got.Token != "env-token" || got.Source != AuthSourceEnv {
		t.Fatalf("auth token = %#v, want env token", got)
	}
}

func TestLoadAuthTokenEmptyEnvDoesNotFallback(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv(EnvTokenName, " ")
	if err := SaveToken("https://service.example.com", "local-token"); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadAuthToken("https://service.example.com"); err == nil {
		t.Fatal("expected empty env token error")
	} else if !strings.Contains(err.Error(), EnvTokenName+" is set but empty") {
		t.Fatalf("err = %v, want empty env token error", err)
	}
}

func TestDeleteToken(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	if err := SaveToken("https://service.example.com", "tok_test"); err != nil {
		t.Fatal(err)
	}
	if err := DeleteToken("https://service.example.com/"); err != nil {
		t.Fatal(err)
	}
	got, err := LoadToken("https://service.example.com")
	if err != nil {
		t.Fatal(err)
	}
	if got != "" {
		t.Fatalf("token = %q, want empty", got)
	}
}

func TestExchangeDariTokenUsesSupabaseBearer(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/auth/dari/exchange" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s, want POST", r.Method)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer supabase-token" {
			t.Fatalf("authorization = %q", got)
		}
		_ = json.NewEncoder(w).Encode(DariExchangeResponse{Token: "managed-token", Email: "user@example.test"})
	}))
	defer server.Close()

	got, err := New(server.URL, "").ExchangeDariToken(context.Background(), "supabase-token")
	if err != nil {
		t.Fatal(err)
	}
	if got.Token != "managed-token" || got.Email != "user@example.test" {
		t.Fatalf("response = %#v", got)
	}
}

func TestCreateRunDoesNotSendAgentSetID(t *testing.T) {
	bundlePath := filepath.Join(t.TempDir(), "bundle.tar.gz")
	if err := os.WriteFile(bundlePath, []byte("bundle"), 0o600); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/runs" {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		mr, err := r.MultipartReader()
		if err != nil {
			t.Fatal(err)
		}
		seen := map[string]bool{}
		fields := map[string]string{}
		for {
			part, err := mr.NextPart()
			if err == io.EOF {
				break
			}
			if err != nil {
				t.Fatal(err)
			}
			seen[part.FormName()] = true
			if part.FormName() != "bundle" {
				b, err := io.ReadAll(part)
				if err != nil {
					t.Fatal(err)
				}
				fields[part.FormName()] = string(b)
			}
		}
		if seen["agent_set_id"] {
			t.Fatalf("managed run request should not include agent_set_id")
		}
		for _, want := range []string{"mode", "tasks_json", "feedback_llm_ids_json", "editor_llm_id", "bundle"} {
			if !seen[want] {
				t.Fatalf("missing multipart field %q; seen=%#v", want, seen)
			}
		}
		if fields["feedback_llm_ids_json"] != `["dumb-claude","smart-claude"]` {
			t.Fatalf("feedback_llm_ids_json = %q", fields["feedback_llm_ids_json"])
		}
		if fields["editor_llm_id"] != "smart-claude" {
			t.Fatalf("editor_llm_id = %q", fields["editor_llm_id"])
		}
		_ = json.NewEncoder(w).Encode(CreateRunResponse{RunID: "run_test", Status: "queued"})
	}))
	defer server.Close()

	got, err := New(server.URL, "managed-token").CreateRun(context.Background(), "check", []string{"task"}, bundlePath, CreateRunOptions{
		FeedbackLLMIDs: []string{"dumb-claude", "smart-claude"},
		EditorLLMID:    "smart-claude",
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.RunID != "run_test" || got.Status != "queued" {
		t.Fatalf("response = %#v", got)
	}
}

func TestLogoutAllUsesEndpoint(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/auth/logout-all" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s, want POST", r.Method)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer managed-token" {
			t.Fatalf("authorization = %q", got)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	if err := New(server.URL, "managed-token").LogoutAll(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestLogoutAllKindUsesKindQuery(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/auth/logout-all" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		if got := r.URL.Query().Get("kind"); got != "automation" {
			t.Fatalf("kind = %q, want automation", got)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	if err := New(server.URL, "managed-token").LogoutAllKind(context.Background(), "automation"); err != nil {
		t.Fatal(err)
	}
}

func TestInvalidEnvTokenDoesNotFallbackToLocalCredentials(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv(EnvTokenName, "env-token")
	if err := SaveToken("https://service.example.com", "local-token"); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer env-token" {
			t.Fatalf("authorization = %q, want env token", got)
		}
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	}))
	defer server.Close()

	auth, err := LoadAuthToken("https://service.example.com")
	if err != nil {
		t.Fatal(err)
	}
	client := NewWithAuthToken(server.URL, auth)
	if _, err := client.Me(context.Background()); err == nil {
		t.Fatal("expected invalid env token error")
	} else if _, ok := err.(*InvalidEnvTokenError); !ok {
		t.Fatalf("err = %T %v, want InvalidEnvTokenError", err, err)
	}
}

func TestAuthTokenClientEndpoints(t *testing.T) {
	var sawCreate, sawList, sawRevoke bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer managed-token" {
			t.Fatalf("authorization = %q", got)
		}
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1/auth/tokens":
			sawCreate = true
			var req TokenCreateRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatal(err)
			}
			if req.Name != "github-actions" {
				t.Fatalf("name = %q", req.Name)
			}
			_ = json.NewEncoder(w).Encode(TokenCreateResponse{
				AuthTokenInfo: AuthTokenInfo{ID: "tok_123", Name: req.Name, Kind: "automation", Scopes: []string{"managed:check"}},
				Token:         "mdt_v1_tok_123_secret",
			})
		case r.Method == http.MethodGet && r.URL.Path == "/v1/auth/tokens":
			sawList = true
			_ = json.NewEncoder(w).Encode(TokenListResponse{Tokens: []AuthTokenInfo{{ID: "tok_123", Name: "github-actions", Kind: "automation"}}})
		case r.Method == http.MethodPost && r.URL.Path == "/v1/auth/tokens/tok_123/revoke":
			sawRevoke = true
			w.WriteHeader(http.StatusOK)
		default:
			t.Fatalf("unexpected %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	client := New(server.URL, "managed-token")
	created, err := client.CreateAuthToken(context.Background(), TokenCreateRequest{Name: "github-actions"})
	if err != nil {
		t.Fatal(err)
	}
	if created.Token != "mdt_v1_tok_123_secret" {
		t.Fatalf("created token = %q", created.Token)
	}
	listed, err := client.ListAuthTokens(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(listed.Tokens) != 1 || listed.Tokens[0].ID != "tok_123" {
		t.Fatalf("listed = %#v", listed)
	}
	if err := client.RevokeAuthToken(context.Background(), "tok_123"); err != nil {
		t.Fatal(err)
	}
	if !sawCreate || !sawList || !sawRevoke {
		t.Fatalf("missing endpoint calls create=%v list=%v revoke=%v", sawCreate, sawList, sawRevoke)
	}
}
