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
		{path: "/a.js.pen-tmp", expected: true},
		{path: "/.codepen.config.json", expected: true},
		{path: "/.codepen/pen.config.json", expected: false},
		{path: "/.environment.js", expected: false},
		// Case-insensitive filesystems (macOS, Windows) resolve these to the real names.
		{path: "/.GIT/config", expected: true},
		{path: "/sub/.Git/hooks/pre-commit", expected: true},
		{path: "/.Env", expected: true},
		{path: "/.ENV.local", expected: true},
		{path: "/Node_Modules/dep.js", expected: true},
		// Windows drops trailing dots and spaces from names.
		{path: "/.git./config", expected: true},
		{path: "/.env ", expected: true},
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

func TestRead_SymlinkedRoot(t *testing.T) {
	t.Parallel()
	target := t.TempDir()
	write(t, target, "src/app.js", "1")
	link := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	got, err := Read(link)
	if err != nil {
		t.Fatal(err)
	}
	if string(got["/src/app.js"]) != "1" || len(got) != 1 {
		t.Errorf("Read(symlinked root) = %v, want only /src/app.js", keys(got))
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

func TestWrite_RefusesBadPaths(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		path string
	}{
		{name: "parent", path: "/../escape.txt"},
		{name: "nested parent", path: "/a/../../escape.txt"},
		{name: "double slash", path: "//etc/x"},
		{name: "only parent", path: "/.."},
		{name: "nul byte", path: "/a\x00b"},
		{name: "backslash", path: "/a\\b"},
		{name: "empty", path: ""},
		{name: "root", path: "/"},
		{name: "dot", path: "/."},
		{name: "trailing slash", path: "/a/"},
		{name: "no leading slash", path: "a.txt"},
		{name: "git hook", path: "/.git/hooks/pre-commit"},
		{name: "git config", path: "/.git/config"},
		{name: "git config in other case", path: "/.GIT/config"},
		{name: "env in other case", path: "/.Env"},
		{name: "env", path: "/.env"},
		{name: "envrc", path: "/.envrc"},
		{name: "env variant", path: "/.env.production"},
		{name: "node modules", path: "/node_modules/x.js"},
		{name: "ignored after cleaning", path: "/a/../.git/config"},
		{name: "temp file", path: "/a.js.pen-tmp"},
		{name: "local pen config", path: "/.codepen.config.json"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			parent := t.TempDir()
			root := filepath.Join(parent, "work")
			err := Write(root, tt.path, strings.NewReader("x"))
			if err == nil || !strings.Contains(err.Error(), "refusing to write") {
				t.Errorf("Write(%q) error = %v, want refusal", tt.path, err)
			}
			if _, err := os.Stat(filepath.Join(parent, "escape.txt")); !errors.Is(err, fs.ErrNotExist) {
				t.Errorf("escape.txt was written outside the root")
			}
			if entries, _ := os.ReadDir(root); len(entries) > 0 {
				t.Errorf("Write(%q) left %d entries in the root", tt.path, len(entries))
			}
		})
	}
}

func TestWriteLocal(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	if err := WriteLocal(root, "/.codepen.config.json", strings.NewReader("x")); err != nil {
		t.Fatal(err)
	}
	if err := WriteLocal(root, "/../escape.txt", strings.NewReader("x")); err == nil {
		t.Error("WriteLocal() wrote outside the root")
	}
}

func TestWrite_ReplacesPlantedTempSymlink(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "victim")
	write(t, root, "a.js", "old")
	if err := os.Symlink(outside, filepath.Join(root, "a.js"+tmpSuffix)); err != nil {
		t.Fatal(err)
	}
	if err := Write(root, "/a.js", strings.NewReader("new")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(outside); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("write was redirected through the planted symlink: %v", err)
	}
	if data, _ := os.ReadFile(filepath.Join(root, "a.js")); string(data) != "new" {
		t.Errorf("a.js = %q, want \"new\"", data)
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

func TestCheckPaths(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		paths   []string
		problem string // part of the error; "" for none
	}{
		{name: "plain paths", paths: []string{"/a.js", "/src/a.js", "/b.css"}},
		{name: "outside the root", paths: []string{"/a.js", "/z/../../x"}, problem: "not a file path inside"},
		{name: "same name in another case", paths: []string{"/App.js", "/app.js"}, problem: "same file"},
		{name: "folders in another case", paths: []string{"/Src/a.js", "/src/b.js"}, problem: "same file"},
		{name: "trailing dot on Windows", paths: []string{"/a.js", "/a.js."}, problem: "same file"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := CheckPaths(tt.paths)
			if tt.problem == "" && err != nil {
				t.Errorf("CheckPaths(%q) = %v", tt.paths, err)
			}
			if tt.problem != "" && (err == nil || !strings.Contains(err.Error(), tt.problem)) {
				t.Errorf("CheckPaths(%q) = %v, want %q", tt.paths, err, tt.problem)
			}
		})
	}
}
