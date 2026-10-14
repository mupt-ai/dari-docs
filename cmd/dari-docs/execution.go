package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/mupt-ai/dari-docs/internal/projectconfig"
	"github.com/mupt-ai/dari-docs/internal/runtimeenv"
)

func parseKwargs(flag string, values []string) (map[string]any, error) {
	out := make(map[string]any, len(values))
	for _, raw := range values {
		key, value, ok := strings.Cut(raw, "=")
		key = strings.TrimSpace(key)
		if !ok || key == "" {
			return nil, fmt.Errorf("--%s requires KEY=JSON_OR_STRING", flag)
		}
		var parsed any
		if json.Unmarshal([]byte(value), &parsed) != nil {
			parsed = value
		}
		out[key] = parsed
	}
	return out, nil
}

func (opts *checkOptimizeOptions) resolveExecution() error {
	agent, err := parseKwargs("agent-kwarg", opts.AgentKwargs)
	if err != nil {
		return err
	}
	sandbox, err := parseKwargs("sandbox-kwarg", opts.SandboxKwargs)
	if err != nil {
		return err
	}
	env := map[string]string{}
	for _, raw := range opts.AgentEnv {
		name, value, ok := strings.Cut(raw, "=")
		if !ok || name == "" {
			return fmt.Errorf("--agent-env requires NAME=VALUE")
		}
		env[name] = value
	}
	opts.Execution = projectconfig.MergeExecution(opts.Execution, &projectconfig.Execution{
		Harness: opts.Harness, Sandbox: opts.Sandbox, AgentKwargs: agent, AgentEnv: env, EnvironmentKwargs: sandbox,
	})
	if err := opts.Execution.Validate(); err != nil {
		return err
	}
	name := opts.RunnerTokenEnv
	explicit := name != ""
	if name == "" {
		name = "DARI_DOCS_RUNNER_TOKEN"
	}
	if _, err := runtimeenv.ValidateName(name); err != nil {
		return err
	}
	opts.RunnerToken = os.Getenv(name)
	if explicit && opts.RunnerToken == "" {
		return fmt.Errorf("--runner-token-env %s requested but env var is empty", name)
	}
	return nil
}
