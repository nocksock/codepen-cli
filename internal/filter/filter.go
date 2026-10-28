// Package filter decides which paths sync: the built-in ignores of tree, then a
// Pen's include and exclude lists, written like .gitignore patterns.
package filter

import (
	"errors"
	"fmt"
	"path"
	"strings"

	"github.com/nocksock/codepen-cli/internal/tree"
)

// Filter holds include and exclude patterns. A nil Filter skips only what
// tree.IsIgnored does.
type Filter struct {
	include, exclude []string
}

// New checks the patterns and returns a Filter for them. With include empty,
// everything not excluded syncs.
func New(include, exclude []string) (*Filter, error) {
	for _, list := range [][]string{include, exclude} {
		for _, pattern := range list {
			if err := Check(pattern); err != nil {
				return nil, err
			}
		}
	}
	return &Filter{include: include, exclude: exclude}, nil
}

// Check reports why pattern can't be used, if it can't.
func Check(pattern string) error {
	trimmed := strings.Trim(strings.TrimSpace(pattern), "/")
	if trimmed == "" {
		return fmt.Errorf("%q is not a pattern: it matches no name", pattern)
	}
	for _, part := range strings.Split(trimmed, "/") {
		if part == "" || part == "." || part == ".." {
			return fmt.Errorf("%q is not a pattern: use plain folder names, without ., .. or //", pattern)
		}
		if _, err := path.Match(part, ""); errors.Is(err, path.ErrBadPattern) {
			return fmt.Errorf("%q is not a pattern: %q is malformed", pattern, part)
		}
	}
	return nil
}

// Skips reports whether the file at p ("/a/b.js") stays out of the sync.
func (f *Filter) Skips(p string) bool {
	if tree.IsIgnored(p) {
		return true
	}
	if f == nil {
		return false
	}
	if len(f.include) > 0 && !matchesAny(f.include, p, false) {
		return true
	}
	return matchesAny(f.exclude, p, false)
}

// SkipsDir reports whether the folder at dir stays out of the sync as a whole.
// Only excludes count: whether an include matches something inside a folder
// isn't known without looking, so the folder's files are filtered one by one.
func (f *Filter) SkipsDir(dir string) bool {
	if tree.IsIgnored(dir) {
		return true
	}
	return f != nil && matchesAny(f.exclude, dir, true)
}

func matchesAny(patterns []string, p string, isDir bool) bool {
	for _, pattern := range patterns {
		if match(pattern, p, isDir) {
			return true
		}
	}
	return false
}

// Match reports whether pattern matches the file at p, as .gitignore would: a
// pattern matching a folder matches everything in it.
func Match(pattern, p string) bool { return match(pattern, p, false) }

func match(pattern, p string, isDir bool) bool {
	pattern = strings.ToLower(strings.TrimSpace(pattern))
	isDirOnly := strings.HasSuffix(pattern, "/")
	pattern = strings.TrimSuffix(pattern, "/")
	isAnchored := strings.Contains(pattern, "/")
	parts := strings.Split(strings.TrimPrefix(pattern, "/"), "/")
	segments := strings.Split(strings.TrimPrefix(strings.ToLower(p), "/"), "/")
	// Each folder above p, and p itself, can be what the pattern names; a
	// folder-only pattern can't name a file.
	last := len(segments)
	if isDirOnly && !isDir {
		last--
	}
	for n := 1; n <= last; n++ {
		prefix := segments[:n]
		if !isAnchored {
			prefix = prefix[n-1:]
		}
		if matchSegments(parts, prefix) {
			return true
		}
	}
	return false
}

// matchSegments matches pattern parts against path segments; ** matches any
// number of segments, other parts one segment each.
func matchSegments(parts, segments []string) bool {
	if len(parts) == 0 {
		return len(segments) == 0
	}
	if parts[0] == "**" {
		return matchSegments(parts[1:], segments) || (len(segments) > 0 && matchSegments(parts, segments[1:]))
	}
	if len(segments) == 0 {
		return false
	}
	ok, _ := path.Match(parts[0], segments[0])
	return ok && matchSegments(parts[1:], segments[1:])
}
