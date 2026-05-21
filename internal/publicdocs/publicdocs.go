package publicdocs

import (
	"fmt"
	"net/url"
	"path"
	"strings"
)

func NormalizeURLs(rawURLs []string) ([]string, error) {
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

func IsLLMSTextURL(raw string) bool {
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
