// Package ablation turns a docs snapshot into a SelfBench repo ablation: a Bash script, run in /app
// as each task's image is built, that leaves the changed docs exactly as the snapshot has them.
package ablation

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"path"
	"sort"
	"strings"
	"time"

	"github.com/mupt-ai/dari-docs/internal/docsdiff"
)

// MaxScriptBytes is the largest script SelfBench accepts for an environment.
const MaxScriptBytes = 64 * 1024

// MaxNameLength is the longest environment name SelfBench accepts.
const MaxNameLength = 60

// Script returns the script that writes the snapshot's files into /app and removes the ones it
// lacks. Its output is the same for the same snapshot, so its hash names it.
func Script(s docsdiff.Snapshot) (string, error) {
	var b strings.Builder
	// No revision in the script: the same docs make the same script, so a rerun reuses its build.
	b.WriteString("# dari-docs: sets the changed docs, nothing else\n")
	b.WriteString("set -euo pipefail\ncd /app\n")
	var remove []string
	var write []docsdiff.File
	for _, f := range s.Files {
		if err := checkPath(f.Path); err != nil {
			return "", err
		}
		if f.Absent {
			remove = append(remove, f.Path)
		} else {
			write = append(write, f)
		}
	}
	if len(remove) > 0 {
		b.WriteString("rm -f --")
		for _, p := range remove {
			b.WriteString(" " + quote(p))
		}
		b.WriteString("\n")
	}
	if len(write) > 0 {
		archive, err := tarball(write)
		if err != nil {
			return "", err
		}
		b.WriteString("printf '%s' '" + base64.StdEncoding.EncodeToString(archive) + "' | base64 -d | tar -xzf - -C /app\n")
	}
	script := b.String()
	if len(script) > MaxScriptBytes {
		return "", fmt.Errorf("the changed docs come to a %d KB script, past SelfBench's %d KB limit for a repo ablation: narrow `docs` in the config or split the change", len(script)/1024, MaxScriptBytes/1024)
	}
	return script, nil
}

// Hash is a short, stable name for a script's change.
func Hash(script string) string {
	sum := sha256.Sum256([]byte(script))
	return hex.EncodeToString(sum[:])[:10]
}

// Name is an environment's name for one side of a change: `dari-docs #12 after 1a2b3c4d5e`.
func Name(pr int, side, script string) string {
	prefix := "dari-docs"
	if pr > 0 {
		prefix = fmt.Sprintf("%s #%d", prefix, pr)
	}
	name := fmt.Sprintf("%s %s %s", prefix, side, Hash(script))
	if len(name) > MaxNameLength {
		name = name[:MaxNameLength]
	}
	return name
}

// PRPrefix is how every environment a pull request's checks made begins.
func PRPrefix(pr int) string { return fmt.Sprintf("dari-docs #%d ", pr) }

func tarball(files []docsdiff.File) ([]byte, error) {
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	var buf bytes.Buffer
	gz, err := gzip.NewWriterLevel(&buf, gzip.BestCompression)
	if err != nil {
		return nil, err
	}
	tw := tar.NewWriter(gz)
	epoch := time.Unix(0, 0).UTC()
	dirs := map[string]bool{}
	for _, f := range files {
		// Parents first, so any tar makes them.
		var parents []string
		for dir := path.Dir(f.Path); dir != "." && !dirs[dir]; dir = path.Dir(dir) {
			parents = append([]string{dir}, parents...)
			dirs[dir] = true
		}
		for _, dir := range parents {
			if err := tw.WriteHeader(&tar.Header{Typeflag: tar.TypeDir, Name: dir + "/", Mode: 0o755, ModTime: epoch, Format: tar.FormatPAX}); err != nil {
				return nil, err
			}
		}
		mode := int64(0o644)
		if f.Executable {
			mode = 0o755
		}
		if err := tw.WriteHeader(&tar.Header{Typeflag: tar.TypeReg, Name: f.Path, Mode: mode, Size: int64(len(f.Content)), ModTime: epoch, Format: tar.FormatPAX}); err != nil {
			return nil, err
		}
		if _, err := tw.Write(f.Content); err != nil {
			return nil, err
		}
	}
	if err := tw.Close(); err != nil {
		return nil, err
	}
	if err := gz.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// checkPath refuses a path that could reach outside /app.
func checkPath(p string) error {
	if p == "" || strings.HasPrefix(p, "/") || path.Clean(p) != p || p == ".." || strings.HasPrefix(p, "../") {
		return fmt.Errorf("refusing docs path %q", p)
	}
	for _, part := range strings.Split(p, "/") {
		if part == ".git" {
			return fmt.Errorf("refusing docs path %q", p)
		}
	}
	return nil
}

func quote(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }
