package main

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/mupt-ai/dari-docs/internal/projectconfig"
)

func TestResolveFlueConfigUsesDeploymentURLs(t *testing.T) {
	repo := t.TempDir()
	cfg := projectconfig.Config{TesterURL: "https://tester.example", EditorURL: "https://editor.example", AgentRuntime: "flue"}
	if err := projectconfig.Save(repo, cfg); err != nil {
		t.Fatal(err)
	}
	opts := checkOptimizeOptions{Command: "optimize", RepoRoot: repo, Parallel: 1}

	if err := resolveFlueCheckOptimizeConfig(&opts); err != nil {
		t.Fatal(err)
	}
	if opts.TesterURL != cfg.TesterURL || opts.EditorURL != cfg.EditorURL {
		t.Fatalf("urls = %q/%q, want %q/%q", opts.TesterURL, opts.EditorURL, cfg.TesterURL, cfg.EditorURL)
	}
}

func TestResolveLLMFlagsUsesGlobalModelForFeedback(t *testing.T) {
	opts := checkOptimizeOptions{LLMID: "claude-sonnet-4-6"}
	opts.resolveLLMFlags()
	if strings.Join(opts.FeedbackLLMIDs, ",") != "claude-sonnet-4-6" {
		t.Fatalf("feedback models = %#v", opts.FeedbackLLMIDs)
	}
	if opts.EditorLLMID != "claude-sonnet-4-6" {
		t.Fatalf("editor model = %q", opts.EditorLLMID)
	}
}

func TestResolveLLMFlagsFeedbackOverridesGlobalModel(t *testing.T) {
	opts := checkOptimizeOptions{LLMID: "claude-sonnet-4-6", FeedbackLLMRaw: []string{"gpt-5.5,claude-opus-4-7"}, EditorLLMIDFlag: "claude-haiku-4-5"}
	opts.resolveLLMFlags()
	if strings.Join(opts.FeedbackLLMIDs, ",") != "gpt-5.5,claude-opus-4-7" {
		t.Fatalf("feedback models = %#v", opts.FeedbackLLMIDs)
	}
	if opts.EditorLLMID != "claude-haiku-4-5" {
		t.Fatalf("editor model = %q", opts.EditorLLMID)
	}
}

func TestResolveFlueConfigRejectsDariFlags(t *testing.T) {
	repo := t.TempDir()
	if err := projectconfig.Save(repo, projectconfig.Config{TesterURL: "https://tester.example", AgentRuntime: "flue"}); err != nil {
		t.Fatal(err)
	}
	opts := checkOptimizeOptions{Command: "check", RepoRoot: repo, Parallel: 1, APIKey: "dari_test"}

	err := resolveFlueCheckOptimizeConfig(&opts)
	if err == nil || !strings.Contains(err.Error(), "Dari API and agent ID flags are not supported") {
		t.Fatalf("err = %v, want Dari flag rejection", err)
	}
}

func TestResolveFlueConfigRejectsLegacyConfig(t *testing.T) {
	repo := t.TempDir()
	if err := projectconfig.Save(repo, projectconfig.Config{TesterAgentID: "agt_tester"}); err != nil {
		t.Fatal(err)
	}
	opts := checkOptimizeOptions{Command: "check", RepoRoot: repo, Parallel: 1}

	err := resolveFlueCheckOptimizeConfig(&opts)
	if err == nil || !strings.Contains(err.Error(), "was not created for Flue deployments") {
		t.Fatalf("err = %v, want legacy config rejection", err)
	}
}

func TestResolveFlueConfigRequiresTesterURL(t *testing.T) {
	repo := t.TempDir()
	opts := checkOptimizeOptions{Command: "check", RepoRoot: repo, Parallel: 1}

	err := resolveFlueCheckOptimizeConfig(&opts)
	if err == nil || !strings.Contains(err.Error(), "missing Flue tester URL") {
		t.Fatalf("err = %v, want tester URL error", err)
	}
}

func TestProjectConfigPathStillUnderDariDocs(t *testing.T) {
	if got := projectconfig.Path("/repo"); got != filepath.Join("/repo", ".dari-docs", "config.json") {
		t.Fatalf("config path = %q", got)
	}
}
