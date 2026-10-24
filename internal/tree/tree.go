// Package tree maps a local directory to and from a Snapshot of its files,
// keyed by "/"-rooted paths as the CodePen API uses them.
package tree

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

// Snapshot holds file contents by path, e.g. "/src/app.js".
type Snapshot map[string][]byte

// tmpSuffix marks a file that Write has not finished yet.
const tmpSuffix = ".cdpn-tmp"

var ignoredNames = map[string]bool{
	".git": true, "node_modules": true, ".env": true, ".envrc": true, ".DS_Store": true,
}

// IsIgnored reports whether path ("/a/b") is never mirrored: VCS and
// dependency folders, env files (secrets and local state) and unfinished writes.
func IsIgnored(path string) bool {
	if strings.HasSuffix(path, tmpSuffix) {
		return true
	}
	for _, part := range strings.Split(path, "/") {
		if ignoredNames[part] || strings.HasPrefix(part, ".env.") {
			return true
		}
	}
	return false
}

// Read returns every regular, non-ignored file below root. Symlinks are skipped.
func Read(root string) (Snapshot, error) {
	files := Snapshot{}
	err := filepath.WalkDir(root, func(file string, entry fs.DirEntry, err error) error {
		if err != nil || file == root {
			return err
		}
		path, err := penPath(root, file)
		if err != nil {
			return err
		}
		if IsIgnored(path) {
			return skip(entry)
		}
		if !entry.Type().IsRegular() {
			return nil
		}
		data, err := os.ReadFile(file)
		if err != nil {
			return err
		}
		files[path] = data
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("reading %q: %w", root, err)
	}
	return files, nil
}

func penPath(root, file string) (string, error) {
	rel, err := filepath.Rel(root, file)
	if err != nil {
		return "", err
	}
	return "/" + filepath.ToSlash(rel), nil
}

func skip(entry fs.DirEntry) error {
	if entry.IsDir() {
		return fs.SkipDir
	}
	return nil
}

// Write stores the contents of r at path below root, atomically: it writes a
// temporary file next to the target, then renames it into place. A path that
// would leave root (such as a remote "/../x") is refused, and all access goes
// through os.Root so symlinks cannot lead outside either.
func Write(root, path string, r io.Reader) error {
	name := filepath.FromSlash(strings.TrimPrefix(path, "/"))
	if !filepath.IsLocal(name) || filepath.Clean(name) == "." {
		return fmt.Errorf("refusing to write %q: it is outside %q", path, root)
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		return fmt.Errorf("creating %q: %w", root, err)
	}
	dir, err := os.OpenRoot(root)
	if err != nil {
		return fmt.Errorf("opening %q: %w", root, err)
	}
	defer dir.Close()
	if err := replace(dir, name, r); err != nil {
		return fmt.Errorf("writing %q: %w", path, err)
	}
	return nil
}

func replace(dir *os.Root, name string, r io.Reader) error {
	if err := dir.MkdirAll(filepath.Dir(name), 0o755); err != nil {
		return err
	}
	if err := writeTemp(dir, name+tmpSuffix, r); err != nil {
		return err
	}
	if err := dir.Rename(name+tmpSuffix, name); err != nil {
		return errors.Join(err, dir.Remove(name+tmpSuffix))
	}
	return nil
}

func writeTemp(dir *os.Root, name string, r io.Reader) error {
	f, err := dir.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	if _, err := io.Copy(f, r); err != nil {
		return errors.Join(err, f.Close(), dir.Remove(name))
	}
	if err := f.Close(); err != nil {
		return errors.Join(err, dir.Remove(name))
	}
	return nil
}

// IsText reports whether data can go to the API as text: valid UTF-8 without NUL bytes.
func IsText(data []byte) bool {
	return bytes.IndexByte(data, 0) < 0 && utf8.Valid(data)
}
