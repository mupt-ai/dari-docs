package managed

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	DefaultBaseURL = "https://dari-docs.dari.dev"
	EnvTokenName   = "DARI_DOCS_TOKEN"
)

const (
	defaultHTTPTimeout = 120 * time.Second

	AuthSourceEnv   = "env"
	AuthSourceLocal = "local"
)

type Client struct {
	BaseURL     string
	Token       string
	TokenSource string
	HTTP        *http.Client
}

type HTTPError struct {
	Method     string
	Path       string
	StatusCode int
	Body       string
}

func (e *HTTPError) Error() string {
	return fmt.Sprintf("%s %s: http %d: %s", e.Method, e.Path, e.StatusCode, strings.TrimSpace(e.Body))
}

type InvalidEnvTokenError struct {
	Method string
	Path   string
}

func (e *InvalidEnvTokenError) Error() string {
	return fmt.Sprintf("%s is set, but it is invalid, expired, or revoked. Create a new token with `dari-docs auth token create --name github-actions`, then update your CI secret store.", EnvTokenName)
}

func New(baseURL, token string) *Client {
	return NewWithAuthToken(baseURL, AuthToken{Token: token})
}

func NewWithAuthToken(baseURL string, auth AuthToken) *Client {
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}
	return &Client{
		BaseURL:     strings.TrimRight(baseURL, "/"),
		Token:       auth.Token,
		TokenSource: auth.Source,
		HTTP:        &http.Client{Timeout: defaultHTTPTimeout},
	}
}

type AuthToken struct {
	Token  string
	Source string
}

type BalanceResponse struct {
	Email        string `json:"email"`
	BalanceCents int64  `json:"balance_cents"`
}

type MeResponse struct {
	Email        string        `json:"email"`
	BalanceCents int64         `json:"balance_cents"`
	Token        AuthTokenInfo `json:"token"`
}

type AuthTokenInfo struct {
	ID          string     `json:"id"`
	Name        string     `json:"name,omitempty"`
	Kind        string     `json:"kind"`
	TokenPrefix string     `json:"token_prefix,omitempty"`
	Scopes      []string   `json:"scopes"`
	CreatedAt   *time.Time `json:"created_at,omitempty"`
	LastUsedAt  *time.Time `json:"last_used_at,omitempty"`
	ExpiresAt   *time.Time `json:"expires_at,omitempty"`
	RevokedAt   *time.Time `json:"revoked_at,omitempty"`
}

type TokenCreateRequest struct {
	Name      string     `json:"name"`
	Scopes    []string   `json:"scopes,omitempty"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
}

type TokenCreateResponse struct {
	AuthTokenInfo
	Token string `json:"token"`
}

type TokenListResponse struct {
	Tokens []AuthTokenInfo `json:"tokens"`
}

type DariExchangeResponse struct {
	Token       string `json:"token"`
	Email       string `json:"email"`
	DisplayName string `json:"display_name,omitempty"`
}

type CheckoutRequest struct {
	AmountCents int64 `json:"amount_cents"`
}

type CheckoutResponse struct {
	CheckoutSessionID string `json:"checkout_session_id"`
	CheckoutURL       string `json:"checkout_url"`
}

type CreateRunResponse struct {
	RunID  string `json:"run_id"`
	Status string `json:"status"`
}

type CreateRunOptions struct {
	LiveVerify         bool
	RuntimeSecretsJSON string
	FeedbackLLMIDs     []string
	EditorLLMID        string
}

type RunStatus struct {
	ID                   string              `json:"id"`
	Mode                 string              `json:"mode"`
	Status               string              `json:"status"`
	Error                string              `json:"error,omitempty"`
	Tasks                []string            `json:"tasks,omitempty"`
	TaskCount            int                 `json:"task_count"`
	CreatedAt            time.Time           `json:"created_at"`
	CompletedAt          *time.Time          `json:"completed_at,omitempty"`
	LLMs                 []RunLLMSummary     `json:"llms"`
	Sessions             []RunSessionSummary `json:"sessions"`
	FeedbackReports      []string            `json:"feedback_reports,omitempty"`
	AggregateFeedback    string              `json:"aggregate_feedback,omitempty"`
	UpdatedDocsAvailable bool                `json:"updated_docs_available"`
	ReservedCents        int64               `json:"reserved_cents"`
	ChargedCents         int64               `json:"charged_cents"`
	Estimated            bool                `json:"estimated"`
}

type RunLLMSummary struct {
	Kind  string `json:"kind"`
	LLMID string `json:"llm_id"`
	Count int    `json:"count"`
}

type RunSessionSummary struct {
	Kind        string     `json:"kind"`
	TaskIndex   int        `json:"task_index"`
	Status      string     `json:"status"`
	LLMID       string     `json:"llm_id"`
	CreatedAt   time.Time  `json:"created_at"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
}

type RunConfig struct {
	FreeCreditCents            int64    `json:"free_credit_cents"`
	TesterSessionReserveCents  int64    `json:"tester_session_reserve_cents"`
	EditorSessionReserveCents  int64    `json:"editor_session_reserve_cents"`
	ServiceFeeCents            int64    `json:"service_fee_cents"`
	MaxTasksPerRun             int      `json:"max_tasks_per_run"`
	MaxTaskBytes               int64    `json:"max_task_bytes"`
	MaxActiveRunsPerUser       int      `json:"max_active_runs_per_user"`
	MaxBundleBytes             int64    `json:"max_bundle_bytes"`
	BundleMaxUncompressedBytes int64    `json:"bundle_max_uncompressed_bytes"`
	BundleMaxFileBytes         int64    `json:"bundle_max_file_bytes"`
	DefaultLLMID               string   `json:"default_llm_id"`
	DefaultFeedbackLLMIDs      []string `json:"default_feedback_llm_ids"`
	AllowedLLMIDs              []string `json:"allowed_llm_ids"`
}

func (c *Client) ExchangeDariToken(ctx context.Context, accessToken string) (DariExchangeResponse, error) {
	var out DariExchangeResponse
	err := c.doWithBearer(ctx, http.MethodPost, "/v1/auth/dari/exchange", accessToken, "application/json", nil, &out)
	return out, err
}

func (c *Client) RunConfig(ctx context.Context) (RunConfig, error) {
	var out RunConfig
	err := c.doJSON(ctx, http.MethodGet, "/v1/runs/config", nil, &out)
	return out, err
}

func (c *Client) Me(ctx context.Context) (MeResponse, error) {
	var out MeResponse
	err := c.doJSON(ctx, http.MethodGet, "/v1/me", nil, &out)
	return out, err
}

func (c *Client) Balance(ctx context.Context) (BalanceResponse, error) {
	var out BalanceResponse
	err := c.doJSON(ctx, http.MethodGet, "/v1/billing/balance", nil, &out)
	return out, err
}

func (c *Client) CreateCheckout(ctx context.Context, amountCents int64) (CheckoutResponse, error) {
	var out CheckoutResponse
	err := c.doJSON(ctx, http.MethodPost, "/v1/billing/checkout", CheckoutRequest{AmountCents: amountCents}, &out)
	return out, err
}

func (c *Client) Logout(ctx context.Context) error {
	return c.doJSON(ctx, http.MethodPost, "/v1/auth/logout", nil, nil)
}

func (c *Client) LogoutAll(ctx context.Context) error {
	return c.LogoutAllKind(ctx, "")
}

func (c *Client) LogoutAllKind(ctx context.Context, kind string) error {
	path := "/v1/auth/logout-all"
	if kind != "" {
		path += "?kind=" + url.QueryEscape(kind)
	}
	return c.doJSON(ctx, http.MethodPost, path, nil, nil)
}

func (c *Client) CreateAuthToken(ctx context.Context, req TokenCreateRequest) (TokenCreateResponse, error) {
	var out TokenCreateResponse
	err := c.doJSON(ctx, http.MethodPost, "/v1/auth/tokens", req, &out)
	return out, err
}

func (c *Client) ListAuthTokens(ctx context.Context) (TokenListResponse, error) {
	var out TokenListResponse
	err := c.doJSON(ctx, http.MethodGet, "/v1/auth/tokens", nil, &out)
	return out, err
}

func (c *Client) RevokeAuthToken(ctx context.Context, id string) error {
	if strings.TrimSpace(id) == "" {
		return fmt.Errorf("token id is required")
	}
	return c.doJSON(ctx, http.MethodPost, "/v1/auth/tokens/"+url.PathEscape(strings.TrimSpace(id))+"/revoke", nil, nil)
}

func (c *Client) CreateRun(ctx context.Context, mode string, tasks []string, bundlePath string, opts CreateRunOptions) (CreateRunResponse, error) {
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	if err := mw.WriteField("mode", mode); err != nil {
		return CreateRunResponse{}, err
	}
	taskJSON, err := json.Marshal(tasks)
	if err != nil {
		return CreateRunResponse{}, err
	}
	if err := mw.WriteField("tasks_json", string(taskJSON)); err != nil {
		return CreateRunResponse{}, err
	}
	if opts.LiveVerify {
		if err := mw.WriteField("live_verify", "true"); err != nil {
			return CreateRunResponse{}, err
		}
	}
	if opts.RuntimeSecretsJSON != "" {
		if err := mw.WriteField("runtime_secrets_json", opts.RuntimeSecretsJSON); err != nil {
			return CreateRunResponse{}, err
		}
	}
	if len(opts.FeedbackLLMIDs) > 0 {
		llmJSON, err := json.Marshal(opts.FeedbackLLMIDs)
		if err != nil {
			return CreateRunResponse{}, err
		}
		if err := mw.WriteField("feedback_llm_ids_json", string(llmJSON)); err != nil {
			return CreateRunResponse{}, err
		}
	}
	if opts.EditorLLMID != "" {
		if err := mw.WriteField("editor_llm_id", opts.EditorLLMID); err != nil {
			return CreateRunResponse{}, err
		}
	}
	if err := addMultipartFile(mw, "bundle", bundlePath); err != nil {
		return CreateRunResponse{}, err
	}
	if err := mw.Close(); err != nil {
		return CreateRunResponse{}, err
	}
	var out CreateRunResponse
	err = c.do(ctx, http.MethodPost, "/v1/runs", mw.FormDataContentType(), &body, &out)
	return out, err
}

func addMultipartFile(mw *multipart.Writer, field, path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	part, err := mw.CreateFormFile(field, filepath.Base(path))
	if err != nil {
		return err
	}
	_, err = io.Copy(part, f)
	return err
}

func (c *Client) GetRun(ctx context.Context, id string) (RunStatus, error) {
	var out RunStatus
	err := c.doJSON(ctx, http.MethodGet, "/v1/runs/"+url.PathEscape(id), nil, &out)
	return out, err
}

func (c *Client) DownloadUpdatedDocs(ctx context.Context, id, outPath string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+"/v1/runs/"+url.PathEscape(id)+"/updated-docs.zip", nil)
	if err != nil {
		return err
	}
	c.authorize(req)
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		if resp.StatusCode == http.StatusUnauthorized && c.TokenSource == AuthSourceEnv {
			return &InvalidEnvTokenError{Method: http.MethodGet, Path: "/v1/runs/" + url.PathEscape(id) + "/updated-docs.zip"}
		}
		return fmt.Errorf("download updated docs: http %d: %s", resp.StatusCode, strings.TrimSpace(string(b)))
	}
	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		return err
	}
	f, err := os.Create(outPath)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(f, resp.Body)
	return err
}

func (c *Client) doJSON(ctx context.Context, method, path string, in any, out any) error {
	var body io.Reader
	if in != nil {
		b, err := json.Marshal(in)
		if err != nil {
			return err
		}
		body = bytes.NewReader(b)
	}
	return c.do(ctx, method, path, "application/json", body, out)
}

func (c *Client) do(ctx context.Context, method, path, contentType string, body io.Reader, out any) error {
	return c.doWithHTTP(ctx, c.HTTP, method, path, contentType, body, out)
}

func (c *Client) doWithBearer(ctx context.Context, method, path, bearer, contentType string, body io.Reader, out any) error {
	return c.doWithBearerHTTP(ctx, c.HTTP, method, path, bearer, contentType, body, out)
}

func (c *Client) doWithHTTP(ctx context.Context, client *http.Client, method, path, contentType string, body io.Reader, out any) error {
	return c.doWithBearerHTTP(ctx, client, method, path, c.Token, contentType, body, out)
}

func (c *Client) doWithBearerHTTP(ctx context.Context, client *http.Client, method, path, bearer, contentType string, body io.Reader, out any) error {
	req, err := http.NewRequestWithContext(ctx, method, c.BaseURL+path, body)
	if err != nil {
		return err
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		if resp.StatusCode == http.StatusUnauthorized && bearer == c.Token && c.TokenSource == AuthSourceEnv {
			return &InvalidEnvTokenError{Method: method, Path: path}
		}
		return &HTTPError{Method: method, Path: path, StatusCode: resp.StatusCode, Body: string(b)}
	}
	if out == nil || len(b) == 0 {
		return nil
	}
	if err := json.Unmarshal(b, out); err != nil {
		return fmt.Errorf("decode %s %s: %w; body=%s", method, path, err, string(b))
	}
	return nil
}

func (c *Client) authorize(req *http.Request) {
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}
}

type Credentials struct {
	Services map[string]ServiceCredentials `json:"services"`
}

type ServiceCredentials struct {
	Token string `json:"token"`
}

func LoadToken(baseURL string) (string, error) {
	creds, err := loadCredentials()
	if err != nil {
		return "", err
	}
	if creds.Services == nil {
		return "", nil
	}
	baseURL = strings.TrimRight(baseURL, "/")
	return creds.Services[baseURL].Token, nil
}

func LoadAuthToken(baseURL string) (AuthToken, error) {
	if rawToken, ok := os.LookupEnv(EnvTokenName); ok {
		token := strings.TrimSpace(rawToken)
		if token == "" {
			return AuthToken{}, fmt.Errorf("%s is set but empty; unset it or provide a valid token", EnvTokenName)
		}
		return AuthToken{Token: token, Source: AuthSourceEnv}, nil
	}
	token, err := LoadToken(baseURL)
	if err != nil {
		return AuthToken{}, err
	}
	if token == "" {
		return AuthToken{}, nil
	}
	return AuthToken{Token: token, Source: AuthSourceLocal}, nil
}

func SaveToken(baseURL, token string) error {
	creds, err := loadCredentials()
	if err != nil {
		return err
	}
	if creds.Services == nil {
		creds.Services = map[string]ServiceCredentials{}
	}
	baseURL = strings.TrimRight(baseURL, "/")
	creds.Services[baseURL] = ServiceCredentials{Token: token}
	p, err := credentialsPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(creds, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')
	return os.WriteFile(p, b, 0o600)
}

func DeleteToken(baseURL string) error {
	creds, err := loadCredentials()
	if err != nil {
		return err
	}
	if creds.Services != nil {
		delete(creds.Services, strings.TrimRight(baseURL, "/"))
	}
	p, err := credentialsPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(creds, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')
	return os.WriteFile(p, b, 0o600)
}

func loadCredentials() (Credentials, error) {
	p, err := credentialsPath()
	if err != nil {
		return Credentials{}, err
	}
	b, err := os.ReadFile(p)
	if os.IsNotExist(err) {
		return Credentials{}, nil
	}
	if err != nil {
		return Credentials{}, err
	}
	var creds Credentials
	if err := json.Unmarshal(b, &creds); err != nil {
		return Credentials{}, err
	}
	return creds, nil
}

func credentialsPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".dari-docs", "credentials.json"), nil
}
