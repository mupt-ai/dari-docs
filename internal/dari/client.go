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
	ID                string  `json:"id"`
	AgentID           string  `json:"agent_id"`
	VersionID         string  `json:"version_id"`
	Status            string  `json:"status"`
	LastMessageID     *string `json:"last_message_id"`
	LastMessageStatus *string `json:"last_message_status"`
}

type MessageSummary struct {
	ID     string `json:"id"`
	Status string `json:"status"`
}

type SendEventResponse struct {
	Message MessageSummary `json:"message"`
	Session Session        `json:"session"`
}

func (c *Client) UploadFile(ctx context.Context, path string) (UploadedFile, error) {
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	f, err := os.Open(path)
	if err != nil {
		return UploadedFile{}, err
	}
	defer f.Close()
	part, err := mw.CreateFormFile("file", filepath.Base(path))
	if err != nil {
		return UploadedFile{}, err
	}
	if _, err := io.Copy(part, f); err != nil {
		return UploadedFile{}, err
	}
	if err := mw.Close(); err != nil {
		return UploadedFile{}, err
	}
	var out UploadedFile
	err = c.doJSON(ctx, http.MethodPost, "/v1/files", mw.FormDataContentType(), &body, &out)
	return out, err
}

type CreateSessionRequest struct {
	Secrets   map[string]string `json:"secrets,omitempty"`
	LLMAPIKey string            `json:"llm_api_key,omitempty"`
}

func (c *Client) CreateSession(ctx context.Context, agentID string, req CreateSessionRequest) (Session, error) {
	var out Session
	b, _ := json.Marshal(req)
	err := c.doJSON(ctx, http.MethodPost, "/v1/agents/"+url.PathEscape(agentID)+"/sessions", "application/json", bytes.NewReader(b), &out)
	return out, err
}

type ContentBlock map[string]any

func TextBlock(text string) ContentBlock   { return ContentBlock{"type": "text", "text": text} }
func FileBlock(fileID string) ContentBlock { return ContentBlock{"type": "file", "file_id": fileID} }

func (c *Client) SendUserMessage(ctx context.Context, sessionID string, content []ContentBlock) (SendEventResponse, error) {
	payload := map[string]any{"type": "user.message", "content": content}
	b, _ := json.Marshal(payload)
	var out SendEventResponse
	err := c.doJSON(ctx, http.MethodPost, "/v1/sessions/"+url.PathEscape(sessionID)+"/events", "application/json", bytes.NewReader(b), &out)
	return out, err
}

func (c *Client) GetSession(ctx context.Context, sessionID string) (Session, error) {
	var out Session
	err := c.doJSON(ctx, http.MethodGet, "/v1/sessions/"+url.PathEscape(sessionID), "", nil, &out)
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

func (c *Client) WaitForCompletion(ctx context.Context, sessionID string, interval time.Duration, timeout time.Duration) (Session, error) {
	deadline := time.Now().Add(timeout)
	for {
		s, err := c.GetSession(ctx, sessionID)
		if err != nil {
			return s, err
		}
		status := ""
		if s.LastMessageStatus != nil {
			status = *s.LastMessageStatus
		}
		if status == "completed" || status == "failed" {
			return s, nil
		}
		if time.Now().After(deadline) {
			return s, fmt.Errorf("timeout waiting for session %s last_message_status=%q", sessionID, status)
		}
		select {
		case <-ctx.Done():
			return s, ctx.Err()
		case <-time.After(interval):
		}
	}
}

func (c *Client) DownloadWorkspaceZip(ctx context.Context, sessionID string, paths []string, outPath string) error {
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
	_, err = io.Copy(f, resp.Body)
	return err
}

func ExtractZip(zipPath, dest string) error {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return err
	}
	defer r.Close()
	absDest, err := filepath.Abs(dest)
	if err != nil {
		return err
	}
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
		_, copyErr := io.Copy(w, rc)
		closeErr := w.Close()
		rc.Close()
		if copyErr != nil {
			return copyErr
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
		return fmt.Errorf("%s %s: http %d: %s", method, path, resp.StatusCode, strings.TrimSpace(string(b)))
	}
	if out == nil || len(b) == 0 {
		return nil
	}
	if err := json.Unmarshal(b, out); err != nil {
		return fmt.Errorf("decode %s %s: %w; body=%s", method, path, err, string(b))
	}
	return nil
}
