package tree

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestIsIgnored(t *testing.T) {
	t.Parallel()
	tests := []struct {
		path     string
		expected bool
	}{
		{path: "/index.html", expected: false},
		{path: "/src/app.js", expected: false},
		{path: "/.git/config", expected: true},
		{path: "/node_modules/dep/index.js", expected: true},
		{path: "/.env", expected: true},
		{path: "/.env.production", expected: true},
		{path: "/.envrc", expected: true},
		{path: "/img/.DS_Store", expected: true},
		{path: "/a.js.cdpn-tmp", expected: true},
		{path: "/.environment.js", expected: false},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			t.Parallel()
			if got := IsIgnored(tt.path); got != tt.expected {
				t.Errorf("IsIgnored(%q) = %v, want %v", tt.path, got, tt.expected)
			}
		})
	}
}

func TestRead(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	write(t, root, "index.html", "<h1>x</h1>")
	write(t, root, "src/app.js", "1")
	write(t, root, "empty.js", "")
	write(t, root, "node_modules/dep/index.js", "x")
	write(t, root, ".env.local", "CODEPEN_ID=x")
	if err := os.Symlink(filepath.Join(root, "index.html"), filepath.Join(root, "link.html")); err != nil {
		t.Fatal(err)
	}

	got, err := Read(root)
	if err != nil {
		t.Fatal(err)
	}
	expected := map[string]string{"/index.html": "<h1>x</h1>", "/src/app.js": "1", "/empty.js": ""}
	if len(got) != len(expected) {
		t.Errorf("Read() has %d files, want %d: %v", len(got), len(expected), keys(got))
	}
	for path, content := range expected {
		if string(got[path]) != content {
			t.Errorf("Read()[%q] = %q, want %q", path, got[path], content)
		}
	}
}

func TestRead_MissingRoot(t *testing.T) {
	t.Parallel()
	_, err := Read(filepath.Join(t.TempDir(), "missing"))
	if !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("Read(missing) error = %v, want fs.ErrNotExist", err)
	}
}

func TestWrite(t *testing.T) {
	t.Parallel()
	root := filepath.Join(t.TempDir(), "work")
	if err := Write(root, "/src/deep/app.js", strings.NewReader("1")); err != nil {
		t.Fatal(err)
	}
	if err := Write(root, "/src/deep/app.js", strings.NewReader("2")); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(root, "src", "deep", "app.js"))
	if err != nil || string(data) != "2" {
		t.Fatalf("file = %q, %v; want \"2\"", data, err)
	}
	if _, err := os.Stat(filepath.Join(root, "src", "deep", "app.js"+tmpSuffix)); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("temporary file left behind: %v", err)
	}
}

func TestWrite_TopLevelFile(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	if err := Write(root, "/index.html", strings.NewReader("x")); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(root, "index.html"))
	if err != nil || string(data) != "x" {
		t.Fatalf("file = %q, %v; want \"x\"", data, err)
	}
}

func TestWrite_RefusesPathsOutsideRoot(t *testing.T) {
	t.Parallel()
	parent := t.TempDir()
	root := filepath.Join(parent, "work")
	for _, path := range []string{"/../escape.txt", "/a/../../escape.txt"} {
		err := Write(root, path, strings.NewReader("x"))
		if err == nil || !strings.Contains(err.Error(), "refusing to write") {
			t.Errorf("Write(%q) error = %v, want refusal", path, err)
		}
	}
	if _, err := os.Stat(filepath.Join(parent, "escape.txt")); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("escape.txt was written outside the root")
	}
}

func TestWrite_RefusesEmptyPaths(t *testing.T) {
	t.Parallel()
	for _, path := range []string{"", "/", "/."} {
		if err := Write(t.TempDir(), path, strings.NewReader("x")); err == nil {
			t.Errorf("Write(%q) succeeded, want an error", path)
		}
	}
}

func TestWrite_DoesNotFollowSymlinkOutOfRoot(t *testing.T) {
	t.Parallel()
	outside := t.TempDir()
	root := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(root, "out")); err != nil {
		t.Fatal(err)
	}
	if err := Write(root, "/out/escape.txt", strings.NewReader("x")); err == nil {
		t.Error("Write through a symlink leaving the root succeeded, want an error")
	}
	if _, err := os.Stat(filepath.Join(outside, "escape.txt")); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("escape.txt was written outside the root: %v", err)
	}
}

func TestWrite_CleansUpWhenRenameFails(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	write(t, root, "dir/file", "x")
	if err := Write(root, "/dir", strings.NewReader("y")); err == nil {
		t.Error("Write over a directory succeeded, want an error")
	}
	if _, err := os.Stat(filepath.Join(root, "dir"+tmpSuffix)); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("temporary file left behind: %v", err)
	}
}

// The next two tests pin down os.Root behaviour that Write relies on.
func TestRoot_MkdirAllDotIsNoop(t *testing.T) {
	t.Parallel()
	dir, err := os.OpenRoot(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer dir.Close()
	if err := dir.MkdirAll(".", 0o755); err != nil {
		t.Errorf("Root.MkdirAll(\".\") = %v, want nil", err)
	}
}

func TestRoot_RenameReplacesExistingFile(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	write(t, root, "a", "old")
	write(t, root, "b", "new")
	dir, err := os.OpenRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	defer dir.Close()
	if err := dir.Rename("b", "a"); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(root, "a"))
	if err != nil || string(data) != "new" {
		t.Errorf("a = %q, %v; want \"new\"", data, err)
	}
}

func TestIsText(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		input    []byte
		expected bool
	}{
		{name: "ascii", input: []byte("hello"), expected: true},
		{name: "utf-8", input: []byte("grüße ✓"), expected: true},
		{name: "nul byte", input: []byte("a\x00b"), expected: false},
		{name: "invalid utf-8", input: []byte{0xff, 0xfe}, expected: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := IsText(tt.input); got != tt.expected {
				t.Errorf("IsText(%q) = %v, want %v", tt.input, got, tt.expected)
			}
		})
	}
}

func write(t *testing.T, root, path, content string) {
	t.Helper()
	file := filepath.Join(root, filepath.FromSlash(path))
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func keys(s Snapshot) []string {
	out := []string{}
	for k := range s {
		out = append(out, k)
	}
	return out
}
