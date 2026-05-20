package bundle

import (
	"fmt"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
)

const (
	SkipReasonInvalidPath     = "invalid_path"
	SkipReasonDuplicate       = "duplicate"
	SkipReasonIgnoredDir      = "ignored_directory"
	SkipReasonExcluded        = "excluded"
	SkipReasonUnsupportedFile = "unsupported"
	SkipReasonOversized       = "oversized"
)

type CandidateFile struct {
	Path      string
	SizeBytes int64
}

type SelectedFile struct {
	Path        string `json:"path"`
	SizeBytes   int64  `json:"size_bytes"`
	ContentType string `json:"content_type,omitempty"`
}

type SkippedCandidateFile struct {
	Path      string `json:"path"`
	SizeBytes int64  `json:"size_bytes"`
	Reason    string `json:"reason"`
}

type SelectionResult struct {
	Selected      []SelectedFile         `json:"selected"`
	Skipped       []SkippedCandidateFile `json:"skipped"`
	SelectedBytes int64                  `json:"selected_bytes"`
}

type SelectionDefaults struct {
	SkipDirs   []string `json:"skip_dirs"`
	Extensions []string `json:"extensions"`
	Names      []string `json:"names"`
}

type compiledPattern struct {
	raw      string
	basename bool
}

func DefaultSelectionDefaults() SelectionDefaults {
	return SelectionDefaults{
		SkipDirs:   sortedBoolMapKeys(defaultSkipDirs),
		Extensions: sortedBoolMapKeys(defaultExts),
		Names:      sortedBoolMapKeys(defaultNames),
	}
}

func SelectFiles(candidates []CandidateFile, opts CreateOptions) (SelectionResult, error) {
	opts, include, exclude, err := normalizeCreateOptions(opts)
	if err != nil {
		return SelectionResult{}, err
	}
	return selectFilesWithPatterns(candidates, opts, include, exclude), nil
}

func normalizeCreateOptions(opts CreateOptions) (CreateOptions, []compiledPattern, []compiledPattern, error) {
	if opts.MaxFileBytes <= 0 {
		opts.MaxFileBytes = DefaultMaxFileBytes
	}
	include, err := compilePatterns(opts.Include)
	if err != nil {
		return CreateOptions{}, nil, nil, err
	}
	exclude, err := compilePatterns(opts.Exclude)
	if err != nil {
		return CreateOptions{}, nil, nil, err
	}
	return opts, include, exclude, nil
}

func compilePatterns(patterns []string) ([]compiledPattern, error) {
	compiled := make([]compiledPattern, 0, len(patterns))
	for _, pattern := range patterns {
		pattern = normalizePattern(pattern)
		if pattern == "" {
			continue
		}
		if !doublestar.ValidatePattern(pattern) {
			return nil, fmt.Errorf("invalid bundle glob %q", pattern)
		}
		compiled = append(compiled, compiledPattern{
			raw:      pattern,
			basename: !strings.Contains(pattern, "/"),
		})
	}
	return compiled, nil
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

func selectFilesWithPatterns(candidates []CandidateFile, opts CreateOptions, include, exclude []compiledPattern) SelectionResult {
	var result SelectionResult
	seen := map[string]bool{}
	for _, candidate := range candidates {
		rel := normalizeCandidatePath(candidate.Path)
		if rel == "" || ValidateRelativePath(rel) != nil || candidate.SizeBytes < 0 {
			result.Skipped = append(result.Skipped, SkippedCandidateFile{
				Path:      candidate.Path,
				SizeBytes: candidate.SizeBytes,
				Reason:    SkipReasonInvalidPath,
			})
			continue
		}
		if seen[rel] {
			result.Skipped = append(result.Skipped, SkippedCandidateFile{
				Path:      rel,
				SizeBytes: candidate.SizeBytes,
				Reason:    SkipReasonDuplicate,
			})
			continue
		}
		seen[rel] = true
		if hasDefaultSkipDir(rel) {
			result.Skipped = append(result.Skipped, SkippedCandidateFile{
				Path:      rel,
				SizeBytes: candidate.SizeBytes,
				Reason:    SkipReasonIgnoredDir,
			})
			continue
		}
		if matchesExclude(exclude, rel) {
			result.Skipped = append(result.Skipped, SkippedCandidateFile{
				Path:      rel,
				SizeBytes: candidate.SizeBytes,
				Reason:    SkipReasonExcluded,
			})
			continue
		}
		if !looksLikeDocsFile(path.Base(rel)) && !matchesInclude(include, rel) {
			result.Skipped = append(result.Skipped, SkippedCandidateFile{
				Path:      rel,
				SizeBytes: candidate.SizeBytes,
				Reason:    SkipReasonUnsupportedFile,
			})
			continue
		}
		if candidate.SizeBytes > opts.MaxFileBytes {
			result.Skipped = append(result.Skipped, SkippedCandidateFile{
				Path:      rel,
				SizeBytes: candidate.SizeBytes,
				Reason:    SkipReasonOversized,
			})
			continue
		}
		result.Selected = append(result.Selected, SelectedFile{
			Path:        rel,
			SizeBytes:   candidate.SizeBytes,
			ContentType: contentType(rel),
		})
		result.SelectedBytes += candidate.SizeBytes
	}
	sort.Slice(result.Selected, func(i, j int) bool { return result.Selected[i].Path < result.Selected[j].Path })
	sort.Slice(result.Skipped, func(i, j int) bool {
		if result.Skipped[i].Path == result.Skipped[j].Path {
			return result.Skipped[i].Reason < result.Skipped[j].Reason
		}
		return result.Skipped[i].Path < result.Skipped[j].Path
	})
	return result
}

func normalizeCandidatePath(raw string) string {
	rel := strings.TrimSpace(filepath.ToSlash(raw))
	rel = strings.TrimPrefix(rel, "./")
	rel = strings.TrimPrefix(rel, "/")
	if rel == "" {
		return ""
	}
	return path.Clean(rel)
}

func hasDefaultSkipDir(rel string) bool {
	for _, segment := range strings.Split(rel, "/") {
		if defaultSkipDirs[segment] {
			return true
		}
	}
	return false
}

func shouldSkipDirectory(name, rel string, exclude []compiledPattern) (bool, string) {
	if defaultSkipDirs[name] {
		return true, SkipReasonIgnoredDir
	}
	if matchesExclude(exclude, rel) {
		return true, SkipReasonExcluded
	}
	return false, ""
}

func matchesInclude(patterns []compiledPattern, rel string) bool {
	rel = normalizeCandidatePath(rel)
	for _, pattern := range patterns {
		target := rel
		if pattern.basename {
			target = path.Base(rel)
		}
		if matchCompiledPattern(pattern, target) {
			return true
		}
	}
	return false
}

func matchesExclude(patterns []compiledPattern, rel string) bool {
	rel = normalizeCandidatePath(rel)
	if rel == "" {
		return false
	}
	for _, pattern := range patterns {
		if pattern.basename {
			for _, segment := range strings.Split(rel, "/") {
				if matchCompiledPattern(pattern, segment) {
					return true
				}
			}
			continue
		}
		if matchCompiledPattern(pattern, rel) {
			return true
		}
		for _, ancestor := range ancestors(rel) {
			if matchCompiledPattern(pattern, ancestor) {
				return true
			}
		}
	}
	return false
}

func matchCompiledPattern(pattern compiledPattern, target string) bool {
	ok, err := doublestar.Match(pattern.raw, target)
	return err == nil && ok
}

func ancestors(rel string) []string {
	var out []string
	for dir := path.Dir(rel); dir != "." && dir != "/" && dir != ""; dir = path.Dir(dir) {
		out = append(out, dir)
	}
	return out
}

func sortedBoolMapKeys(values map[string]bool) []string {
	out := make([]string, 0, len(values))
	for value := range values {
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}
