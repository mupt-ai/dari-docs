package platformauth

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"
)

const (
	LoginTimeout       = 5 * time.Minute
	callbackStateParam = "dari_docs_state"
)

type Config struct {
	SupabaseURL            string   `json:"supabase_url"`
	SupabasePublishableKey string   `json:"supabase_publishable_key"`
	Providers              []string `json:"providers"`
}

type Session struct {
	AccessToken string
	Email       string
}

func FetchConfig(ctx context.Context, apiBaseURL string) (Config, error) {
	apiBaseURL = strings.TrimRight(apiBaseURL, "/")
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiBaseURL+"/v1/auth/config", nil)
	if err != nil {
		return Config{}, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return Config{}, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return Config{}, fmt.Errorf("fetch Dari auth config: http %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var cfg Config
	if err := json.Unmarshal(body, &cfg); err != nil {
		return Config{}, fmt.Errorf("decode Dari auth config: %w", err)
	}
	cfg.SupabaseURL = strings.TrimRight(cfg.SupabaseURL, "/")
	if cfg.SupabaseURL == "" || cfg.SupabasePublishableKey == "" {
		return Config{}, errors.New("Dari auth config response was invalid")
	}
	return cfg, nil
}

func LoginWithBrowser(ctx context.Context, cfg Config, stdin io.Reader, stderr io.Writer) (Session, error) {
	if stdin == nil {
		stdin = os.Stdin
	}
	if stderr == nil {
		stderr = io.Discard
	}
	verifier, challenge, err := newPKCEPair()
	if err != nil {
		return Session{}, err
	}
	state, err := newOAuthState()
	if err != nil {
		return Session{}, err
	}
	callback, err := startCallbackServer(state)
	if err != nil {
		return Session{}, err
	}
	defer callback.Close()

	authRedirectURL, err := redirectURLWithState(callback.RedirectURL, state)
	if err != nil {
		return Session{}, err
	}
	loginURL := buildAuthorizeURL(cfg, authRedirectURL, challenge)
	if openBrowser(loginURL) {
		fmt.Fprintf(stderr, "Waiting for browser login. If it does not complete automatically, open this URL:\n  %s\n\nAfter signing in, paste the localhost callback URL below.\n", loginURL)
	} else {
		fmt.Fprintf(stderr, "Open this URL in a browser to continue login:\n  %s\n\nAfter signing in, paste the localhost callback URL below.\n", loginURL)
	}
	cb, err := callback.WaitOrInput(ctx, stdin, LoginTimeout, stderr, state)
	if err != nil {
		return Session{}, err
	}
	if cb.Error != "" {
		return Session{}, fmt.Errorf("browser login failed: %s", cb.Error)
	}
	return exchangeCode(ctx, cfg, cb.Code, verifier, authRedirectURL)
}

func buildAuthorizeURL(cfg Config, redirectURL, challenge string) string {
	q := url.Values{}
	q.Set("provider", "google")
	q.Set("redirect_to", redirectURL)
	q.Set("code_challenge", challenge)
	q.Set("code_challenge_method", "S256")
	q.Set("flow_type", "pkce")
	return cfg.SupabaseURL + "/auth/v1/authorize?" + q.Encode()
}

func redirectURLWithState(rawURL, state string) (string, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", fmt.Errorf("parse callback redirect URL: %w", err)
	}
	q := u.Query()
	q.Set(callbackStateParam, state)
	u.RawQuery = q.Encode()
	return u.String(), nil
}

func exchangeCode(ctx context.Context, cfg Config, code, verifier, redirectURL string) (Session, error) {
	payload := map[string]string{"auth_code": code, "code_verifier": verifier}
	body, err := json.Marshal(payload)
	if err != nil {
		return Session{}, err
	}
	q := url.Values{}
	q.Set("grant_type", "pkce")
	q.Set("redirect_to", redirectURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, cfg.SupabaseURL+"/auth/v1/token?"+q.Encode(), bytes.NewReader(body))
	if err != nil {
		return Session{}, err
	}
	req.Header.Set("apiKey", cfg.SupabasePublishableKey)
	req.Header.Set("Authorization", "Bearer "+cfg.SupabasePublishableKey)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return Session{}, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return Session{}, fmt.Errorf("exchange auth code: http %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}
	var out struct {
		AccessToken string `json:"access_token"`
		User        struct {
			Email string `json:"email"`
		} `json:"user"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return Session{}, fmt.Errorf("decode auth session: %w", err)
	}
	if out.AccessToken == "" {
		return Session{}, errors.New("auth session response missing access token")
	}
	return Session{AccessToken: out.AccessToken, Email: out.User.Email}, nil
}

func newPKCEPair() (verifier string, challenge string, err error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", "", err
	}
	verifier = base64.RawURLEncoding.EncodeToString(b)
	sum := sha256.Sum256([]byte(verifier))
	challenge = base64.RawURLEncoding.EncodeToString(sum[:])
	return verifier, challenge, nil
}

func newOAuthState() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

type callbackResult struct {
	Code  string
	Error string
	State string
}

type callbackServer struct {
	RedirectURL string
	listener    net.Listener
	srv         *http.Server
	result      chan callbackResult
	once        sync.Once
}

func startCallbackServer(expectedState string) (*callbackServer, error) {
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("bind local callback listener: %w", err)
	}
	addr := lis.Addr().(*net.TCPAddr)
	cb := &callbackServer{
		RedirectURL: fmt.Sprintf("http://127.0.0.1:%d/callback", addr.Port),
		listener:    lis,
		result:      make(chan callbackResult, 1),
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		res := callbackResult{Code: q.Get("code"), Error: firstNonEmpty(q.Get("error_description"), q.Get("error")), State: q.Get(callbackStateParam)}
		body := "dari-docs login complete. You can close this tab.\n"
		status := http.StatusOK
		if err := validateOAuthState(expectedState, res.State); err != nil {
			res.Code = ""
			res.Error = err.Error()
			body = "dari-docs login failed: " + res.Error + "\n"
			status = http.StatusBadRequest
		} else if res.Error != "" {
			body = "dari-docs login failed: " + res.Error + "\n"
			status = http.StatusBadRequest
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
		cb.once.Do(func() { cb.result <- res })
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	})
	cb.srv = &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	go func() { _ = cb.srv.Serve(lis) }()
	return cb, nil
}

func (cb *callbackServer) WaitOrInput(ctx context.Context, stdin io.Reader, timeout time.Duration, stderr io.Writer, expectedState string) (callbackResult, error) {
	fmt.Fprint(stderr, "Paste callback URL: ")
	lineCh := make(chan string, 1)
	errCh := make(chan error, 1)
	go func() {
		scanner := bufio.NewScanner(stdin)
		if scanner.Scan() {
			lineCh <- scanner.Text()
			return
		}
		if err := scanner.Err(); err != nil {
			errCh <- err
			return
		}
		errCh <- io.EOF
	}()
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	select {
	case res := <-cb.result:
		if res.Code == "" && res.Error == "" {
			return res, errors.New("browser callback did not include a code")
		}
		return res, nil
	case line := <-lineCh:
		return parseManualCallback(line, expectedState)
	case err := <-errCh:
		return callbackResult{}, err
	case <-deadline.C:
		return callbackResult{}, errors.New("timed out waiting for browser login to complete")
	case <-ctx.Done():
		return callbackResult{}, ctx.Err()
	}
}

func (cb *callbackServer) Close() {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_ = cb.srv.Shutdown(ctx)
}

func parseManualCallback(raw, expectedState string) (callbackResult, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return callbackResult{}, errors.New("empty callback URL")
	}
	if strings.HasPrefix(raw, "http://") || strings.HasPrefix(raw, "https://") {
		u, err := url.Parse(raw)
		if err != nil {
			return callbackResult{}, fmt.Errorf("parse callback URL: %w", err)
		}
		q := u.Query()
		res := callbackResult{Code: q.Get("code"), Error: firstNonEmpty(q.Get("error_description"), q.Get("error")), State: q.Get(callbackStateParam)}
		if err := validateOAuthState(expectedState, res.State); err != nil {
			return res, err
		}
		if res.Code == "" && res.Error == "" {
			return res, errors.New("callback URL did not include a code")
		}
		return res, nil
	}
	if expectedState != "" {
		return callbackResult{}, errors.New("paste the full callback URL so OAuth state can be verified")
	}
	return callbackResult{Code: raw}, nil
}

func validateOAuthState(expected, actual string) error {
	if expected == "" {
		return errors.New("OAuth state was not initialized")
	}
	if actual == "" {
		return errors.New("OAuth callback did not include state")
	}
	if actual != expected {
		return errors.New("OAuth callback state did not match")
	}
	return nil
}

func openBrowser(rawURL string) bool {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", rawURL)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", rawURL)
	default:
		cmd = exec.Command("xdg-open", rawURL)
	}
	return cmd.Start() == nil
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
