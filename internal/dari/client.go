package dari

import (
	"archive/zip"
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

type Client struct {
	BaseURL string
	APIKey  string
	HTTP    *http.Client
}

const DefaultWorkspaceZipMaxUncompressedBytes int64 = 100 * 1024 * 1024

type HTTPError struct {
	Method     string
	Path       string
	StatusCode int
	Body       string
}

func (e *HTTPError) Error() string {
	return fmt.Sprintf("%s %s: http %d: %s", e.Method, e.Path, e.StatusCode, e.Body)
}

func New(baseURL, apiKey string) *Client {
	if baseURL == "" {
		baseURL = "https://api.dari.dev"
	}
	baseURL = strings.TrimRight(baseURL, "/")
	return &Client{BaseURL: baseURL, APIKey: apiKey, HTTP: &http.Client{Timeout: 120 * time.Second}}
}

type UploadedFile struct {
	ID        string `json:"id"`
	Filename  string `json:"filename"`
	SizeBytes int64  `json:"size_bytes"`
}

type Session struct {
	ID                string            `json:"id"`
	AgentID           string            `json:"agent_id"`
	VersionID         string            `json:"version_id"`
	LLMID             *string           `json:"llm_id"`
	Status            string            `json:"status"`
	LastMessageID     *string           `json:"last_message_id"`
	LastMessageStatus *string           `json:"last_message_status"`
	Metadata          map[string]string `json:"metadata,omitempty"`
}

type AgentVersionDetail struct {
	Agent struct {
		ID              string  `json:"id"`
		ActiveVersionID *string `json:"active_version_id"`
	} `json:"agent"`
	Version struct {
		ID      string `json:"id"`
		AgentID string `json:"agent_id"`
	} `json:"version"`
}

type CostSummary struct {
	ScopeKind    string `json:"scope_kind"`
	ScopeID      string `json:"scope_id"`
	EventCount   int    `json:"event_count"`
	TotalCostUSD string `json:"total_cost_usd"`
}

func (c *Client) UploadFile(ctx context.Context, path string) (UploadedFile, error) {
	f, err := os.Open(path)
	if err != nil {
		return UploadedFile{}, err
	}
	defer f.Close()
	return c.UploadReader(ctx, filepath.Base(path), f)
}

func (c *Client) UploadReader(ctx context.Context, filename string, r io.Reader) (UploadedFile, error) {
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	part, err := mw.CreateFormFile("file", filepath.Base(filename))
	if err != nil {
		return UploadedFile{}, err
	}
	if _, err := io.Copy(part, r); err != nil {
		return UploadedFile{}, err
	}
	if err := mw.Close(); err != nil {
		return UploadedFile{}, err
	}
	var out UploadedFile
	err = c.doJSON(ctx, http.MethodPost, "/v1/files", mw.FormDataContentType(), &body, &out)
	return out, err
}

type CreateSessionBatchRequest struct {
	IdempotencyKey string                   `json:"idempotency_key,omitempty"`
	Items          []CreateSessionBatchItem `json:"items"`
}

type CreateSessionBatchItem struct {
	AgentID   string                    `json:"agent_id"`
	VersionID string                    `json:"version_id,omitempty"`
	LLMID     string                    `json:"llm_id,omitempty"`
	Metadata  map[string]string         `json:"metadata,omitempty"`
	Secrets   map[string]string         `json:"secrets,omitempty"`
	Message   CreateSessionBatchMessage `json:"message"`
}

type CreateSessionBatchMessage struct {
	Content  []ContentBlock `json:"content"`
	Metadata map[string]any `json:"metadata,omitempty"`
}

type SessionBatch struct {
	BatchID  string                `json:"batch_id"`
	Status   string                `json:"status"`
	Counts   SessionBatchCounts    `json:"counts,omitempty"`
	Sessions []SessionBatchSession `json:"sessions"`
}

type SessionBatchCounts struct {
	Queued    int `json:"queued"`
	Running   int `json:"running"`
	Completed int `json:"completed"`
	Failed    int `json:"failed"`
}

type SessionBatchSession struct {
	Index             int               `json:"index"`
	SessionID         string            `json:"session_id"`
	Status            string            `json:"status"`
	LastMessageStatus string            `json:"last_message_status"`
	AgentID           string            `json:"agent_id"`
	VersionID         string            `json:"version_id"`
	LLMID             string            `json:"llm_id"`
	Metadata          map[string]string `json:"metadata"`
	TranscriptURL     string            `json:"transcript_url"`
	Error             string            `json:"error"`
}

func (c *Client) CreateSessionBatch(ctx context.Context, req CreateSessionBatchRequest) (SessionBatch, error) {
	var out SessionBatch
	b, err := json.Marshal(req)
	if err != nil {
		return out, fmt.Errorf("encode session batch request: %w", err)
	}
	if err := c.doJSON(ctx, http.MethodPost, "/v1/session-batches", "application/json", bytes.NewReader(b), &out); err != nil {
		return out, err
	}
	if out.Sessions == nil {
		out.Sessions = []SessionBatchSession{}
	}
	return out, nil
}

func (c *Client) GetSessionBatch(ctx context.Context, batchID string) (SessionBatch, error) {
	var out SessionBatch
	if err := c.doJSON(ctx, http.MethodGet, "/v1/session-batches/"+url.PathEscape(batchID), "", nil, &out); err != nil {
		return out, err
	}
	if out.Sessions == nil {
		out.Sessions = []SessionBatchSession{}
	}
	return out, nil
}

type ContentBlock map[string]any

func TextBlock(text string) ContentBlock   { return ContentBlock{"type": "text", "text": text} }
func FileBlock(fileID string) ContentBlock { return ContentBlock{"type": "file", "file_id": fileID} }

func (c *Client) GetSession(ctx context.Context, sessionID string) (Session, error) {
	var out Session
	err := c.doJSON(ctx, http.MethodGet, "/v1/sessions/"+url.PathEscape(sessionID), "", nil, &out)
	return out, err
}

func (c *Client) GetAgentVersion(ctx context.Context, agentID string, versionID string) (AgentVersionDetail, error) {
	var out AgentVersionDetail
	err := c.doJSON(ctx, http.MethodGet, "/v1/agents/"+url.PathEscape(agentID)+"/versions/"+url.PathEscape(versionID), "", nil, &out)
	return out, err
}

type Transcript struct {
	Timeline struct {
		Items []struct {
			Type    string `json:"type"`
			Status  string `json:"status"`
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
			ErrorMessage *string `json:"error_message"`
		} `json:"items"`
	} `json:"timeline"`
}

func (c *Client) GetTranscript(ctx context.Context, sessionID string) (Transcript, error) {
	var out Transcript
	err := c.doJSON(ctx, http.MethodGet, "/v1/sessions/"+url.PathEscape(sessionID)+"/transcript", "", nil, &out)
	return out, err
}

func (c *Client) GetSessionCost(ctx context.Context, sessionID string) (CostSummary, error) {
	var out CostSummary
	err := c.doJSON(ctx, http.MethodGet, "/v1/costs/sessions/"+url.PathEscape(sessionID), "", nil, &out)
	return out, err
}

func FinalAssistantText(t Transcript) string {
	var parts []string
	for _, item := range t.Timeline.Items {
		if item.Type != "assistant_message" {
			continue
		}
		for _, c := range item.Content {
			if c.Type == "text" && strings.TrimSpace(c.Text) != "" {
				parts = append(parts, c.Text)
			}
		}
		if item.ErrorMessage != nil && *item.ErrorMessage != "" {
			parts = append(parts, "ERROR: "+*item.ErrorMessage)
		}
	}
	return strings.TrimSpace(strings.Join(parts, "\n\n"))
}

func (c *Client) WaitForBatchCompletion(
	ctx context.Context,
	batchID string,
	interval time.Duration,
	timeout time.Duration,
) (SessionBatch, error) {
	if interval <= 0 {
		interval = time.Second
	}
	deadline := time.Now().Add(timeout)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		b, err := c.GetSessionBatch(ctx, batchID)
		if err != nil {
			return b, fmt.Errorf("get session batch: %w", err)
		}
		switch b.Status {
		case "completed", "failed", "partial_failed":
			return b, nil
		}
		if time.Now().After(deadline) {
			return b, fmt.Errorf("timeout waiting for session batch %s status=%q", batchID, b.Status)
		}
		select {
		case <-ctx.Done():
			return b, ctx.Err()
		case <-ticker.C:
		}
	}
}

func (c *Client) DownloadWorkspaceZip(ctx context.Context, sessionID string, paths []string, outPath string) error {
	return c.DownloadWorkspaceZipWithLimit(ctx, sessionID, paths, outPath, 0)
}

func (c *Client) DownloadWorkspaceZipWithLimit(
	ctx context.Context,
	sessionID string,
	paths []string,
	outPath string,
	maxBytes int64,
) error {
	u := "/v1/sessions/" + url.PathEscape(sessionID) + "/workspace.zip"
	if len(paths) > 0 {
		q := url.Values{}
		for _, p := range paths {
			q.Add("path", p)
		}
		u += "?" + q.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+u, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.APIKey)
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("download workspace: http %d: %s", resp.StatusCode, strings.TrimSpace(string(b)))
	}
	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		return err
	}
	f, err := os.Create(outPath)
	if err != nil {
		return err
	}
	defer f.Close()
	reader := io.Reader(resp.Body)
	if maxBytes > 0 {
		reader = io.LimitReader(resp.Body, maxBytes+1)
	}
	n, err := io.Copy(f, reader)
	if err == nil && maxBytes > 0 && n > maxBytes {
		_ = f.Close()
		_ = os.Remove(outPath)
		return fmt.Errorf("download workspace exceeds size limit of %d bytes", maxBytes)
	}
	return err
}

func (c *Client) WriteWorkspaceZipWithLimit(ctx context.Context, sessionID string, paths []string, w io.Writer, maxBytes int64) error {
	u := "/v1/sessions/" + url.PathEscape(sessionID) + "/workspace.zip"
	if len(paths) > 0 {
		q := url.Values{}
		for _, p := range paths {
			q.Add("path", p)
		}
		u += "?" + q.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+u, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.APIKey)
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("download workspace: http %d: %s", resp.StatusCode, strings.TrimSpace(string(b)))
	}
	reader := io.Reader(resp.Body)
	if maxBytes > 0 {
		reader = io.LimitReader(resp.Body, maxBytes+1)
	}
	n, err := io.Copy(w, reader)
	if err == nil && maxBytes > 0 && n > maxBytes {
		return fmt.Errorf("download workspace exceeds size limit of %d bytes", maxBytes)
	}
	return err
}

func ExtractZip(zipPath, dest string) error {
	return ExtractZipWithLimit(zipPath, dest, DefaultWorkspaceZipMaxUncompressedBytes)
}

func ExtractZipWithLimit(zipPath, dest string, maxUncompressedBytes int64) error {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return err
	}
	defer r.Close()
	absDest, err := filepath.Abs(dest)
	if err != nil {
		return err
	}
	var total int64
	for _, f := range r.File {
		clean := filepath.Clean(f.Name)
		if clean == "." || strings.HasPrefix(clean, "../") || filepath.IsAbs(clean) {
			return fmt.Errorf("unsafe zip path %q", f.Name)
		}
		outPath := filepath.Join(absDest, clean)
		if !strings.HasPrefix(outPath, absDest+string(os.PathSeparator)) && outPath != absDest {
			return fmt.Errorf("zip path escapes dest: %q", f.Name)
		}
		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(outPath, 0o755); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
			return err
		}
		rc, err := f.Open()
		if err != nil {
			return err
		}
		w, err := os.OpenFile(outPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, f.FileInfo().Mode())
		if err != nil {
			rc.Close()
			return err
		}
		reader := io.Reader(rc)
		if maxUncompressedBytes > 0 {
			remaining := maxUncompressedBytes - total
			if remaining < 0 {
				remaining = 0
			}
			reader = io.LimitReader(rc, remaining+1)
		}
		n, copyErr := io.Copy(w, reader)
		total += n
		closeErr := w.Close()
		rc.Close()
		if copyErr != nil {
			return copyErr
		}
		if maxUncompressedBytes > 0 && total > maxUncompressedBytes {
			_ = os.Remove(outPath)
			return fmt.Errorf("zip exceeds uncompressed size limit of %d bytes", maxUncompressedBytes)
		}
		if closeErr != nil {
			return closeErr
		}
	}
	return nil
}

func (c *Client) doJSON(ctx context.Context, method, path, contentType string, body io.Reader, out any) error {
	req, err := http.NewRequestWithContext(ctx, method, c.BaseURL+path, body)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.APIKey)
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return &HTTPError{Method: method, Path: path, StatusCode: resp.StatusCode, Body: strings.TrimSpace(string(b))}
	}
	if out == nil || len(b) == 0 {
		return nil
	}
	if err := json.Unmarshal(b, out); err != nil {
		return fmt.Errorf("decode %s %s: %w; body=%s", method, path, err, string(b))
	}
	return nil
}
