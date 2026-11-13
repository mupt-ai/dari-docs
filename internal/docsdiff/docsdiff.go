// Package docsdiff finds the documentation files a change touches, and their contents on each side
// of it, from a git checkout.
package docsdiff

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"sort"
	"strings"
)

// File is one documentation path as a revision has it. Absent means the revision has no file there.
type File struct {
	Path       string
	Absent     bool
	Executable bool
	Content    []byte
}

// Snapshot is the changed documentation paths as one revision has them.
type Snapshot struct {
	Revision string
	Files    []File
}

// Change is a docs change: the changed paths before (at the merge base) and after (at head).
type Change struct {
	Base    string
	Head    string
	Before  Snapshot
	After   Snapshot
	Skipped []string
}

// Paths are the changed paths, sorted.
func (c Change) Paths() []string {
	paths := make([]string, 0, len(c.After.Files))
	for _, f := range c.After.Files {
		paths = append(paths, f.Path)
	}
	return paths
}

// Empty reports a change that touches no documentation.
func (c Change) Empty() bool { return len(c.After.Files) == 0 }

// Git runs git in one checkout.
type Git struct {
	Dir string
}

func (g Git) run(ctx context.Context, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = g.Dir
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return out, nil
}

// Resolve returns the full commit SHA rev names.
func (g Git) Resolve(ctx context.Context, rev string) (string, error) {
	out, err := g.run(ctx, "rev-parse", "--verify", "--end-of-options", rev+"^{commit}")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// Diff compares the documentation at the merge base of base and head with head's, so the two
// snapshots differ by exactly the change head makes.
func (g Git) Diff(ctx context.Context, base, head string, pathspecs []string) (Change, error) {
	headSHA, err := g.Resolve(ctx, head)
	if err != nil {
		return Change{}, err
	}
	baseSHA, err := g.Resolve(ctx, base)
	if err != nil {
		return Change{}, err
	}
	out, err := g.run(ctx, "merge-base", baseSHA, headSHA)
	if err != nil {
		return Change{}, fmt.Errorf("no merge base for %s and %s (fetch full history: actions/checkout with fetch-depth: 0): %w", base, head, err)
	}
	mergeBase := strings.TrimSpace(string(out))
	args := append([]string{"diff", "--name-only", "-z", "--no-renames", mergeBase, headSHA, "--"}, pathspecs...)
	out, err = g.run(ctx, args...)
	if err != nil {
		return Change{}, err
	}
	var paths []string
	for _, p := range strings.Split(string(out), "\x00") {
		if p != "" {
			paths = append(paths, p)
		}
	}
	sort.Strings(paths)
	change := Change{Base: mergeBase, Head: headSHA}
	change.Before.Revision = mergeBase
	change.After.Revision = headSHA
	for _, p := range paths {
		before, okBefore, err := g.file(ctx, mergeBase, p)
		if err != nil {
			return Change{}, err
		}
		after, okAfter, err := g.file(ctx, headSHA, p)
		if err != nil {
			return Change{}, err
		}
		if !okBefore || !okAfter {
			change.Skipped = append(change.Skipped, p)
			continue
		}
		change.Before.Files = append(change.Before.Files, before)
		change.After.Files = append(change.After.Files, after)
	}
	return change, nil
}

// file reads path at rev. ok is false for an entry docs checks can't carry, such as a symlink or a
// submodule.
func (g Git) file(ctx context.Context, rev, path string) (File, bool, error) {
	out, err := g.run(ctx, "ls-tree", "-z", rev, "--", path)
	if err != nil {
		return File{}, false, err
	}
	entry := strings.TrimSuffix(string(out), "\x00")
	if entry == "" {
		return File{Path: path, Absent: true}, true, nil
	}
	meta, _, _ := strings.Cut(entry, "\t")
	fields := strings.Fields(meta)
	if len(fields) != 3 || fields[1] != "blob" {
		return File{}, false, nil
	}
	mode := fields[0]
	if mode != "100644" && mode != "100755" {
		return File{}, false, nil
	}
	content, err := g.run(ctx, "cat-file", "blob", fields[2])
	if err != nil {
		return File{}, false, err
	}
	return File{Path: path, Executable: mode == "100755", Content: content}, true, nil
}
