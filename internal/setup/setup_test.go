package setup

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/mupt-ai/dari-docs/internal/selfbench"
)

var models = []selfbench.CatalogModel{
	{ID: "dari/routing", Provider: "dari", Harnesses: []string{"pi"}},
	{ID: "claude-sonnet-5-5", Provider: "anthropic", Harnesses: []string{"claude-code", "pi"}},
	{ID: AuthorModel, Provider: "openai", Harnesses: []string{"codex", "pi"}},
}

// A workspace without SelfBench's managed models runs everything on its own credentials, and
// SelfBench refuses settings that name none: these are the shapes it validates.
func TestChooseNamesTheWorkspacesOwnCredentials(t *testing.T) {
	creds := []selfbench.Credential{
		{ID: "11111111-0000-0000-0000-000000000001", Kind: "anthropic", Auth: "claude-login"},
		{ID: "11111111-0000-0000-0000-000000000002", Kind: "anthropic", Auth: "api-key"},
		{ID: "11111111-0000-0000-0000-000000000003", Kind: "e2b"},
		{ID: "11111111-0000-0000-0000-000000000004", Kind: "modal"},
		{ID: "11111111-0000-0000-0000-000000000005", Kind: "openai", Auth: "codex-login"},
	}
	p := Choose(
		selfbench.Catalog{Models: models, Sandboxes: []string{"e2b", "modal", "daytona"}},
		selfbench.GenerationOptions{Models: models, Sandboxes: []string{"modal", "vercel", "e2b"}, Credentials: creds},
		"",
	)
	if len(p.Missing) > 0 {
		t.Fatalf("missing %v", p.Missing)
	}
	if p.Model.CatalogID != "claude-sonnet-5-5" || p.Model.Harnesses[0] != "claude-code" || p.Model.CredentialID != creds[1].ID {
		t.Errorf("model = %+v; want Sonnet on Claude Code with the Anthropic API key", p.Model)
	}
	if p.Sandbox != "modal" || p.SandboxCredentialID != creds[3].ID {
		t.Errorf("sandbox = %s %s; want Modal", p.Sandbox, p.SandboxCredentialID)
	}
	var g map[string]string
	if err := json.Unmarshal(p.Generation, &g); err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		"authorModel": AuthorModel, "verifierModel": AuthorModel, "reasoning": "high",
		"modelAccess": "credential", "modelCredentialId": creds[4].ID,
		"sandbox": "modal", "sandboxCredentialId": creds[3].ID,
		"harborEnvironment": "modal", "harborCredentialId": creds[3].ID,
	}
	for k, v := range want {
		if g[k] != v {
			t.Errorf("generation.%s = %q, want %q", k, g[k], v)
		}
	}
}

func TestChooseLeavesManagedSettingsToSelfBench(t *testing.T) {
	p := Choose(
		selfbench.Catalog{Models: models, Managed: selfbench.Managed{Models: true, Sandbox: true}},
		selfbench.GenerationOptions{Managed: selfbench.Managed{Models: true, Sandbox: true}},
		"",
	)
	if p.Model.CredentialID != "managed-model" || p.Sandbox != "managed" || p.SandboxCredentialID != "managed-sandbox" || p.Generation != nil {
		t.Errorf("plan = %+v; a managed deployment needs no credentials or generation settings", p)
	}
}

func TestChooseSaysWhatTheWorkspaceLacks(t *testing.T) {
	p := Choose(
		selfbench.Catalog{Models: models, Sandboxes: []string{"modal"}},
		selfbench.GenerationOptions{Models: models, Sandboxes: []string{"modal"}},
		"",
	)
	got := strings.Join(p.Missing, "\n")
	for _, want := range []string{"anthropic credential", "sandbox credential (modal) for running trials", "write tasks with"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
}
