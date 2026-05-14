package platformauth

import (
	"net/url"
	"testing"
)

func TestParseManualCallbackAcceptsCallbackURL(t *testing.T) {
	got, err := parseManualCallback("http://127.0.0.1:12345/callback?code=abc123&dari_docs_state=state123", "state123")
	if err != nil {
		t.Fatal(err)
	}
	if got.Code != "abc123" {
		t.Fatalf("code = %q, want abc123", got.Code)
	}
}

func TestParseManualCallbackRejectsWrongState(t *testing.T) {
	_, err := parseManualCallback("http://127.0.0.1:12345/callback?code=abc123&dari_docs_state=wrong", "state123")
	if err == nil {
		t.Fatal("expected state mismatch error")
	}
}

func TestParseManualCallbackRejectsMissingState(t *testing.T) {
	_, err := parseManualCallback("http://127.0.0.1:12345/callback?code=abc123", "state123")
	if err == nil {
		t.Fatal("expected missing state error")
	}
}

func TestParseManualCallbackRejectsRawCodeWhenStateIsRequired(t *testing.T) {
	_, err := parseManualCallback("abc123", "state123")
	if err == nil {
		t.Fatal("expected raw code error")
	}
}

func TestPKCEPairShape(t *testing.T) {
	verifier, challenge, err := newPKCEPair()
	if err != nil {
		t.Fatal(err)
	}
	if len(verifier) < 43 {
		t.Fatalf("verifier length = %d, want at least 43", len(verifier))
	}
	if len(challenge) == 0 {
		t.Fatal("challenge is empty")
	}
}

func TestOAuthStateShape(t *testing.T) {
	state, err := newOAuthState()
	if err != nil {
		t.Fatal(err)
	}
	if len(state) < 32 {
		t.Fatalf("state length = %d, want at least 32", len(state))
	}
}

func TestBuildAuthorizeURLIncludesState(t *testing.T) {
	redirectURL, err := redirectURLWithState("http://127.0.0.1/callback", "state123")
	if err != nil {
		t.Fatal(err)
	}
	raw := buildAuthorizeURL(Config{SupabaseURL: "https://auth.example.test"}, redirectURL, "challenge")
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	if got := q.Get("state"); got != "" {
		t.Fatalf("top-level state = %q, want empty so Supabase can manage provider state", got)
	}
	gotRedirect := q.Get("redirect_to")
	redirect, err := url.Parse(gotRedirect)
	if err != nil {
		t.Fatal(err)
	}
	if got := redirect.Query().Get(callbackStateParam); got != "state123" {
		t.Fatalf("callback state = %q, want state123", got)
	}
	if got := q.Get("code_challenge"); got != "challenge" {
		t.Fatalf("code_challenge = %q, want challenge", got)
	}
	if got := q.Get("code_challenge_method"); got != "S256" {
		t.Fatalf("code_challenge_method = %q, want S256", got)
	}
	if got := q.Get("flow_type"); got != "pkce" {
		t.Fatalf("flow_type = %q, want pkce", got)
	}
}
