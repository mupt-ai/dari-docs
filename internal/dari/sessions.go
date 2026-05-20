package dari

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

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

type ContentBlock map[string]any

func TextBlock(text string) ContentBlock   { return ContentBlock{"type": "text", "text": text} }
func FileBlock(fileID string) ContentBlock { return ContentBlock{"type": "file", "file_id": fileID} }

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

func (c *Client) GetTranscriptRaw(ctx context.Context, sessionID string) (json.RawMessage, error) {
	var out json.RawMessage
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
