package projectconfig

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestExecutionConfigRoundTrip(t *testing.T) {
	repo := t.TempDir()
	want := Config{AgentRuntime: "harbor", LLMMode: "harbor-env", Execution: &Execution{Harness: "module:Agent", Sandbox: "modal", AgentKwargs: map[string]any{"count": float64(2)}, AgentEnv: map[string]string{"MODE": "test"}, EnvironmentKwargs: map[string]any{"region": "west"}}}
	if err := Save(repo, want); err != nil {
		t.Fatal(err)
	}
	got, ok, err := Load(repo)
	if err != nil || !ok {
		t.Fatalf("load: %v, %v", ok, err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v, want %#v", got, want)
	}
	b, err := json.Marshal(want)
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"execution", "harness", "sandbox", "agentKwargs", "agentEnv", "environmentKwargs"} {
		if !strings.Contains(string(b), `"`+field+`"`) {
			t.Fatalf("missing %s in %s", field, b)
		}
	}
}

func TestMergeExecutionDoesNotMutateMaps(t *testing.T) {
	base := &Execution{Harness: "claude-code", AgentKwargs: map[string]any{"a": 1}, AgentEnv: map[string]string{"MODE": "old"}, EnvironmentKwargs: map[string]any{"region": "west"}}
	override := &Execution{AgentKwargs: map[string]any{"b": 2}}
	got := MergeExecution(base, override)
	got.AgentKwargs["a"] = 3
	got.AgentKwargs["b"] = 4
	got.AgentEnv["MODE"] = "new"
	got.EnvironmentKwargs["region"] = "east"
	if base.AgentKwargs["a"] != 1 || override.AgentKwargs["b"] != 2 || base.AgentEnv["MODE"] != "old" || base.EnvironmentKwargs["region"] != "west" {
		t.Fatal("merge mutated input maps")
	}
	if got.Harness != "claude-code" {
		t.Fatal("lost harness")
	}
	if MergeExecution(nil, &Execution{}) != nil {
		t.Fatal("empty execution should be nil")
	}
}
