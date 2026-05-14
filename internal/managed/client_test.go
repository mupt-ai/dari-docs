package managed

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
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
