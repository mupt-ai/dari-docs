package main

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/mupt-ai/dari-docs/internal/projectconfig"
)

func TestParseKwargs(t *testing.T) {
	tests := []struct {
		name, input string
		want        any
		bad         bool
	}{
		{"string", "model=custom/model", "custom/model", false},
		{"number", "n=3", float64(3), false},
		{"boolean", "enabled=true", true, false},
		{"object", `options={"a":[1,false]}`, map[string]any{"a": []any{float64(1), false}}, false},
		{"null", "value=null", nil, false},
		{"quoted", `value="true"`, "true", false},
		{"equals", "value=a=b", "a=b", false},
		{"empty value", "value=", "", false},
		{"missing equals", "value", nil, true},
		{"missing key", "=value", nil, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseKwargs("agent-kwarg", []string{tt.input})
			if tt.bad {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			for _, value := range got {
				if !reflect.DeepEqual(value, tt.want) {
					t.Fatalf("got %#v, want %#v", value, tt.want)
				}
			}
		})
	}
}

func TestExecutionCLIOverridesConfig(t *testing.T) {
	t.Setenv("DARI_DOCS_RUNNER_TOKEN", "")
	repo := t.TempDir()
	cfg := projectconfig.Config{AgentRuntime: "harbor", LLMMode: "harbor-env", TesterURL: "https://tester.example", Execution: &projectconfig.Execution{
		Harness: "old", Sandbox: "modal", AgentKwargs: map[string]any{"keep": true, "replace": "old"}, AgentEnv: map[string]string{"MODE": "old"}, EnvironmentKwargs: map[string]any{"region": "west"},
	}}
	if err := projectconfig.Save(repo, cfg); err != nil {
		t.Fatal(err)
	}
	opts := checkOptimizeOptions{Command: "check", RepoRoot: repo, Harness: "custom.agent:Harness", EditorHarness: "custom.editor:Harness", Sandbox: "custom.env:Sandbox", AgentKwargs: []string{"replace=7"}, AgentEnv: []string{"MODE=new"}, SandboxKwargs: []string{"region=east"}}
	if err := resolveHarborCheckOptimizeConfig(&opts); err != nil {
		t.Fatal(err)
	}
	want := &projectconfig.Execution{Harness: opts.Harness, Sandbox: opts.Sandbox, AgentKwargs: map[string]any{"keep": true, "replace": float64(7)}, AgentEnv: map[string]string{"MODE": "new"}, EnvironmentKwargs: map[string]any{"region": "east"}}
	if !reflect.DeepEqual(opts.Execution, want) {
		t.Fatalf("execution = %#v", opts.Execution)
	}
	if cfg.Execution.Harness != "old" {
		t.Fatal("mutated config")
	}
}

func TestExecutionDefaultsOmitted(t *testing.T) {
	t.Setenv("DARI_DOCS_RUNNER_TOKEN", "")
	opts := checkOptimizeOptions{}
	if err := opts.resolveExecution(); err != nil {
		t.Fatal(err)
	}
	if opts.Execution != nil {
		t.Fatalf("execution = %#v", opts.Execution)
	}
}

func TestAgentEnvRejectsSecrets(t *testing.T) {
	for _, name := range []string{"API_KEY", "GITHUB_TOKEN", "CLIENT_SECRET", "PASSWORD", "AWS_CREDENTIALS", "AUTH_HEADER", "bad-name"} {
		t.Run(name, func(t *testing.T) {
			opts := checkOptimizeOptions{AgentEnv: []string{name + "=do-not-print"}}
			err := opts.resolveExecution()
			if err == nil || strings.Contains(err.Error(), "do-not-print") {
				t.Fatalf("err = %v", err)
			}
		})
	}
	repo := t.TempDir()
	cfg := projectconfig.Config{AgentRuntime: "harbor", LLMMode: "harbor-env", TesterURL: "https://tester.example", Execution: &projectconfig.Execution{AgentEnv: map[string]string{"API_KEY": "secret"}}}
	if err := projectconfig.Save(repo, cfg); err != nil {
		t.Fatal(err)
	}
	opts := checkOptimizeOptions{Command: "check", RepoRoot: repo}
	if err := resolveHarborCheckOptimizeConfig(&opts); err == nil {
		t.Fatal("expected config secret rejection")
	}
}

func TestRunnerTokenEnvironment(t *testing.T) {
	t.Setenv("DARI_DOCS_RUNNER_TOKEN", "automatic-token")
	t.Setenv("CUSTOM_RUNNER_TOKEN", "explicit-token")
	for _, tt := range []struct {
		name, env, want string
		bad             bool
	}{
		{"automatic", "", "automatic-token", false},
		{"explicit", "CUSTOM_RUNNER_TOKEN", "explicit-token", false},
		{"missing", "MISSING_RUNNER_TOKEN", "", true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if tt.bad {
				t.Setenv(tt.env, "")
			}
			opts := checkOptimizeOptions{RunnerTokenEnv: tt.env}
			err := opts.resolveExecution()
			if (err != nil) != tt.bad {
				t.Fatalf("err = %v", err)
			}
			if opts.RunnerToken != tt.want {
				t.Fatalf("token = %q", opts.RunnerToken)
			}
		})
	}
}

func TestHarborFlagsAcceptCustomClasses(t *testing.T) {
	opts := defaultCheckOptimizeOptions("optimize")
	cmd := newCheckOptimizeCommand("optimize")
	// Bind a separate command's flags to test parsing without making network calls.
	cmd.ResetFlags()
	bindCheckOptimizeFlags(cmd, opts)
	err := cmd.ParseFlags([]string{"--harness", "pkg.agent:Agent", "--editor-harness", "pkg.editor:Agent", "--sandbox", "pkg.env:Env", "--agent-kwarg", `options={"count":2}`, "--agent-env", "MODE=test", "--sandbox-kwarg", "region=east", "--runner-token-env", "TEST_RUNNER_TOKEN"})
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("TEST_RUNNER_TOKEN", "token")
	if err := opts.resolveExecution(); err != nil {
		t.Fatal(err)
	}
	b, err := json.Marshal(opts.Execution)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), `"harness":"pkg.agent:Agent"`) || opts.EditorHarness != "pkg.editor:Agent" {
		t.Fatalf("execution = %s", b)
	}
}

func TestFlueConfigRequiresRedeploy(t *testing.T) {
	repo := t.TempDir()
	if err := projectconfig.Save(repo, projectconfig.Config{AgentRuntime: "flue", LLMMode: "flue-env"}); err != nil {
		t.Fatal(err)
	}
	opts := checkOptimizeOptions{RepoRoot: repo, Command: "check", TesterURL: "https://tester.example"}
	err := resolveHarborCheckOptimizeConfig(&opts)
	if err == nil || !strings.Contains(err.Error(), "init") || !strings.Contains(err.Error(), "redeploy") {
		t.Fatalf("err = %v", err)
	}
}
