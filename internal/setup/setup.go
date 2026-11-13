// Package setup chooses what a new repository's docs checks run on: a model, its credential, a
// sandbox, and the settings SelfBench writes and builds tasks with. A workspace without SelfBench's
// managed models or sandboxes needs its own credentials named for each.
package setup

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/mupt-ai/dari-docs/internal/config"
	"github.com/mupt-ai/dari-docs/internal/selfbench"
)

// AuthorModel is the model the self-bench CLI writes tasks with by default.
const AuthorModel = "gpt-6.1-sol"

// Plan is what init writes into the config, and what it couldn't choose.
type Plan struct {
	Model               config.Model
	Sandbox             string
	SandboxCredentialID string
	Generation          json.RawMessage
	// Missing says what the workspace lacks, one line each.
	Missing []string
}

// Choose picks the model (the requested one, else the catalog's newest Claude Sonnet), a
// credential for it, a sandbox, and the generation settings.
func Choose(catalog selfbench.Catalog, gen selfbench.GenerationOptions, requested string) Plan {
	var p Plan
	creds := gen.Credentials
	id := requested
	if id == "" {
		id = DefaultModel(catalog.Models)
	}
	model, found := find(catalog.Models, id)
	if !found {
		p.Missing = append(p.Missing, fmt.Sprintf("SelfBench's catalog has no model %q; set `models[0].catalogId`", id))
		model = selfbench.CatalogModel{ID: id}
	}
	p.Model = config.Model{CatalogID: model.ID, Harnesses: []string{harness(model)}}

	switch {
	case catalog.Managed.Models:
		p.Model.CredentialID = config.DefaultCredential
	default:
		if c, ok := credentialFor(creds, model.Provider); ok {
			p.Model.CredentialID = c.ID
		} else {
			p.Missing = append(p.Missing, fmt.Sprintf("a %s credential for %s (SelfBench → Settings → Credentials)", orUnknown(model.Provider), model.ID))
		}
	}

	if catalog.Managed.Sandbox {
		p.Sandbox, p.SandboxCredentialID = config.DefaultSandbox, config.DefaultSandboxCredential
	} else if c, ok := sandboxFor(creds, catalog.Sandboxes); ok {
		p.Sandbox, p.SandboxCredentialID = c.Kind, c.ID
	} else {
		p.Missing = append(p.Missing, fmt.Sprintf("a sandbox credential (%s) for running trials", strings.Join(catalog.Sandboxes, ", ")))
	}

	if gen.Available != nil && !*gen.Available || gen.Managed.Models && gen.Managed.Sandbox {
		// SelfBench takes no generation settings, or its managed ones serve.
		return p
	}
	g := map[string]any{"reasoning": "high"}
	if gen.Managed.Models {
		g["authorModel"], g["verifierModel"], g["modelAccess"] = AuthorModel, AuthorModel, "managed"
	} else if author, c, ok := authorFor(gen, creds, model.ID); ok {
		g["authorModel"], g["verifierModel"], g["modelAccess"], g["modelCredentialId"] = author, author, "credential", c.ID
	} else {
		p.Missing = append(p.Missing, "a model credential SelfBench can write tasks with (OpenAI or Anthropic)")
		return p
	}
	if gen.Managed.Sandbox {
		g["sandbox"] = "managed"
	} else if c, ok := sandboxFor(creds, gen.Sandboxes); ok {
		// Verification runs in a Harbor environment of its own; the same account serves both.
		g["sandbox"], g["sandboxCredentialId"] = c.Kind, c.ID
		g["harborEnvironment"], g["harborCredentialId"] = c.Kind, c.ID
	} else {
		p.Missing = append(p.Missing, fmt.Sprintf("a sandbox credential (%s) for building tasks", strings.Join(gen.Sandboxes, ", ")))
		return p
	}
	p.Generation, _ = json.Marshal(g)
	return p
}

// DefaultModel is the newest Claude Sonnet in the catalog (it lists newest first), else its first
// model that isn't a router.
func DefaultModel(models []selfbench.CatalogModel) string {
	for _, m := range models {
		if strings.Contains(m.ID, "claude-sonnet") {
			return m.ID
		}
	}
	for _, m := range models {
		if !strings.Contains(m.ID, "routing") {
			return m.ID
		}
	}
	return ""
}

func find(models []selfbench.CatalogModel, id string) (selfbench.CatalogModel, bool) {
	for _, m := range models {
		if m.ID == id {
			return m, true
		}
	}
	return selfbench.CatalogModel{}, false
}

// harness is Claude Code when the model runs on it, else the first harness it runs on.
func harness(m selfbench.CatalogModel) string {
	for _, h := range m.Harnesses {
		if h == config.DefaultHarness {
			return h
		}
	}
	if len(m.Harnesses) > 0 {
		return m.Harnesses[0]
	}
	return config.DefaultHarness
}

// credentialFor is the workspace's credential for a provider: an API key first, else a sign-in.
func credentialFor(creds []selfbench.Credential, provider string) (selfbench.Credential, bool) {
	var login *selfbench.Credential
	for i, c := range creds {
		if provider == "" || c.Kind != provider {
			continue
		}
		if c.Auth == "api-key" {
			return c, true
		}
		if login == nil {
			login = &creds[i]
		}
	}
	if login != nil {
		return *login, true
	}
	return selfbench.Credential{}, false
}

// sandboxFor is the workspace's credential for the first offered sandbox it has one for, Modal first.
func sandboxFor(creds []selfbench.Credential, offered []string) (selfbench.Credential, bool) {
	order := append([]string{"modal"}, offered...)
	for _, kind := range order {
		if !contains(offered, kind) {
			continue
		}
		for _, c := range creds {
			if c.Kind == kind {
				return c, true
			}
		}
	}
	return selfbench.Credential{}, false
}

// authorFor is a model SelfBench can write tasks with on the workspace's credentials: the CLI's
// default author, then the model checks run, then any other.
func authorFor(gen selfbench.GenerationOptions, creds []selfbench.Credential, checkModel string) (string, selfbench.Credential, bool) {
	candidates := []string{AuthorModel, checkModel}
	for _, m := range gen.Models {
		candidates = append(candidates, m.ID)
	}
	for _, id := range candidates {
		m, ok := find(gen.Models, id)
		if !ok || strings.Contains(m.ID, "routing") {
			continue
		}
		if c, ok := credentialFor(creds, m.Provider); ok {
			return m.ID, c, true
		}
	}
	return "", selfbench.Credential{}, false
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

func orUnknown(s string) string {
	if s == "" {
		return "model"
	}
	return s
}
