package agentbundle

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"strings"
	"testing"
)

func TestValidateManagedManifestAllowsAnthropicRuntimeEnvelope(t *testing.T) {
	data := []byte(`
llm:
  model: anthropic/claude-sonnet-4.6
sandbox:
  secrets:
    - DARI_DOCS_RUNTIME_SECRETS_JSON
`)
	if err := ValidateManagedManifestYAML(data); err != nil {
		t.Fatal(err)
	}
}

func TestValidateManagedManifestRejectsCredentialSensitiveFields(t *testing.T) {
	tests := map[string]string{
		"non anthropic": `
llm:
  model: openai/gpt-5.5
`,
		"llm secret": `
llm:
  model: anthropic/claude-sonnet-4.6
  api_key_secret: ANTHROPIC_API_KEY
`,
		"base url": `
llm:
  model: anthropic/claude-sonnet-4.6
  base_url: https://proxy.example.test/v1
`,
		"sandbox provider secret": `
llm:
  model: anthropic/claude-sonnet-4.6
sandbox:
  provider_api_key_secret: E2B_API_KEY
`,
		"arbitrary sandbox secret": `
llm:
  model: anthropic/claude-sonnet-4.6
sandbox:
  secrets:
    - GITHUB_TOKEN
`,
	}
	for name, data := range tests {
		t.Run(name, func(t *testing.T) {
			err := ValidateManagedManifestYAML([]byte(data))
			if err == nil {
				t.Fatal("expected validation error")
			}
			if strings.TrimSpace(err.Error()) == "" {
				t.Fatal("empty validation error")
			}
		})
	}
}

func TestValidateManagedBundleRejectsUnsafeArchiveEntries(t *testing.T) {
	data := managedBundleForTest(t, []bundleFileForTest{
		{Name: "dari.yml", Content: validManagedDariYAMLForTest},
		{Name: "../evil", Content: "bad"},
	})
	err := ValidateManagedBundle(data, "tester agent")
	if err == nil || !strings.Contains(err.Error(), "invalid entry path") {
		t.Fatalf("err = %v, want invalid entry path", err)
	}
}

func TestValidateManagedBundleRejectsUnsupportedArchiveEntries(t *testing.T) {
	var body bytes.Buffer
	gz := gzip.NewWriter(&body)
	tw := tar.NewWriter(gz)
	if err := tw.WriteHeader(&tar.Header{Name: "dari.yml", Typeflag: tar.TypeSymlink, Linkname: "other.yml"}); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	err := ValidateManagedBundle(body.Bytes(), "tester agent")
	if err == nil || !strings.Contains(err.Error(), "unsupported entry") {
		t.Fatalf("err = %v, want unsupported entry", err)
	}
}

func TestValidateManagedBundleRejectsDuplicateArchiveEntries(t *testing.T) {
	data := managedBundleForTest(t, []bundleFileForTest{
		{Name: "dari.yml", Content: validManagedDariYAMLForTest},
		{Name: "dari.yml", Content: validManagedDariYAMLForTest},
	})
	err := ValidateManagedBundle(data, "tester agent")
	if err == nil || !strings.Contains(err.Error(), "duplicate entry") {
		t.Fatalf("err = %v, want duplicate entry", err)
	}
}

const validManagedDariYAMLForTest = "llm:\n  model: anthropic/claude-sonnet-4.6\n"

type bundleFileForTest struct {
	Name    string
	Content string
}

func managedBundleForTest(t *testing.T, files []bundleFileForTest) []byte {
	t.Helper()
	var body bytes.Buffer
	gz := gzip.NewWriter(&body)
	tw := tar.NewWriter(gz)
	for _, file := range files {
		if err := tw.WriteHeader(&tar.Header{Name: file.Name, Typeflag: tar.TypeReg, Mode: 0o644, Size: int64(len(file.Content))}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(file.Content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return body.Bytes()
}
