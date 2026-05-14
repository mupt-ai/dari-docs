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
	LLM *struct {
		Model        string `yaml:"model"`
		BaseURL      string `yaml:"base_url"`
		APIKeySecret string `yaml:"api_key_secret"`
	} `yaml:"llm"`
	Sandbox *struct {
		ProviderAPIKeySecret string   `yaml:"provider_api_key_secret"`
		Secrets              []string `yaml:"secrets"`
	} `yaml:"sandbox"`
}

func ValidateManagedManifestYAML(data []byte) error {
	var manifest managedManifest
	if err := yaml.Unmarshal(data, &manifest); err != nil {
		return fmt.Errorf("parse dari.yml: %w", err)
	}
	if manifest.LLM == nil {
		return fmt.Errorf("llm is required")
	}
	if !strings.HasPrefix(strings.TrimSpace(manifest.LLM.Model), "anthropic/") {
		return fmt.Errorf("llm.model must start with anthropic/")
	}
	if strings.TrimSpace(manifest.LLM.APIKeySecret) != "" {
		return fmt.Errorf("llm.api_key_secret must be omitted for managed Dari Docs agents")
	}
	if strings.TrimSpace(manifest.LLM.BaseURL) != "" {
		return fmt.Errorf("llm.base_url must be omitted for managed Dari Docs agents")
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
