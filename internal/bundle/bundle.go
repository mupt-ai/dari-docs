package bundle

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

const (
	DefaultMaxUncompressedBytes int64 = 100 * 1024 * 1024
	DefaultMaxFileBytes         int64 = 5 * 1024 * 1024
)

type FileRecord struct {
	Path        string `json:"path"`
	SizeBytes   int64  `json:"size_bytes"`
	SHA256      string `json:"sha256"`
	ContentType string `json:"content_type,omitempty"`
}

type Manifest struct {
	SchemaVersion int          `json:"schema_version"`
	CreatedAt     string       `json:"created_at"`
	RepoRoot      string       `json:"repo_root"`
	Files         []FileRecord `json:"files"`
}

type Result struct {
	Path         string
	Manifest     Manifest
	SHA256       string
	Bytes        int64
	MaxFileBytes int64
	Skipped      SkipSummary
}

type CreateOptions struct {
	Include      []string
	Exclude      []string
	MaxFileBytes int64
}

type SkipSummary struct {
	IgnoredDirs      int
	UnsupportedFiles int
	ExcludedFiles    int
	OversizedFiles   []SkippedFile
}

type SkippedFile struct {
	Path      string
	SizeBytes int64
}

type ReadOptions struct {
	MaxUncompressedBytes int64
	MaxFileBytes         int64
}

var defaultSkipDirs = map[string]bool{
	".git": true, "node_modules": true, ".dari-docs": true, ".next": true,
	"dist": true, "build": true, "coverage": true, ".turbo": true,
}

var defaultExts = map[string]bool{
	".md": true, ".mdx": true, ".txt": true, ".json": true, ".yml": true, ".yaml": true,
	".toml": true, ".css": true, ".js": true, ".jsx": true, ".ts": true, ".tsx": true,
}

var defaultNames = map[string]bool{
	"mint.json": true, "docs.json": true, "openapi.json": true, "openapi.yaml": true,
	"README": true, "README.md": true, "llms.txt": true, "llms-full.txt": true,
}

type compiledPattern struct {
	raw      string
	rx       *regexp.Regexp
	basename bool
	prefix   string
}

func Create(repoRoot, outPath string) (Result, error) {
	return CreateWithOptions(repoRoot, outPath, CreateOptions{})
}

func CreateWithOptions(repoRoot, outPath string, opts CreateOptions) (Result, error) {
	opts, include, exclude, err := normalizeCreateOptions(opts)
	if err != nil {
		return Result{}, err
	}
	absRoot, err := filepath.Abs(repoRoot)
	if err != nil {
		return Result{}, err
	}
	var files []FileRecord
	var skipped SkipSummary
	if err := filepath.WalkDir(absRoot, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		name := d.Name()
		rel, err := filepath.Rel(absRoot, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if rel == "." {
			rel = ""
		}
		if d.IsDir() {
			if path != absRoot && defaultSkipDirs[name] {
				skipped.IgnoredDirs++
				return filepath.SkipDir
			}
			if path != absRoot && matchesAny(exclude, rel) {
				skipped.ExcludedFiles++
				return filepath.SkipDir
			}
			return nil
		}
		if matchesAny(exclude, rel) {
			skipped.ExcludedFiles++
			return nil
		}
		if strings.HasPrefix(rel, "../") || strings.HasPrefix(rel, "/") {
			return nil
		}
		if d.Type()&os.ModeSymlink != 0 {
			skipped.UnsupportedFiles++
			return nil
		}
		if !looksLikeDocsFile(name) && !matchesAny(include, rel) {
			skipped.UnsupportedFiles++
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			skipped.UnsupportedFiles++
			return nil
		}
		if info.Size() > opts.MaxFileBytes {
			skipped.OversizedFiles = append(skipped.OversizedFiles, SkippedFile{Path: rel, SizeBytes: info.Size()})
			return nil
		}
		h, err := fileSHA256(path)
		if err != nil {
			return err
		}
		files = append(files, FileRecord{Path: rel, SizeBytes: info.Size(), SHA256: h, ContentType: contentType(name)})
		return nil
	}); err != nil {
		return Result{}, err
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	manifest := Manifest{SchemaVersion: 1, CreatedAt: time.Now().UTC().Format(time.RFC3339), RepoRoot: filepath.Base(absRoot), Files: files}
	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		return Result{}, err
	}
	out, err := os.Create(outPath)
	if err != nil {
		return Result{}, err
	}
	defer out.Close()
	hash := sha256.New()
	mw := io.MultiWriter(out, hash)
	gz := gzip.NewWriter(mw)
	tw := tar.NewWriter(gz)
	if err := writeManifest(tw, manifest); err != nil {
		return Result{}, err
	}
	for _, rec := range files {
		if err := writeFile(tw, absRoot, rec.Path); err != nil {
			return Result{}, err
		}
	}
	if err := tw.Close(); err != nil {
		return Result{}, err
	}
	if err := gz.Close(); err != nil {
		return Result{}, err
	}
	if err := out.Close(); err != nil {
		return Result{}, err
	}
	info, err := os.Stat(outPath)
	if err != nil {
		return Result{}, err
	}
	sort.Slice(skipped.OversizedFiles, func(i, j int) bool { return skipped.OversizedFiles[i].Path < skipped.OversizedFiles[j].Path })
	return Result{Path: outPath, Manifest: manifest, SHA256: hex.EncodeToString(hash.Sum(nil)), Bytes: info.Size(), MaxFileBytes: opts.MaxFileBytes, Skipped: skipped}, nil
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
		cp := compiledPattern{raw: pattern, basename: !strings.Contains(pattern, "/")}
		if strings.HasSuffix(pattern, "/**") {
			cp.prefix = strings.TrimSuffix(pattern, "/**")
		}
		rx, err := regexp.Compile("^" + globRegexp(pattern) + "$")
		if err != nil {
			return nil, fmt.Errorf("invalid bundle glob %q: %w", pattern, err)
		}
		cp.rx = rx
		compiled = append(compiled, cp)
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

func globRegexp(pattern string) string {
	var sb strings.Builder
	for i := 0; i < len(pattern); i++ {
		ch := pattern[i]
		switch ch {
		case '*':
			if i+1 < len(pattern) && pattern[i+1] == '*' {
				sb.WriteString(".*")
				i++
			} else {
				sb.WriteString("[^/]*")
			}
		case '?':
			sb.WriteString("[^/]")
		default:
			sb.WriteString(regexp.QuoteMeta(string(ch)))
		}
	}
	return sb.String()
}

func matchesAny(patterns []compiledPattern, rel string) bool {
	rel = strings.TrimPrefix(filepath.ToSlash(rel), "./")
	for _, pattern := range patterns {
		if pattern.prefix != "" && (rel == pattern.prefix || strings.HasPrefix(rel, pattern.prefix+"/")) {
			return true
		}
		target := rel
		if pattern.basename {
			target = path.Base(rel)
		}
		if pattern.rx.MatchString(target) {
			return true
		}
	}
	return false
}

func WriteSummary(w io.Writer, r Result) {
	fmt.Fprintf(w, "Built docs bundle: %s\n", r.Path)
	fmt.Fprintf(w, "  Included: %s, %s compressed\n", countPhrase(len(r.Manifest.Files), "file", "files"), formatBytes(r.Bytes))
	parts := []string{}
	if r.Skipped.IgnoredDirs > 0 {
		parts = append(parts, countPhrase(r.Skipped.IgnoredDirs, "ignored directory", "ignored directories"))
	}
	if r.Skipped.UnsupportedFiles > 0 {
		parts = append(parts, countPhrase(r.Skipped.UnsupportedFiles, "unsupported file", "unsupported files"))
	}
	if r.Skipped.ExcludedFiles > 0 {
		parts = append(parts, countPhrase(r.Skipped.ExcludedFiles, "excluded path", "excluded paths"))
	}
	if len(r.Skipped.OversizedFiles) > 0 {
		maxFileBytes := r.MaxFileBytes
		if maxFileBytes <= 0 {
			maxFileBytes = DefaultMaxFileBytes
		}
		parts = append(parts, fmt.Sprintf("%s over the %s per-file limit", countPhrase(len(r.Skipped.OversizedFiles), "file", "files"), formatBytes(maxFileBytes)))
	}
	if len(parts) > 0 {
		fmt.Fprintf(w, "  Skipped: %s\n", strings.Join(parts, ", "))
	}
	if len(r.Skipped.OversizedFiles) > 0 {
		fmt.Fprintln(w, "  Large skipped files were not uploaded:")
		limit := len(r.Skipped.OversizedFiles)
		if limit > 10 {
			limit = 10
		}
		for _, skipped := range r.Skipped.OversizedFiles[:limit] {
			fmt.Fprintf(w, "    %s (%s)\n", skipped.Path, formatBytes(skipped.SizeBytes))
		}
		if extra := len(r.Skipped.OversizedFiles) - limit; extra > 0 {
			fmt.Fprintf(w, "    ... and %d more\n", extra)
		}
		fmt.Fprintln(w, "  Split or reduce these files if agents need them; include patterns do not bypass the per-file limit.")
	}
}

func countPhrase(n int, singular, plural string) string {
	if n == 1 {
		return fmt.Sprintf("1 %s", singular)
	}
	return fmt.Sprintf("%d %s", n, plural)
}

func formatBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for value := n / unit; value >= unit; value /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(n)/float64(div), "KMGTPE"[exp])
}

func Read(path string) (Result, error) {
	return ReadWithOptions(path, ReadOptions{})
}

func ReadWithOptions(path string, opts ReadOptions) (Result, error) {
	opts = normalizeReadOptions(opts)
	info, err := os.Stat(path)
	if err != nil {
		return Result{}, err
	}
	sum, err := fileSHA256(path)
	if err != nil {
		return Result{}, err
	}
	f, err := os.Open(path)
	if err != nil {
		return Result{}, err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return Result{}, err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	var manifest Manifest
	foundManifest := false
	totalUncompressed := int64(0)
	entries := map[string]FileRecord{}
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return Result{}, err
		}
		if strings.Contains(hdr.Name, "\\") {
			return Result{}, fmt.Errorf("bundle contains invalid tar entry path %q", hdr.Name)
		}
		name := filepath.ToSlash(hdr.Name)
		if err := validateArchiveEntryName(name); err != nil {
			return Result{}, err
		}
		if hdr.Typeflag != tar.TypeReg && hdr.Typeflag != tar.TypeRegA {
			return Result{}, fmt.Errorf("bundle contains unsupported tar entry %q", name)
		}
		if hdr.Size < 0 {
			return Result{}, fmt.Errorf("bundle entry %q has invalid size", name)
		}
		if strings.HasPrefix(name, "files/") && opts.MaxFileBytes > 0 && hdr.Size > opts.MaxFileBytes {
			return Result{}, fmt.Errorf("bundle entry %q exceeds file size limit", name)
		}
		totalUncompressed += hdr.Size
		if opts.MaxUncompressedBytes > 0 && totalUncompressed > opts.MaxUncompressedBytes {
			return Result{}, fmt.Errorf("bundle exceeds uncompressed size limit")
		}
		if name == "manifest.json" {
			if foundManifest {
				return Result{}, fmt.Errorf("bundle contains duplicate manifest.json")
			}
			manifestBytes, err := io.ReadAll(tr)
			if err != nil {
				return Result{}, err
			}
			if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
				return Result{}, fmt.Errorf("decode manifest.json: %w", err)
			}
			foundManifest = true
			continue
		}
		rel := strings.TrimPrefix(name, "files/")
		if err := validateManifestPath(rel); err != nil {
			return Result{}, err
		}
		if _, exists := entries[rel]; exists {
			return Result{}, fmt.Errorf("bundle contains duplicate file %q", rel)
		}
		h := sha256.New()
		n, err := io.Copy(h, tr)
		if err != nil {
			return Result{}, err
		}
		entries[rel] = FileRecord{Path: rel, SizeBytes: n, SHA256: hex.EncodeToString(h.Sum(nil)), ContentType: contentType(rel)}
	}
	if !foundManifest {
		return Result{}, fmt.Errorf("bundle missing manifest.json")
	}
	if err := validateManifest(manifest, entries, opts); err != nil {
		return Result{}, err
	}
	return Result{Path: path, Manifest: manifest, SHA256: sum, Bytes: info.Size(), MaxFileBytes: opts.MaxFileBytes}, nil
}

func normalizeReadOptions(opts ReadOptions) ReadOptions {
	if opts.MaxUncompressedBytes <= 0 {
		opts.MaxUncompressedBytes = DefaultMaxUncompressedBytes
	}
	if opts.MaxFileBytes <= 0 {
		opts.MaxFileBytes = DefaultMaxFileBytes
	}
	return opts
}

func validateArchiveEntryName(name string) error {
	if name == "" || strings.Contains(name, "\\") || path.IsAbs(name) || path.Clean(name) != name {
		return fmt.Errorf("bundle contains invalid tar entry path %q", name)
	}
	if name == "." || name == ".." || strings.HasPrefix(name, "../") {
		return fmt.Errorf("bundle contains invalid tar entry path %q", name)
	}
	if name != "manifest.json" && !strings.HasPrefix(name, "files/") {
		return fmt.Errorf("bundle contains unexpected tar entry %q", name)
	}
	return nil
}

func validateManifestPath(p string) error {
	if p == "" || strings.Contains(p, "\\") || path.IsAbs(p) || path.Clean(p) != p {
		return fmt.Errorf("bundle contains invalid file path %q", p)
	}
	if p == "." || p == ".." || strings.HasPrefix(p, "../") {
		return fmt.Errorf("bundle contains invalid file path %q", p)
	}
	return nil
}

// ValidateRelativePath checks whether p is safe as a repo-relative bundle path.
func ValidateRelativePath(p string) error {
	return validateManifestPath(p)
}

func validateManifest(manifest Manifest, entries map[string]FileRecord, opts ReadOptions) error {
	if manifest.SchemaVersion != 1 {
		return fmt.Errorf("bundle manifest has unsupported schema_version %d", manifest.SchemaVersion)
	}
	seen := map[string]bool{}
	for _, rec := range manifest.Files {
		if err := validateManifestPath(rec.Path); err != nil {
			return err
		}
		if seen[rec.Path] {
			return fmt.Errorf("bundle manifest contains duplicate file %q", rec.Path)
		}
		seen[rec.Path] = true
		if rec.SizeBytes < 0 {
			return fmt.Errorf("bundle manifest file %q has invalid size", rec.Path)
		}
		if opts.MaxFileBytes > 0 && rec.SizeBytes > opts.MaxFileBytes {
			return fmt.Errorf("bundle manifest file %q exceeds file size limit", rec.Path)
		}
		if len(rec.SHA256) != sha256.Size*2 {
			return fmt.Errorf("bundle manifest file %q has invalid sha256", rec.Path)
		}
		if _, err := hex.DecodeString(rec.SHA256); err != nil {
			return fmt.Errorf("bundle manifest file %q has invalid sha256", rec.Path)
		}
		entry, ok := entries[rec.Path]
		if !ok {
			return fmt.Errorf("bundle missing file %q", rec.Path)
		}
		if entry.SizeBytes != rec.SizeBytes {
			return fmt.Errorf("bundle file %q size does not match manifest", rec.Path)
		}
		if entry.SHA256 != rec.SHA256 {
			return fmt.Errorf("bundle file %q sha256 does not match manifest", rec.Path)
		}
	}
	for p := range entries {
		if !seen[p] {
			return fmt.Errorf("bundle contains file %q not listed in manifest", p)
		}
	}
	return nil
}

func looksLikeDocsFile(name string) bool {
	if defaultNames[name] {
		return true
	}
	return defaultExts[strings.ToLower(filepath.Ext(name))]
}

func contentType(name string) string {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".md", ".mdx":
		return "text/markdown"
	case ".json":
		return "application/json"
	case ".yml", ".yaml", ".toml":
		return "text/plain"
	default:
		return "text/plain"
	}
}

func fileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func writeManifest(tw *tar.Writer, manifest Manifest) error {
	b, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')
	hdr := &tar.Header{Name: "manifest.json", Mode: 0o644, Size: int64(len(b)), ModTime: time.Now()}
	if err := tw.WriteHeader(hdr); err != nil {
		return err
	}
	_, err = tw.Write(b)
	return err
}

func writeFile(tw *tar.Writer, root, rel string) error {
	path := filepath.Join(root, filepath.FromSlash(rel))
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("refusing to bundle non-regular file %q", rel)
	}
	hdr, err := tar.FileInfoHeader(info, "")
	if err != nil {
		return err
	}
	hdr.Name = "files/" + rel
	hdr.Mode = 0o644
	if err := tw.WriteHeader(hdr); err != nil {
		return err
	}
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := io.Copy(tw, f); err != nil {
		return fmt.Errorf("copy %s: %w", rel, err)
	}
	return nil
}
