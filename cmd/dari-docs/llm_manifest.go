package main

import (
	"bytes"
	"fmt"
	"os"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

type llmModelEntry struct {
	node     *yaml.Node
	provider string
}

type llmManifest struct {
	path    string
	doc     yaml.Node
	entries []llmModelEntry
}

func setLLMAPIKeySecret(path, secret string) error {
	manifest, err := loadLLMManifest(path)
	if err != nil {
		return err
	}
	providers := map[string]bool{}
	for _, entry := range manifest.entries {
		providers[entry.provider] = true
	}
	if len(providers) > 1 {
		return fmt.Errorf("--llm-api-key-secret cannot be applied to multiple LLM providers (%s); use --anthropic-api-key-secret and/or --openai-api-key-secret", strings.Join(providerNames(providers), ", "))
	}
	for _, entry := range manifest.entries {
		yamlSetMappingScalar(entry.node, "api_key_secret", secret)
	}
	return manifest.write()
}

func setLLMAPIKeySecretsByProvider(path string, providerSecrets map[string]string) error {
	providerSecrets = normalizeProviderSecrets(providerSecrets)
	if len(providerSecrets) == 0 {
		return nil
	}

	manifest, err := loadLLMManifest(path)
	if err != nil {
		return err
	}
	matched := map[string]bool{}
	for _, entry := range manifest.entries {
		secret, ok := providerSecrets[entry.provider]
		if !ok {
			continue
		}
		matched[entry.provider] = true
		yamlSetMappingScalar(entry.node, "api_key_secret", secret)
	}
	for provider := range providerSecrets {
		if !matched[provider] {
			return fmt.Errorf("could not find %s llm option in %s", provider, path)
		}
	}
	return manifest.write()
}

func setAgentDefaultProviderSecret(path, secret string) error {
	secret = strings.TrimSpace(secret)
	if secret == "" {
		return nil
	}
	if manifestHasLLMBlock(path) {
		return setLLMAPIKeySecret(path, secret)
	}
	return setFlueProviderSecrets(path, map[string]string{"anthropic": secret})
}

func setAgentProviderSecrets(path string, providerSecrets map[string]string) error {
	providerSecrets = normalizeProviderSecrets(providerSecrets)
	if len(providerSecrets) == 0 {
		return nil
	}
	if manifestHasLLMBlock(path) {
		return setLLMAPIKeySecretsByProvider(path, providerSecrets)
	}
	return setFlueProviderSecrets(path, providerSecrets)
}

func manifestHasLLMBlock(path string) bool {
	b, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(b, &doc); err != nil {
		return false
	}
	return yamlMappingValue(yamlDocumentRoot(&doc), "llm") != nil
}

func setFlueProviderSecrets(path string, providerSecrets map[string]string) error {
	providerSecrets = normalizeProviderSecrets(providerSecrets)
	if len(providerSecrets) == 0 {
		return nil
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(b, &doc); err != nil {
		return fmt.Errorf("parse %s: %w", path, err)
	}
	root := yamlDocumentRoot(&doc)
	if root == nil || root.Kind != yaml.MappingNode {
		return fmt.Errorf("manifest %s must be a mapping", path)
	}
	sandbox := yamlEnsureMapping(root, "sandbox")
	env := yamlEnsureMapping(sandbox, "env")
	secrets := yamlEnsureSequence(sandbox, "secrets")

	removeSecrets := map[string]bool{}
	for provider := range providerSecrets {
		envName := flueProviderSecretEnvName(provider)
		if envName == "" {
			return fmt.Errorf("unsupported Flue LLM provider %q", provider)
		}
		if old := yamlMappingValue(env, envName); old != nil {
			removeSecrets[strings.TrimSpace(old.Value)] = true
		}
		if defaultName := flueProviderDefaultSecretName(provider); defaultName != "" {
			removeSecrets[defaultName] = true
		}
	}
	for provider, secret := range providerSecrets {
		yamlSetMappingScalar(env, flueProviderSecretEnvName(provider), secret)
	}
	yamlPruneSequenceScalars(secrets, removeSecrets)
	for _, secret := range providerSecrets {
		yamlAppendUniqueSequenceScalar(secrets, secret)
	}

	var out bytes.Buffer
	enc := yaml.NewEncoder(&out)
	enc.SetIndent(2)
	if err := enc.Encode(&doc); err != nil {
		_ = enc.Close()
		return fmt.Errorf("encode %s: %w", path, err)
	}
	if err := enc.Close(); err != nil {
		return fmt.Errorf("encode %s: %w", path, err)
	}
	return os.WriteFile(path, out.Bytes(), 0o644)
}

func flueProviderSecretEnvName(provider string) string {
	switch normalizeProvider(provider) {
	case "anthropic":
		return "DARI_DOCS_ANTHROPIC_API_KEY_SECRET_NAME"
	case "openai":
		return "DARI_DOCS_OPENAI_API_KEY_SECRET_NAME"
	case "openrouter":
		return "DARI_DOCS_OPENROUTER_API_KEY_SECRET_NAME"
	default:
		return ""
	}
}

func flueProviderDefaultSecretName(provider string) string {
	switch normalizeProvider(provider) {
	case "anthropic":
		return "ANTHROPIC_API_KEY"
	case "openai":
		return "OPENAI_API_KEY"
	case "openrouter":
		return "OPENROUTER_API_KEY"
	default:
		return ""
	}
}

func normalizeProviderSecrets(in map[string]string) map[string]string {
	out := map[string]string{}
	for provider, secret := range in {
		provider = normalizeProvider(provider)
		secret = strings.TrimSpace(secret)
		if provider != "" && secret != "" {
			out[provider] = secret
		}
	}
	return out
}

func loadLLMManifest(path string) (*llmManifest, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	manifest := &llmManifest{path: path}
	if err := yaml.Unmarshal(b, &manifest.doc); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	entries, err := collectLLMModelEntries(path, &manifest.doc)
	if err != nil {
		return nil, err
	}
	manifest.entries = entries
	return manifest, nil
}

func (m *llmManifest) write() error {
	var out bytes.Buffer
	enc := yaml.NewEncoder(&out)
	enc.SetIndent(2)
	if err := enc.Encode(&m.doc); err != nil {
		_ = enc.Close()
		return fmt.Errorf("encode %s: %w", m.path, err)
	}
	if err := enc.Close(); err != nil {
		return fmt.Errorf("encode %s: %w", m.path, err)
	}
	return os.WriteFile(m.path, out.Bytes(), 0o644)
}

func collectLLMModelEntries(path string, doc *yaml.Node) ([]llmModelEntry, error) {
	root := yamlDocumentRoot(doc)
	llm := yamlMappingValue(root, "llm")
	if llm == nil {
		return nil, fmt.Errorf("could not find llm block in %s", path)
	}

	var entries []llmModelEntry
	if options := yamlMappingValue(llm, "options"); options != nil {
		if options.Kind != yaml.MappingNode {
			return nil, fmt.Errorf("llm.options in %s must be a mapping", path)
		}
		for i := 1; i < len(options.Content); i += 2 {
			option := options.Content[i]
			model := yamlMappingValue(option, "model")
			if model == nil {
				continue
			}
			entries = append(entries, llmModelEntry{node: option, provider: providerForLLMNode(option, model.Value)})
		}
	} else if model := yamlMappingValue(llm, "model"); model != nil {
		entries = append(entries, llmModelEntry{node: llm, provider: providerForLLMNode(llm, model.Value)})
	}
	if len(entries) == 0 {
		return nil, fmt.Errorf("could not find llm.model in %s", path)
	}
	return entries, nil
}

func providerForLLMNode(node *yaml.Node, model string) string {
	if provider := yamlMappingValue(node, "provider"); provider != nil {
		return normalizeProvider(provider.Value)
	}
	return inferProviderFromModel(model)
}

func providerNames(providers map[string]bool) []string {
	names := make([]string, 0, len(providers))
	for provider := range providers {
		if provider == "" {
			provider = "unspecified"
		}
		names = append(names, provider)
	}
	sort.Strings(names)
	return names
}

func yamlDocumentRoot(doc *yaml.Node) *yaml.Node {
	if doc.Kind == yaml.DocumentNode && len(doc.Content) > 0 {
		return doc.Content[0]
	}
	return doc
}

func yamlMappingValue(node *yaml.Node, key string) *yaml.Node {
	if node == nil || node.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(node.Content); i += 2 {
		if node.Content[i].Value == key {
			return node.Content[i+1]
		}
	}
	return nil
}

func yamlSetMappingScalar(node *yaml.Node, key, value string) {
	if node == nil || node.Kind != yaml.MappingNode {
		return
	}
	for i := 0; i+1 < len(node.Content); i += 2 {
		if node.Content[i].Value == key {
			node.Content[i+1].Kind = yaml.ScalarNode
			node.Content[i+1].Tag = "!!str"
			node.Content[i+1].Value = value
			return
		}
	}
	node.Content = append(node.Content,
		&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key},
		&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: value},
	)
}

func yamlEnsureMapping(node *yaml.Node, key string) *yaml.Node {
	if existing := yamlMappingValue(node, key); existing != nil {
		if existing.Kind != yaml.MappingNode {
			existing.Kind = yaml.MappingNode
			existing.Tag = "!!map"
			existing.Content = nil
		}
		return existing
	}
	child := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	node.Content = append(node.Content,
		&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key},
		child,
	)
	return child
}

func yamlEnsureSequence(node *yaml.Node, key string) *yaml.Node {
	if existing := yamlMappingValue(node, key); existing != nil {
		if existing.Kind != yaml.SequenceNode {
			existing.Kind = yaml.SequenceNode
			existing.Tag = "!!seq"
			existing.Content = nil
		}
		return existing
	}
	child := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
	node.Content = append(node.Content,
		&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key},
		child,
	)
	return child
}

func yamlPruneSequenceScalars(node *yaml.Node, remove map[string]bool) {
	if node == nil || node.Kind != yaml.SequenceNode || len(remove) == 0 {
		return
	}
	out := node.Content[:0]
	for _, item := range node.Content {
		if item.Kind == yaml.ScalarNode && remove[strings.TrimSpace(item.Value)] {
			continue
		}
		out = append(out, item)
	}
	node.Content = out
}

func yamlAppendUniqueSequenceScalar(node *yaml.Node, value string) {
	value = strings.TrimSpace(value)
	if node == nil || node.Kind != yaml.SequenceNode || value == "" {
		return
	}
	for _, item := range node.Content {
		if item.Kind == yaml.ScalarNode && strings.TrimSpace(item.Value) == value {
			return
		}
	}
	node.Content = append(node.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: value})
}

func normalizeProvider(provider string) string {
	return strings.ToLower(strings.TrimSpace(provider))
}

func inferProviderFromModel(model string) string {
	model = strings.ToLower(strings.TrimSpace(model))
	if strings.HasPrefix(model, "openai/") {
		return "openai"
	}
	if strings.HasPrefix(model, "anthropic/") {
		return "anthropic"
	}
	return ""
}
