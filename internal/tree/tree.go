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
	"path"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

// Snapshot holds file contents by path, e.g. "/src/app.js".
type Snapshot map[string][]byte

// tmpSuffix marks a file that Write has not finished yet.
const tmpSuffix = ".pen-tmp"

// ignoredNames are compared after foldName.
var ignoredNames = map[string]bool{
	".git": true, "node_modules": true, ".env": true, ".envrc": true, ".ds_store": true,
	".codepen.config.json": true,
}

// IsIgnored reports whether path ("/a/b") is never mirrored: VCS and
// dependency folders, env files (secrets and local state), the local
// .codepen.config.json and unfinished writes.
// Names match the way case-insensitive filesystems (macOS, Windows) resolve
// them, so "/.GIT/config" is ignored as much as "/.git/config".
func IsIgnored(path string) bool {
	if strings.HasSuffix(foldName(path), tmpSuffix) {
		return true
	}
	for _, part := range strings.Split(path, "/") {
		name := foldName(part)
		if ignoredNames[name] || strings.HasPrefix(name, ".env.") {
			return true
		}
	}
	return false
}

// foldName is the name a case-insensitive filesystem would open: lowercased,
// without the trailing dots and spaces Windows drops.
func foldName(name string) string {
	return strings.TrimRight(strings.ToLower(name), ". ")
}

// Read returns every regular, non-ignored file below root. Symlinks inside the
// tree are skipped; a symlinked root is followed. Files are read through
// os.Root, so a symlink swapped in during the walk cannot lead outside.
func Read(root string) (Snapshot, error) {
	dir, err := os.OpenRoot(root)
	if err != nil {
		return nil, fmt.Errorf("reading %q: %w", root, err)
	}
	defer dir.Close()
	files, err := readAll(dir.FS())
	if err != nil {
		return nil, fmt.Errorf("reading %q: %w", root, err)
	}
	return files, nil
}

func readAll(fsys fs.FS) (Snapshot, error) {
	files := Snapshot{}
	err := fs.WalkDir(fsys, ".", func(name string, entry fs.DirEntry, err error) error {
		if err != nil || name == "." {
			return err
		}
		if IsIgnored("/" + name) {
			return skip(entry)
		}
		if !entry.Type().IsRegular() {
			return nil
		}
		data, err := fs.ReadFile(fsys, name)
		if err != nil {
			return err
		}
		files["/"+name] = data
		return nil
	})
	return files, err
}

func skip(entry fs.DirEntry) error {
	if entry.IsDir() {
		return fs.SkipDir
	}
	return nil
}

// Write stores the contents of r at path below root, atomically: it writes a
// temporary file next to the target, then renames it into place. The target
// keeps its permission bits if it already exists. A path that would leave root
// (such as a remote "/../x") or that is ignored (such as "/.git/config") is
// refused, and all access goes through os.Root so symlinks cannot lead outside
// either.
//
// Write is not safe for concurrent writes to the same path. Names within 9
// bytes of NAME_MAX fail, because the temporary file adds a suffix.
func Write(root, path string, r io.Reader) error {
	return writeFile(root, path, r, true)
}

// WriteLocal is Write for a file that never leaves this machine, such as the
// Pen id: it allows ignored paths.
func WriteLocal(root, path string, r io.Reader) error {
	return writeFile(root, path, r, false)
}

func writeFile(root, path string, r io.Reader, isIgnoredRefused bool) error {
	name, err := cleanName(path, isIgnoredRefused)
	if err != nil {
		return fmt.Errorf("refusing to write %q: not a file path inside %q", path, root)
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

// CheckPaths reports the first remote path Write would refuse, and any two
// paths whose files or folders a case-insensitive filesystem (macOS, Windows)
// would make one. Checking a whole Pen first means a pull fails before writing
// anything.
func CheckPaths(paths []string) error {
	seen := map[string]string{} // spelling of each file and folder, by folded name
	for _, p := range paths {
		if _, err := cleanName(p, true); err != nil {
			return fmt.Errorf("refusing to write %q: not a file path inside the directory", p)
		}
		for _, name := range withFolders(p) {
			folded := foldPath(name)
			if other, ok := seen[folded]; ok && other != name {
				return fmt.Errorf("refusing to write %q and %q: they would be the same file on macOS and Windows; rename one", other, name)
			}
			seen[folded] = name
		}
	}
	return nil
}

// withFolders lists the folders leading to path, then path: "/a/b.js" gives
// "/a" and "/a/b.js".
func withFolders(path string) []string {
	var names []string
	for i := 1; i < len(path); i++ {
		if path[i] == '/' {
			names = append(names, path[:i])
		}
	}
	return append(names, path)
}

// foldPath applies foldName to every part of path.
func foldPath(path string) string {
	parts := strings.Split(path, "/")
	for i, part := range parts {
		parts[i] = foldName(part)
	}
	return strings.Join(parts, "/")
}

var errBadPath = errors.New("bad path")

// cleanName turns a remote path into a local file name below the root, or
// errBadPath if the path is not a plain file path, or is ignored while
// isIgnoredRefused.
func cleanName(p string, isIgnoredRefused bool) (string, error) {
	if !strings.HasPrefix(p, "/") || strings.HasSuffix(p, "/") ||
		strings.Contains(p, "//") || strings.ContainsAny(p, "\\\x00") {
		return "", errBadPath
	}
	clean := path.Clean(strings.TrimPrefix(p, "/")) // not rooted, so ".." stays visible
	name := filepath.FromSlash(clean)
	if !filepath.IsLocal(name) || clean == "." || (isIgnoredRefused && IsIgnored("/"+clean)) {
		return "", errBadPath
	}
	return name, nil
}

func replace(dir *os.Root, name string, r io.Reader) error {
	if err := dir.MkdirAll(filepath.Dir(name), 0o755); err != nil {
		return err
	}
	tmp := name + tmpSuffix
	perm, isKept := existingPerm(dir, name)
	if err := writeTemp(dir, tmp, perm, isKept, r); err != nil {
		return err
	}
	if err := dir.Rename(tmp, name); err != nil {
		return errors.Join(err, dir.Remove(tmp))
	}
	return nil
}

// existingPerm returns the permission bits of the regular file at name and
// true, or 0o644 and false if there is none.
func existingPerm(dir *os.Root, name string) (fs.FileMode, bool) {
	info, err := dir.Lstat(name)
	if err != nil || !info.Mode().IsRegular() {
		return 0o644, false
	}
	return info.Mode().Perm(), true
}

// writeTemp creates name exclusively, after removing any leftover, so a
// planted file or symlink there cannot redirect the write. A new file's perm
// is narrowed by the umask; a kept one is set exactly.
func writeTemp(dir *os.Root, name string, perm fs.FileMode, isKept bool, r io.Reader) error {
	if err := dir.Remove(name); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	f, err := dir.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, perm)
	if err != nil {
		return err
	}
	if isKept {
		if err := f.Chmod(perm); err != nil {
			return errors.Join(err, f.Close(), dir.Remove(name))
		}
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
