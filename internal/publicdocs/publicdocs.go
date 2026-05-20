package publicdocs

import (
	"fmt"
	"net/url"
	"path"
	"strings"

	"github.com/mupt-ai/dari-docs/internal/bundle"
)

const sourceFilePath = "public-docs/source.md"

type Summary struct {
	SeedURLs []string
}

func SourceFiles(rawURLs []string) ([]bundle.ExtraFile, Summary, error) {
	urls, err := normalizeURLs(rawURLs)
	if err != nil {
		return nil, Summary{}, err
	}
	if len(urls) == 0 {
		return nil, Summary{}, nil
	}
	content := publicDocsSourceMarkdown(urls)
	return []bundle.ExtraFile{{Path: sourceFilePath, Content: []byte(content), ContentType: "text/markdown"}}, Summary{SeedURLs: urls}, nil
}

func publicDocsSourceMarkdown(urls []string) string {
	var sb strings.Builder
	sb.WriteString("# Public Docs Source\n\n")
	sb.WriteString("This run uses public documentation URLs. Internet access is required.\n\n")
	sb.WriteString("Start from these URLs and choose the docs relevant to the task yourself:\n\n")
	for _, u := range urls {
		sb.WriteString("- ")
		sb.WriteString(u)
		if isLLMSTextURL(u) {
			sb.WriteString(" — llms.txt manifest; read it and follow the relevant links for the task")
		}
		sb.WriteByte('\n')
	}
	sb.WriteString("\nDo not assume this bundle contains a full copy of the public docs. Use the live URLs above as the source of truth.\n")
	return sb.String()
}

func normalizeURLs(rawURLs []string) ([]string, error) {
	out := make([]string, 0, len(rawURLs))
	seen := map[string]bool{}
	for _, raw := range rawURLs {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		u, err := url.Parse(raw)
		if err != nil || u.Scheme == "" || u.Host == "" {
			return nil, fmt.Errorf("invalid public docs URL %q", raw)
		}
		if u.Scheme != "http" && u.Scheme != "https" {
			return nil, fmt.Errorf("public docs URL must use http or https: %q", raw)
		}
		u.Fragment = ""
		key := u.String()
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, key)
	}
	return out, nil
}

func isLLMSTextURL(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil {
		return false
	}
	name := path.Base(u.Path)
	return name == "llms.txt" || name == "llms-full.txt"
}

func IsURL(raw string) bool {
	u, err := url.Parse(strings.TrimSpace(raw))
	return err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != ""
}
