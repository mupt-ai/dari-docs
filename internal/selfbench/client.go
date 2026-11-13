// Package selfbench is a small client for the SelfBench API routes a docs check uses: repo
// ablations (environments), comparisons, evaluations, and vibe evals (scenarios).
package selfbench

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Client calls one SelfBench deployment as one API key's owner.
type Client struct {
	BaseURL string
	APIKey  string
	Org     string
	HTTP    *http.Client
}

// Error is a response SelfBench refused, with its status and message.
type Error struct {
	Status  int
	Message string
}

func (e *Error) Error() string { return fmt.Sprintf("SelfBench %d: %s", e.Status, e.Message) }

// IsStatus reports whether err is a SelfBench response with this status.
func IsStatus(err error, status int) bool {
	var e *Error
	return errors.As(err, &e) && e.Status == status
}

func (c *Client) http() *http.Client {
	if c.HTTP != nil {
		return c.HTTP
	}
	return &http.Client{Timeout: 2 * time.Minute}
}

// Repo is the route prefix of a connected repository in the client's workspace.
func (c *Client) Repo(fullName string) string {
	owner, name, _ := strings.Cut(fullName, "/")
	return fmt.Sprintf("/api/orgs/%s/repos/%s/%s", url.PathEscape(c.Org), url.PathEscape(owner), url.PathEscape(name))
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
	req, err := http.NewRequestWithContext(ctx, method, strings.TrimRight(c.BaseURL, "/")+path, reader)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.APIKey)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http().Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 64<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode >= 300 {
		var e struct {
			Error string `json:"error"`
		}
		msg := strings.TrimSpace(string(data))
		if json.Unmarshal(data, &e) == nil && e.Error != "" {
			msg = e.Error
		}
		if len(msg) > 500 {
			msg = msg[:500]
		}
		return &Error{Status: resp.StatusCode, Message: msg}
	}
	if out == nil || len(data) == 0 {
		return nil
	}
	if err := json.Unmarshal(data, out); err != nil {
		return fmt.Errorf("SelfBench %s %s: unexpected reply: %w", method, path, err)
	}
	return nil
}

// TaskRef names a task by its batch and ID.
type TaskRef struct {
	RunID  string `json:"runId"`
	TaskID string `json:"taskId"`
}

func (t TaskRef) String() string { return t.RunID + "/" + t.TaskID }

// Coverage is how many of an environment's tasks are built, still building, or failed.
type Coverage struct {
	Ready    int `json:"ready"`
	Checking int `json:"checking"`
	Failed   int `json:"failed"`
	Total    int `json:"total"`
}

// Environment is a repo ablation.
type Environment struct {
	ID     int64    `json:"id"`
	Name   string   `json:"name"`
	Script string   `json:"script,omitempty"`
	Tasks  Coverage `json:"tasks"`
}

// EnvironmentTask is one task's build in an environment.
type EnvironmentTask struct {
	Task          TaskRef `json:"task"`
	State         string  `json:"state"`
	ReasonSummary string  `json:"reasonSummary,omitempty"`
}

// Environments lists the repository's repo ablations.
func (c *Client) Environments(ctx context.Context, repo string) ([]Environment, error) {
	var out struct {
		Environments []Environment `json:"environments"`
	}
	err := c.do(ctx, http.MethodGet, c.Repo(repo)+"/environments", nil, &out)
	return out.Environments, err
}

// CreateEnvironment saves a repo ablation and starts building every approved task in it.
func (c *Client) CreateEnvironment(ctx context.Context, repo, name, script string, generation json.RawMessage) (Environment, error) {
	body := map[string]any{"environment": map[string]string{"name": name, "script": script}}
	if len(generation) > 0 {
		body["generation"] = generation
	}
	var out struct {
		Environment Environment `json:"environment"`
		Failures    []string    `json:"failures"`
	}
	if err := c.do(ctx, http.MethodPost, c.Repo(repo)+"/environments", body, &out); err != nil {
		return Environment{}, err
	}
	return out.Environment, nil
}

// Environment reads one repo ablation with each task's build.
func (c *Client) Environment(ctx context.Context, repo string, id int64) (Environment, []EnvironmentTask, error) {
	var out struct {
		Environment Environment       `json:"environment"`
		Tasks       []EnvironmentTask `json:"tasks"`
	}
	err := c.do(ctx, http.MethodGet, fmt.Sprintf("%s/environments/%d", c.Repo(repo), id), nil, &out)
	return out.Environment, out.Tasks, err
}

// DeleteEnvironment removes a repo ablation and its builds.
func (c *Client) DeleteEnvironment(ctx context.Context, repo string, id int64) error {
	return c.do(ctx, http.MethodDelete, fmt.Sprintf("%s/environments/%d", c.Repo(repo), id), nil, nil)
}

// ComparisonModel is one model a comparison runs.
type ComparisonModel struct {
	CatalogID    string   `json:"catalogId"`
	CredentialID string   `json:"credentialId"`
	Harnesses    []string `json:"harnesses"`
	Thinking     string   `json:"thinking,omitempty"`
}

// Condition is a written-out run condition. A docs check names only an environment.
type Condition struct {
	Name        string `json:"name"`
	Environment int64  `json:"environment"`
}

// ComparisonRequest starts a comparison.
type ComparisonRequest struct {
	ID                  string            `json:"id"`
	Models              []ComparisonModel `json:"models"`
	Sandbox             string            `json:"sandbox"`
	SandboxCredentialID string            `json:"sandboxCredentialId"`
	Tasks               []TaskRef         `json:"tasks,omitempty"`
	JudgeCredentialID   string            `json:"judgeCredentialId,omitempty"`
	Conditions          []Condition       `json:"conditions,omitempty"`
	Baseline            *bool             `json:"baseline,omitempty"`
}

// ComparisonRun is one evaluation a comparison started: one model under one condition.
type ComparisonRun struct {
	ID        string   `json:"id"`
	Model     string   `json:"model"`
	Condition string   `json:"condition,omitempty"`
	Harnesses []string `json:"harnesses"`
	Status    string   `json:"status"`
	Completed int      `json:"completed"`
	Trials    int      `json:"trials"`
}

// Comparison is a comparison's progress.
type Comparison struct {
	ID              string          `json:"id"`
	Runs            []ComparisonRun `json:"runs"`
	SubmissionError string          `json:"submissionError,omitempty"`
}

// Done reports whether every evaluation has finished.
func (c Comparison) Done() bool {
	if len(c.Runs) == 0 {
		return false
	}
	for _, r := range c.Runs {
		if r.Status != "completed" && r.Status != "failed" {
			return false
		}
	}
	return true
}

// StartComparison starts one, or returns it when a comparison with its ID already exists.
func (c *Client) StartComparison(ctx context.Context, repo string, req ComparisonRequest) (Comparison, error) {
	var out Comparison
	err := c.do(ctx, http.MethodPost, c.Repo(repo)+"/evaluations/comparisons", req, &out)
	if err != nil {
		// A rerun sends the same ID; SelfBench never starts it twice.
		if existing, getErr := c.Comparison(ctx, repo, req.ID); getErr == nil {
			return existing, nil
		}
		return Comparison{}, err
	}
	return out, nil
}

// Comparison reads a comparison's progress.
func (c *Client) Comparison(ctx context.Context, repo, id string) (Comparison, error) {
	var out Comparison
	err := c.do(ctx, http.MethodGet, c.Repo(repo)+"/evaluations/comparisons/"+url.PathEscape(id), nil, &out)
	return out, err
}

// ResumeComparison submits a comparison's evaluations SelfBench did not confirm.
func (c *Client) ResumeComparison(ctx context.Context, repo, id string) (Comparison, error) {
	var out Comparison
	err := c.do(ctx, http.MethodPost, c.Repo(repo)+"/evaluations/comparisons/"+url.PathEscape(id)+"/resume", nil, &out)
	return out, err
}

// CancelComparison stops a comparison's evaluations.
func (c *Client) CancelComparison(ctx context.Context, repo, id string) error {
	return c.do(ctx, http.MethodPost, c.Repo(repo)+"/evaluations/comparisons/"+url.PathEscape(id)+"/cancel", nil, nil)
}

// FailureSummary is SelfBench's explanation of a failed trial.
type FailureSummary struct {
	Text string `json:"text"`
}

// Trial is one task run on one harness.
type Trial struct {
	RunID          string             `json:"runId"`
	TaskID         string             `json:"taskId"`
	Harness        string             `json:"harness"`
	Status         string             `json:"status"`
	Rewards        map[string]float64 `json:"rewards"`
	Error          string             `json:"error,omitempty"`
	FailureSummary *FailureSummary    `json:"failureSummary,omitempty"`
	APICostUSD     *float64           `json:"apiCostUsd,omitempty"`
	AgentTimedOut  bool               `json:"agentTimedOut,omitempty"`
	CyberBlocked   bool               `json:"cyberBlocked,omitempty"`
}

// Passed is a scored trial whose tests passed.
func (t Trial) Passed() bool { return t.Rewards["reward"] == 1 }

// RunCondition is the condition an evaluation ran under.
type RunCondition struct {
	Name        string `json:"name"`
	Environment *struct {
		ID   int64  `json:"id"`
		Name string `json:"name"`
	} `json:"environment,omitempty"`
}

// Evaluation is one model's run of a comparison's tasks, with every trial.
type Evaluation struct {
	ID         string        `json:"id"`
	Model      string        `json:"model"`
	ModelLabel string        `json:"modelLabel"`
	Status     string        `json:"status"`
	Condition  *RunCondition `json:"condition,omitempty"`
	Trials     []Trial       `json:"trials"`
	Error      string        `json:"error,omitempty"`
}

// Evaluation reads one evaluation.
func (c *Client) Evaluation(ctx context.Context, repo, id string) (Evaluation, error) {
	var out Evaluation
	err := c.do(ctx, http.MethodGet, c.Repo(repo)+"/evaluations/"+url.PathEscape(id), nil, &out)
	return out, err
}

// Task is a task as the dataset lists it.
type Task struct {
	RunID         string `json:"runId"`
	TaskID        string `json:"taskId"`
	Title         string `json:"title,omitempty"`
	State         string `json:"state"`
	SourcePR      *int   `json:"sourcePr,omitempty"`
	ReasonSummary string `json:"reasonSummary,omitempty"`
}

// Tasks lists the repository's tasks.
func (c *Client) Tasks(ctx context.Context, repo string) ([]Task, error) {
	var out struct {
		Tasks []Task `json:"tasks"`
	}
	err := c.do(ctx, http.MethodGet, c.Repo(repo)+"/tasks", nil, &out)
	return out.Tasks, err
}

// ScenarioRequest describes a repository vibe eval: something a reader of the docs should be able to do.
type ScenarioRequest struct {
	Title      string          `json:"title"`
	Scenario   string          `json:"scenario"`
	Commit     string          `json:"commit"`
	Difficulty string          `json:"difficulty,omitempty"`
	Generation json.RawMessage `json:"generation,omitempty"`
}

// CreateScenario starts authoring a task from a scenario. It needs review before checks run it.
func (c *Client) CreateScenario(ctx context.Context, repo string, req ScenarioRequest) (Task, error) {
	var out struct {
		Task Task `json:"task"`
	}
	err := c.do(ctx, http.MethodPost, c.Repo(repo)+"/tasks/from-scenario", req, &out)
	return out.Task, err
}

// WebURL is the app's page for a path under a repository, such as `comparisons/ID`.
func (c *Client) WebURL(repo, page string) string {
	return fmt.Sprintf("%s/repos/%s/%s", strings.TrimRight(c.BaseURL, "/"), repo, page)
}
