package agentbundle

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"path"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	RuntimeSecretsName                = "DARI_DOCS_RUNTIME_SECRETS_JSON"
	maxManagedBundleUncompressedBytes = 100 * 1024 * 1024
	maxManagedBundleManifestFileBytes = 1 * 1024 * 1024
)

type managedManifest struct {
	LLM     *managedManifestLLM `yaml:"llm"`
	Sandbox *struct {
		ProviderAPIKeySecret string   `yaml:"provider_api_key_secret"`
		Secrets              []string `yaml:"secrets"`
	} `yaml:"sandbox"`
}

type managedManifestLLM struct {
	Default      string                              `yaml:"default"`
	Options      map[string]managedManifestLLMOption `yaml:"options"`
	Model        string                              `yaml:"model"`
	Provider     string                              `yaml:"provider"`
	BaseURL      string                              `yaml:"base_url"`
	APIKeySecret string                              `yaml:"api_key_secret"`
}

type managedManifestLLMOption struct {
	Model        string `yaml:"model"`
	Provider     string `yaml:"provider"`
	BaseURL      string `yaml:"base_url"`
	APIKeySecret string `yaml:"api_key_secret"`
}

func ValidateManagedManifestYAML(data []byte) error {
	var manifest managedManifest
	if err := yaml.Unmarshal(data, &manifest); err != nil {
		return fmt.Errorf("parse dari.yml: %w", err)
	}
	if manifest.LLM == nil {
		return fmt.Errorf("llm is required")
	}
	if err := validateManagedManifestLLM(manifest.LLM); err != nil {
		return err
	}
	if manifest.Sandbox == nil {
		return nil
	}
	if strings.TrimSpace(manifest.Sandbox.ProviderAPIKeySecret) != "" {
		return fmt.Errorf("sandbox.provider_api_key_secret must be omitted for managed Dari Docs agents")
	}
	if len(manifest.Sandbox.Secrets) == 0 {
		return nil
	}
	if len(manifest.Sandbox.Secrets) != 1 || manifest.Sandbox.Secrets[0] != RuntimeSecretsName {
		return fmt.Errorf("sandbox.secrets must be omitted or exactly [%s] for managed Dari Docs agents", RuntimeSecretsName)
	}
	return nil
}

func validateManagedManifestLLM(llm *managedManifestLLM) error {
	if len(llm.Options) == 0 {
		return validateManagedManifestLLMOption("llm", managedManifestLLMOption{Model: llm.Model, Provider: llm.Provider, BaseURL: llm.BaseURL, APIKeySecret: llm.APIKeySecret})
	}
	if strings.TrimSpace(llm.Model) != "" || strings.TrimSpace(llm.Provider) != "" || strings.TrimSpace(llm.BaseURL) != "" || strings.TrimSpace(llm.APIKeySecret) != "" {
		return fmt.Errorf("llm must not mix top-level model/provider fields with llm.options")
	}
	if strings.TrimSpace(llm.Default) == "" {
		return fmt.Errorf("llm.default is required when llm.options is used")
	}
	if _, ok := llm.Options[strings.TrimSpace(llm.Default)]; !ok {
		return fmt.Errorf("llm.default must reference a key in llm.options")
	}
	for id, opt := range llm.Options {
		if strings.TrimSpace(id) == "" {
			return fmt.Errorf("llm.options contains an empty option id")
		}
		if err := validateManagedManifestLLMOption("llm.options."+id, opt); err != nil {
			return err
		}
	}
	return nil
}

func validateManagedManifestLLMOption(label string, opt managedManifestLLMOption) error {
	if strings.TrimSpace(opt.Model) == "" {
		return fmt.Errorf("%s.model is required", label)
	}
	if strings.TrimSpace(opt.APIKeySecret) != "" {
		return fmt.Errorf("%s.api_key_secret must be omitted for managed Dari Docs agents", label)
	}
	if strings.TrimSpace(opt.BaseURL) != "" {
		return fmt.Errorf("%s.base_url must be omitted for managed Dari Docs agents", label)
	}
	provider := strings.TrimSpace(opt.Provider)
	if provider != "" && provider != "openrouter" && provider != "openai" && provider != "anthropic" {
		return fmt.Errorf("%s.provider must be omitted or one of openrouter, openai, or anthropic", label)
	}
	return nil
}

func ValidateManagedBundle(content []byte, label string) error {
	data, err := readDariYAML(content)
	if err != nil {
		return fmt.Errorf("%s: %w", label, err)
	}
	if err := ValidateManagedManifestYAML(data); err != nil {
		return fmt.Errorf("%s dari.yml: %w", label, err)
	}
	return nil
}

func readDariYAML(content []byte) ([]byte, error) {
	gzr, err := gzip.NewReader(bytes.NewReader(content))
	if err != nil {
		return nil, fmt.Errorf("read source bundle gzip: %w", err)
	}
	defer gzr.Close()
	tr := tar.NewReader(gzr)
	var dariYAML []byte
	var totalUncompressed int64
	seen := map[string]bool{}
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("read source bundle tar: %w", err)
		}
		name := path.Clean(h.Name)
		if err := validateSourceBundleEntry(name, h); err != nil {
			return nil, err
		}
		if seen[name] {
			return nil, fmt.Errorf("source bundle contains duplicate entry %q", name)
		}
		seen[name] = true
		totalUncompressed += h.Size
		if totalUncompressed > maxManagedBundleUncompressedBytes {
			return nil, fmt.Errorf("source bundle exceeds uncompressed size limit")
		}
		if name != "dari.yml" {
			continue
		}
		if h.Size > maxManagedBundleManifestFileBytes {
			return nil, fmt.Errorf("dari.yml exceeds size limit")
		}
		data, err := io.ReadAll(io.LimitReader(tr, maxManagedBundleManifestFileBytes+1))
		if err != nil {
			return nil, err
		}
		if int64(len(data)) > maxManagedBundleManifestFileBytes {
			return nil, fmt.Errorf("dari.yml exceeds size limit")
		}
		dariYAML = data
	}
	if len(dariYAML) == 0 {
		return nil, fmt.Errorf("source bundle must contain top-level dari.yml")
	}
	return dariYAML, nil
}

func validateSourceBundleEntry(name string, h *tar.Header) error {
	if strings.Contains(h.Name, "\\") || name != h.Name || name == "" || name == "." || name == ".." || strings.HasPrefix(name, "../") || path.IsAbs(name) {
		return fmt.Errorf("source bundle contains invalid entry path %q", h.Name)
	}
	if h.Typeflag != tar.TypeReg && h.Typeflag != tar.TypeRegA {
		return fmt.Errorf("source bundle contains unsupported entry %q", name)
	}
	if h.Size < 0 {
		return fmt.Errorf("source bundle entry %q has invalid size", name)
	}
	return nil
}
