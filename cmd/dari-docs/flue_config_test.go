package main

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/mupt-ai/dari-docs/internal/projectconfig"
)

func TestResolveFlueConfigUsesRuntimeModel(t *testing.T) {
	repo := t.TempDir()
	cfg := projectconfig.Config{TesterAgentID: "agt_tester", EditorAgentID: "agt_editor", AgentRuntime: "flue"}
	if err := projectconfig.Save(repo, cfg); err != nil {
		t.Fatal(err)
	}
	opts := checkOptimizeOptions{Command: "check", RepoRoot: repo, APIKey: "dari_test"}

	if err := resolveFlueCheckOptimizeConfig(&opts); err != nil {
		t.Fatal(err)
	}

	if opts.FeedbackAgent != "agt_tester" {
		t.Fatalf("FeedbackAgent = %q", opts.FeedbackAgent)
	}
	if len(opts.FeedbackLLMIDs) != 1 || opts.FeedbackLLMIDs[0] != "" {
		t.Fatalf("FeedbackLLMIDs = %#v, want one empty Flue llm marker", opts.FeedbackLLMIDs)
	}
}

func TestResolveFlueConfigRejectsLLMFlag(t *testing.T) {
	repo := t.TempDir()
	if err := projectconfig.Save(repo, projectconfig.Config{TesterAgentID: "agt_tester", AgentRuntime: "flue"}); err != nil {
		t.Fatal(err)
	}
	opts := checkOptimizeOptions{Command: "check", RepoRoot: repo, APIKey: "dari_test", LLMID: "claude-sonnet-4-6"}

	err := resolveFlueCheckOptimizeConfig(&opts)
	if err == nil || !strings.Contains(err.Error(), "Flue agents do not support") {
		t.Fatalf("err = %v, want Flue LLM rejection", err)
	}
}

func TestResolveFlueConfigRejectsLegacyConfig(t *testing.T) {
	repo := t.TempDir()
	if err := projectconfig.Save(repo, projectconfig.Config{TesterAgentID: "agt_tester"}); err != nil {
		t.Fatal(err)
	}
	opts := checkOptimizeOptions{Command: "check", RepoRoot: repo, APIKey: "dari_test"}

	err := resolveFlueCheckOptimizeConfig(&opts)
	if err == nil || !strings.Contains(err.Error(), "was not created for Flue agents") {
		t.Fatalf("err = %v, want legacy config rejection", err)
	}
}

func TestResolveFlueConfigAllowsExplicitAgentsWithoutConfig(t *testing.T) {
	repo := t.TempDir()
	opts := checkOptimizeOptions{Command: "optimize", RepoRoot: repo, APIKey: "dari_test", FeedbackAgent: "agt_tester", EditorAgent: "agt_editor"}

	if err := resolveFlueCheckOptimizeConfig(&opts); err != nil {
		t.Fatal(err)
	}
	if len(opts.FeedbackLLMIDs) != 1 || opts.FeedbackLLMIDs[0] != "" || opts.EditorLLMID != "" {
		t.Fatalf("LLM settings = feedback %#v editor %q, want Flue runtime defaults", opts.FeedbackLLMIDs, opts.EditorLLMID)
	}
}

func TestProjectConfigPathStillUnderDariDocs(t *testing.T) {
	if got := projectconfig.Path("/repo"); got != filepath.Join("/repo", ".dari-docs", "config.json") {
		t.Fatalf("config path = %q", got)
	}
}
