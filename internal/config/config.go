// Package config reads .dari-docs/config.json, the per-repository settings for checking docs
// changes on SelfBench.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// DefaultDocs are the pathspecs treated as documentation when the config names none.
var DefaultDocs = []string{
	"docs",
	"README.md",
	":(glob)**/*.md",
	":(glob)**/*.mdx",
}

const (
	DefaultAPIURL            = "https://app.selfbench.dev"
	DefaultHarness           = "claude-code"
	DefaultCredential        = "managed-model"
	DefaultSandbox           = "managed"
	DefaultSandboxCredential = "managed-sandbox"
	DefaultTimeoutMinutes    = 240
)

// Model is one model a docs check runs, as a SelfBench comparison names it.
type Model struct {
	CatalogID    string   `json:"catalogId"`
	CredentialID string   `json:"credentialId,omitempty"`
	Harnesses    []string `json:"harnesses,omitempty"`
	Thinking     string   `json:"thinking,omitempty"`
}

// Config is the contents of .dari-docs/config.json.
type Config struct {
	// APIURL is the SelfBench deployment: hosted by default, or a self-hosted one.
	APIURL string `json:"apiUrl,omitempty"`
	// Org is the SelfBench workspace. SELFBENCH_ORG overrides it.
	Org string `json:"org,omitempty"`
	// Repo is the connected repository, owner/name. Defaults to GITHUB_REPOSITORY or the origin remote.
	Repo string `json:"repo,omitempty"`
	// Docs are git pathspecs for the files a docs check compares.
	Docs []string `json:"docs,omitempty"`
	// Models run on every task, before and after the docs change.
	Models []Model `json:"models"`
	// Tasks limits the check to these approved tasks, as runId/taskId. Empty means every approved task.
	Tasks               []string `json:"tasks,omitempty"`
	Sandbox             string   `json:"sandbox,omitempty"`
	SandboxCredentialID string   `json:"sandboxCredentialId,omitempty"`
	// JudgeCredentialID is required when any task is graded by an LLM judge.
	JudgeCredentialID string `json:"judgeCredentialId,omitempty"`
	// Generation is passed through to SelfBench when building the docs ablations. Empty uses the
	// deployment's managed models and sandbox.
	Generation json.RawMessage `json:"generation,omitempty"`
	// FailOnRegression makes the check fail when a task passes before the change and fails after.
	FailOnRegression bool `json:"failOnRegression,omitempty"`
	// TimeoutMinutes bounds how long a check waits for builds and trials.
	TimeoutMinutes int `json:"timeoutMinutes,omitempty"`
}

// Path is where a repository keeps its config.
func Path(repoRoot string) string { return filepath.Join(repoRoot, ".dari-docs", "config.json") }

// Load reads and validates the config at path, filling defaults.
func Load(path string) (Config, error) {
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return Config{}, fmt.Errorf("%s not found: run `dari-docs init` first", path)
	}
	if err != nil {
		return Config{}, err
	}
	var c Config
	dec := json.NewDecoder(strings.NewReader(string(b)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&c); err != nil {
		return Config{}, fmt.Errorf("%s: %w", path, err)
	}
	c.applyDefaults()
	if err := c.Validate(); err != nil {
		return Config{}, fmt.Errorf("%s: %w", path, err)
	}
	return c, nil
}

func (c *Config) applyDefaults() {
	if c.APIURL == "" {
		c.APIURL = DefaultAPIURL
	}
	c.APIURL = strings.TrimRight(c.APIURL, "/")
	if len(c.Docs) == 0 {
		c.Docs = append([]string(nil), DefaultDocs...)
	}
	for i := range c.Models {
		if c.Models[i].CredentialID == "" {
			c.Models[i].CredentialID = DefaultCredential
		}
		if len(c.Models[i].Harnesses) == 0 {
			c.Models[i].Harnesses = []string{DefaultHarness}
		}
	}
	if c.Sandbox == "" {
		c.Sandbox = DefaultSandbox
	}
	if c.SandboxCredentialID == "" {
		c.SandboxCredentialID = DefaultSandboxCredential
	}
	if c.TimeoutMinutes <= 0 {
		c.TimeoutMinutes = DefaultTimeoutMinutes
	}
}

// Validate reports the first problem SelfBench would refuse, before any request is sent.
func (c Config) Validate() error {
	if len(c.Models) == 0 {
		return errors.New("models: name at least one model (see `self-bench catalog --repo OWNER/NAME`)")
	}
	if len(c.Models) > 12 {
		return errors.New("models: SelfBench runs at most 12 models in one comparison")
	}
	for i, m := range c.Models {
		if m.CatalogID == "" {
			return fmt.Errorf("models[%d].catalogId is required", i)
		}
	}
	if c.Repo != "" && !validRepo(c.Repo) {
		return fmt.Errorf("repo %q: want owner/name", c.Repo)
	}
	for _, t := range c.Tasks {
		if _, _, ok := SplitTask(t); !ok {
			return fmt.Errorf("tasks: %q is not runId/taskId", t)
		}
	}
	return nil
}

// SplitTask splits a runId/taskId reference.
func SplitTask(ref string) (runID, taskID string, ok bool) {
	runID, taskID, ok = strings.Cut(ref, "/")
	return runID, taskID, ok && runID != "" && taskID != "" && !strings.Contains(taskID, "/")
}

func validRepo(repo string) bool {
	owner, name, ok := strings.Cut(repo, "/")
	return ok && owner != "" && name != "" && !strings.Contains(name, "/")
}

// Template is the config `dari-docs init` writes.
func Template(org, repo, model string) Config {
	if model == "" {
		model = "REPLACE-WITH-A-CATALOG-ID"
	}
	return Config{
		Org:    org,
		Repo:   repo,
		Docs:   append([]string(nil), DefaultDocs...),
		Models: []Model{{CatalogID: model, Harnesses: []string{DefaultHarness}}},
	}
}
