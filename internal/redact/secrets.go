package redact

import (
	"encoding/json"
	"net/url"
	"sort"
	"strings"
)

const replacement = "[REDACTED]"

type Redactor struct {
	replacer *strings.Replacer
}

func NewSecrets(values map[string]string) Redactor {
	if len(values) == 0 {
		return Redactor{}
	}
	variants := map[string]bool{}
	for _, value := range values {
		if value == "" {
			continue
		}
		variants[value] = true
		if escaped := url.QueryEscape(value); escaped != value {
			variants[escaped] = true
		}
		if b, err := json.Marshal(value); err == nil && len(b) >= 2 {
			escaped := string(b[1 : len(b)-1])
			if escaped != value {
				variants[escaped] = true
			}
		}
	}
	if len(variants) == 0 {
		return Redactor{}
	}
	ordered := make([]string, 0, len(variants))
	for value := range variants {
		ordered = append(ordered, value)
	}
	sort.Slice(ordered, func(i, j int) bool {
		if len(ordered[i]) == len(ordered[j]) {
			return ordered[i] < ordered[j]
		}
		return len(ordered[i]) > len(ordered[j])
	})
	pairs := make([]string, 0, len(ordered)*2)
	for _, value := range ordered {
		pairs = append(pairs, value, replacement)
	}
	return Redactor{replacer: strings.NewReplacer(pairs...)}
}

func (r Redactor) String(s string) string {
	if r.replacer == nil || s == "" {
		return s
	}
	return r.replacer.Replace(s)
}

func (r Redactor) Bytes(b []byte) []byte {
	if r.replacer == nil || len(b) == 0 {
		return b
	}
	return []byte(r.replacer.Replace(string(b)))
}
