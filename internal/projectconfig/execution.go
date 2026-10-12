package projectconfig

import (
	"fmt"
	"strings"

	"github.com/mupt-ai/dari-docs/internal/runtimeenv"
)

// Execution selects Harbor's harness and sandbox; unset fields use server defaults.
type Execution struct {
	Harness           string            `json:"harness,omitempty"`
	Sandbox           string            `json:"sandbox,omitempty"`
	AgentKwargs       map[string]any    `json:"agentKwargs,omitempty"`
	AgentEnv          map[string]string `json:"agentEnv,omitempty"`
	EnvironmentKwargs map[string]any    `json:"environmentKwargs,omitempty"`
}

// Empty reports whether the execution object can be omitted from a request.
func (e *Execution) Empty() bool {
	return e == nil || (e.Harness == "" && e.Sandbox == "" && len(e.AgentKwargs) == 0 && len(e.AgentEnv) == 0 && len(e.EnvironmentKwargs) == 0)
}

// Validate rejects secrets in the nonsecret agent environment, including config values.
func (e *Execution) Validate() error {
	if e == nil {
		return nil
	}
	for name := range e.AgentEnv {
		validated, err := runtimeenv.ValidateName(name)
		if err != nil {
			return err
		}
		if validated != name {
			return fmt.Errorf("agent environment name %q must not contain surrounding whitespace", name)
		}
		upper := strings.ToUpper(name)
		for _, marker := range []string{"KEY", "TOKEN", "SECRET", "PASSWORD", "PASSWD", "CREDENTIAL", "AUTH"} {
			if strings.Contains(upper, marker) {
				return fmt.Errorf("agent environment name %q looks secret; use provider secrets on the runner or --live-verify --secret-env for runtime product secrets", name)
			}
		}
	}
	return nil
}

// MergeExecution returns a fresh object, with override fields and map keys taking precedence.
func MergeExecution(base, override *Execution) *Execution {
	out := &Execution{}
	for _, e := range []*Execution{base, override} {
		if e == nil {
			continue
		}
		if e.Harness != "" {
			out.Harness = e.Harness
		}
		if e.Sandbox != "" {
			out.Sandbox = e.Sandbox
		}
		if len(e.AgentKwargs) > 0 && out.AgentKwargs == nil {
			out.AgentKwargs = map[string]any{}
		}
		for k, v := range e.AgentKwargs {
			out.AgentKwargs[k] = v
		}
		if len(e.AgentEnv) > 0 && out.AgentEnv == nil {
			out.AgentEnv = map[string]string{}
		}
		for k, v := range e.AgentEnv {
			out.AgentEnv[k] = v
		}
		if len(e.EnvironmentKwargs) > 0 && out.EnvironmentKwargs == nil {
			out.EnvironmentKwargs = map[string]any{}
		}
		for k, v := range e.EnvironmentKwargs {
			out.EnvironmentKwargs[k] = v
		}
	}
	if out.Empty() {
		return nil
	}
	return out
}
