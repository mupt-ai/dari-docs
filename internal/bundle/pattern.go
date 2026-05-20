package bundle

import (
	"fmt"
	"path"
	"path/filepath"
	"strings"
)

func normalizePatterns(patterns []string) ([]string, error) {
	normalized := make([]string, 0, len(patterns))
	for _, pattern := range patterns {
		pattern = normalizePattern(pattern)
		if pattern == "" {
			continue
		}
		if _, err := path.Match(pattern, ""); err != nil {
			return nil, fmt.Errorf("invalid bundle glob %q: %w", pattern, err)
		}
		normalized = append(normalized, pattern)
	}
	return normalized, nil
}

func normalizePattern(pattern string) string {
	pattern = strings.TrimSpace(filepath.ToSlash(pattern))
	pattern = strings.TrimPrefix(pattern, "./")
	pattern = strings.TrimPrefix(pattern, "/")
	if pattern == "" {
		return ""
	}
	pattern = path.Clean(pattern)
	if pattern == "." {
		return ""
	}
	return pattern
}

func matchesAny(patterns []string, rel string) bool {
	rel = strings.TrimPrefix(filepath.ToSlash(rel), "./")
	for _, pattern := range patterns {
		target := rel
		if !strings.Contains(pattern, "/") {
			target = path.Base(rel)
		}
		if matchPattern(pattern, target) {
			return true
		}
	}
	return false
}

func matchPattern(pattern, name string) bool {
	if !strings.Contains(pattern, "**") {
		matched, _ := path.Match(pattern, name)
		return matched
	}
	return matchDoubleStarPattern(strings.Split(pattern, "/"), strings.Split(name, "/"))
}

func matchDoubleStarPattern(patternParts, nameParts []string) bool {
	for len(patternParts) > 0 {
		part := patternParts[0]
		patternParts = patternParts[1:]
		if part == "**" {
			for i := 0; i <= len(nameParts); i++ {
				if matchDoubleStarPattern(patternParts, nameParts[i:]) {
					return true
				}
			}
			return false
		}
		if len(nameParts) == 0 {
			return false
		}
		matched, _ := path.Match(part, nameParts[0])
		if !matched {
			return false
		}
		nameParts = nameParts[1:]
	}
	return len(nameParts) == 0
}
