// Package github reads the pull request a GitHub Actions job runs for, and keeps one dari-docs
// comment on it up to date.
package github

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"
)

// PullRequest is the pull request an Actions event names.
type PullRequest struct {
	Repo    string
	Number  int
	BaseSHA string
	HeadSHA string
	// Action is the event's action, such as "opened", "synchronize", or "closed".
	Action string
	// Fork is a pull request from another repository, whose jobs get no secrets.
	Fork bool
}

// EventPullRequest reads the pull request from GITHUB_EVENT_PATH. ok is false outside a
// pull_request event.
func EventPullRequest() (PullRequest, bool, error) {
	path := os.Getenv("GITHUB_EVENT_PATH")
	if path == "" {
		return PullRequest{}, false, nil
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return PullRequest{}, false, err
	}
	return ParseEvent(b)
}

// ParseEvent reads a pull_request event payload.
func ParseEvent(payload []byte) (PullRequest, bool, error) {
	var e struct {
		Action      string `json:"action"`
		PullRequest *struct {
			Number int `json:"number"`
			Base   struct {
				SHA  string `json:"sha"`
				Repo struct {
					FullName string `json:"full_name"`
				} `json:"repo"`
			} `json:"base"`
			Head struct {
				SHA  string `json:"sha"`
				Repo *struct {
					FullName string `json:"full_name"`
				} `json:"repo"`
			} `json:"head"`
		} `json:"pull_request"`
	}
	if err := json.Unmarshal(payload, &e); err != nil {
		return PullRequest{}, false, fmt.Errorf("reading the GitHub event: %w", err)
	}
	if e.PullRequest == nil {
		return PullRequest{}, false, nil
	}
	pr := e.PullRequest
	head := ""
	if pr.Head.Repo != nil {
		head = pr.Head.Repo.FullName
	}
	return PullRequest{
		Repo:    pr.Base.Repo.FullName,
		Number:  pr.Number,
		BaseSHA: pr.Base.SHA,
		HeadSHA: pr.Head.SHA,
		Action:  e.Action,
		Fork:    head != pr.Base.Repo.FullName,
	}, true, nil
}

// Client calls GitHub's REST API with a token.
type Client struct {
	APIURL string
	Token  string
	HTTP   *http.Client
}

// FromEnv is a client for the Actions job's GitHub.
func FromEnv(token string) *Client {
	api := os.Getenv("GITHUB_API_URL")
	if api == "" {
		api = "https://api.github.com"
	}
	return &Client{APIURL: api, Token: token}
}

func (c *Client) do(ctx context.Context, method, path string, body, out any) error {
	var reader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, strings.TrimRight(c.APIURL, "/")+path, reader)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	client := c.HTTP
	if client == nil {
		client = &http.Client{Timeout: time.Minute}
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if resp.StatusCode >= 300 {
		return fmt.Errorf("GitHub %s %s: %d %s", method, path, resp.StatusCode, strings.TrimSpace(string(data)))
	}
	if out != nil {
		return json.Unmarshal(data, out)
	}
	return nil
}

// Marker opens every dari-docs comment, so a later run finds and replaces it.
const Marker = "<!-- dari-docs -->"

// State is what one run leaves in its comment for the next: the comparison it started.
type State struct {
	Comparison string `json:"comparison,omitempty"`
}

var stateLine = regexp.MustCompile(`<!-- dari-docs:state (\{.*?\}) -->`)

// Comment is the dari-docs comment on a pull request.
type Comment struct {
	ID    int64
	Body  string
	State State
}

// FindComment returns the pull request's dari-docs comment, if it has one.
func (c *Client) FindComment(ctx context.Context, repo string, pr int) (*Comment, error) {
	for page := 1; page <= 50; page++ {
		var comments []struct {
			ID   int64  `json:"id"`
			Body string `json:"body"`
			User struct {
				Type string `json:"type"`
			} `json:"user"`
		}
		path := fmt.Sprintf("/repos/%s/issues/%d/comments?per_page=100&page=%d", repo, pr, page)
		if err := c.do(ctx, http.MethodGet, path, nil, &comments); err != nil {
			return nil, err
		}
		for _, cm := range comments {
			// Only a bot's comment: anyone can post the marker, and its state names a comparison to stop.
			if cm.User.Type == "Bot" && strings.HasPrefix(cm.Body, Marker) {
				found := &Comment{ID: cm.ID, Body: cm.Body}
				if m := stateLine.FindStringSubmatch(cm.Body); m != nil {
					_ = json.Unmarshal([]byte(m[1]), &found.State)
				}
				return found, nil
			}
		}
		if len(comments) < 100 {
			return nil, nil
		}
	}
	return nil, nil
}

// Body is a comment's full text: the marker, the markdown, and the state for the next run.
func Body(markdown string, state State) string {
	s, _ := json.Marshal(state)
	return fmt.Sprintf("%s\n%s\n<!-- dari-docs:state %s -->\n", Marker, strings.TrimRight(markdown, "\n"), s)
}

// UpsertComment replaces the pull request's dari-docs comment, or posts one.
func (c *Client) UpsertComment(ctx context.Context, repo string, pr int, existing *Comment, body string) (*Comment, error) {
	var out struct {
		ID int64 `json:"id"`
	}
	if existing != nil {
		if err := c.do(ctx, http.MethodPatch, fmt.Sprintf("/repos/%s/issues/comments/%d", repo, existing.ID), map[string]string{"body": body}, &out); err != nil {
			return nil, err
		}
	} else if err := c.do(ctx, http.MethodPost, fmt.Sprintf("/repos/%s/issues/%d/comments", repo, pr), map[string]string{"body": body}, &out); err != nil {
		return nil, err
	}
	return &Comment{ID: out.ID, Body: body}, nil
}

// StepSummary appends markdown to the job's summary, when the job has one.
func StepSummary(markdown string) error {
	path := os.Getenv("GITHUB_STEP_SUMMARY")
	if path == "" {
		return nil
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.WriteString(markdown + "\n")
	return err
}
