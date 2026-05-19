package managedservice

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/mupt-ai/dari-docs/internal/dari"
)

const (
	statusQueued    = "queued"
	statusUploading = "uploading"
	statusStarting  = "starting"
	statusRunning   = "running"
	statusCompleted = "completed"
	statusFailed    = "failed"
)

type Server struct {
	cfg        Config
	db         *pgxpool.Pool
	runStore   *managedRunStore
	dari       *dari.Client
	httpClient *http.Client
}

func Run(ctx context.Context, cfg Config) error {
	if err := runMigrations(ctx, cfg.DatabaseURL); err != nil {
		return err
	}
	db, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer db.Close()
	s := &Server{
		cfg:        cfg,
		db:         db,
		runStore:   newManagedRunStore(db),
		dari:       dari.New(cfg.DariAPIBaseURL, cfg.DariAPIKey),
		httpClient: &http.Client{Timeout: cfg.OutboundHTTPTimeout},
	}
	srv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           s.routes(),
		ReadHeaderTimeout: cfg.HTTPReadHeaderTimeout,
		ReadTimeout:       cfg.HTTPReadTimeout,
		WriteTimeout:      cfg.HTTPWriteTimeout,
		IdleTimeout:       cfg.HTTPIdleTimeout,
	}
	go s.sessionStarterLoop(ctx)
	go s.sessionReconcilerLoop(ctx)
	go s.settlementLoop(ctx)
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	}()
	log.Printf("dari-docs service listening on %s", cfg.Addr)
	err = srv.ListenAndServe()
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

func (s *Server) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", s.handleHealth)
	mux.HandleFunc("/readyz", s.handleReady)
	mux.HandleFunc("/v1/auth/config", s.handleAuthConfig)
	mux.HandleFunc("/v1/auth/dari/exchange", s.handleDariAuthExchange)
	mux.HandleFunc("/v1/auth/logout", s.withAuth(s.handleLogout))
	mux.HandleFunc("/v1/auth/logout-all", s.withUserAuth(s.handleLogoutAll))
	mux.HandleFunc("/v1/auth/tokens", s.withUserAuth(s.handleAuthTokens))
	mux.HandleFunc("/v1/auth/tokens/", s.withUserAuth(s.handleAuthTokenByID))
	mux.HandleFunc("/v1/me", s.withUserAuth(s.handleMe))
	mux.HandleFunc("/v1/billing/balance", s.withUserAuth(s.handleBalance))
	mux.HandleFunc("/v1/billing/config", s.withUserAuth(s.handleBillingConfig))
	mux.HandleFunc("/v1/billing/checkout", s.withUserAuth(s.handleCheckout))
	mux.HandleFunc("/v1/runs/config", s.withUserAuth(s.handleRunConfig))
	mux.HandleFunc("/v1/stripe/webhook", s.handleStripeWebhook)
	mux.HandleFunc("/billing/success", s.handleBillingSuccess)
	mux.HandleFunc("/billing/cancel", s.handleBillingCancel)
	mux.HandleFunc("/v1/runs", s.withUserAuth(s.handleRuns))
	mux.HandleFunc("/v1/runs/", s.withUserAuth(s.handleRunByID))
	mux.HandleFunc("/", s.handleFrontend)
	return mux
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleReady(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if s.db == nil {
		writeError(w, http.StatusServiceUnavailable, "database is not configured")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	if err := s.db.Ping(ctx); err != nil {
		writeError(w, http.StatusServiceUnavailable, "database is not ready")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
}

func (s *Server) handleFrontend(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if r.URL.Path == "/v1" || strings.HasPrefix(r.URL.Path, "/v1/") {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	dist := filepath.Join("web", "dist")
	requestPath := filepath.Clean(strings.TrimPrefix(r.URL.Path, "/"))
	if requestPath == "." {
		requestPath = "index.html"
	}
	candidate := filepath.Join(dist, requestPath)
	if !strings.HasPrefix(candidate, dist+string(os.PathSeparator)) && candidate != filepath.Join(dist, "index.html") {
		writeError(w, http.StatusBadRequest, "invalid path")
		return
	}
	if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
		http.ServeFile(w, r, candidate)
		return
	}
	index := filepath.Join(dist, "index.html")
	if _, err := os.Stat(index); err != nil {
		writeError(w, http.StatusNotFound, "frontend is not built")
		return
	}
	http.ServeFile(w, r, index)
}

var defaultOutboundHTTPClient = &http.Client{Timeout: 30 * time.Second}

func (s *Server) outboundHTTPClient() *http.Client {
	if s.httpClient != nil {
		return s.httpClient
	}
	return defaultOutboundHTTPClient
}

func readJSON(r *http.Request, out any) error {
	defer r.Body.Close()
	dec := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
	dec.DisallowUnknownFields()
	return dec.Decode(out)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeHTML(w http.ResponseWriter, status int, title, body string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	_, _ = fmt.Fprintf(w, `<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>%s</title>
  <style>
    body { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif; margin: 0; padding: 48px 24px; color: #171717; background: #fafafa; }
    main { max-width: 640px; margin: 0 auto; }
    h1 { font-size: 28px; line-height: 1.2; margin: 0 0 12px; }
    p { font-size: 16px; line-height: 1.5; margin: 0; color: #404040; }
  </style>
</head>
<body><main><h1>%s</h1><p>%s</p></main></body>
</html>`, htmlEscape(title), htmlEscape(title), htmlEscape(body))
}

func htmlEscape(v string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;", "'", "&#39;").Replace(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func writeLoggedError(w http.ResponseWriter, status int, msg string, err error) {
	if err != nil {
		log.Printf("%s: %v", msg, err)
	}
	writeError(w, status, msg)
}

func isRequestBodyTooLarge(err error) bool {
	var maxBytesErr *http.MaxBytesError
	if errors.As(err, &maxBytesErr) {
		return true
	}
	return strings.Contains(err.Error(), "http: request body too large")
}

func randomToken(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

func sha256Hex(v string) string {
	sum := sha256.Sum256([]byte(v))
	return hex.EncodeToString(sum[:])
}

func formatCents(cents int64) string {
	sign := ""
	if cents < 0 {
		sign = "-"
		cents = -cents
	}
	return fmt.Sprintf("%s$%d.%02d", sign, cents/100, cents%100)
}
