package envfile

import (
	"maps"
	"os"
	"path/filepath"
	"testing"
)

func TestParse(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		input    string
		expected map[string]string
	}{
		{name: "plain", input: "A=1\nB=two\n", expected: map[string]string{"A": "1", "B": "two"}},
		{name: "double quotes with newline escape", input: `A="x y\nz"`, expected: map[string]string{"A": "x y\nz"}},
		{name: "single quotes stay literal", input: `A='x\ny'`, expected: map[string]string{"A": `x\ny`}},
		{name: "op ref with spaces", input: `CODEPEN_API_KEY="op://Private/CodePen/API Key"`, expected: map[string]string{"CODEPEN_API_KEY": "op://Private/CodePen/API Key"}},
		{name: "export prefix", input: "export A=1", expected: map[string]string{"A": "1"}},
		{name: "comments and blanks", input: "# note\n\nA=1 # trailing\n", expected: map[string]string{"A": "1"}},
		{name: "crlf", input: "A=1\r\nB=2\r\n", expected: map[string]string{"A": "1", "B": "2"}},
		{name: "lines without key", input: "=1\nnope\n", expected: map[string]string{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := Parse(tt.input); !maps.Equal(got, tt.expected) {
				t.Errorf("Parse(%q) = %v, want %v", tt.input, got, tt.expected)
			}
		})
	}
}

func TestRead(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	got, err := Read(filepath.Join(dir, "missing"))
	if err != nil || len(got) != 0 {
		t.Fatalf("Read(missing) = %v, %v; want empty, nil", got, err)
	}
	path := filepath.Join(dir, ".env")
	if err := os.WriteFile(path, []byte("A=1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err = Read(path)
	if err != nil || got["A"] != "1" {
		t.Errorf("Read() = %v, %v; want A=1", got, err)
	}
}

func TestSet(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		before   *string
		expected string
	}{
		{name: "creates the file", before: nil, expected: "CODEPEN_ID=new\n"},
		{name: "replaces the line", before: ptr("A=1\nCODEPEN_ID=old\nB=2\n"), expected: "A=1\nCODEPEN_ID=new\nB=2\n"},
		{name: "appends with a separator", before: ptr("A=1"), expected: "A=1\nCODEPEN_ID=new\n"},
		{name: "appends to an empty file", before: ptr(""), expected: "CODEPEN_ID=new\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			path := filepath.Join(t.TempDir(), ".env.local")
			if tt.before != nil {
				if err := os.WriteFile(path, []byte(*tt.before), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			if err := Set(path, "CODEPEN_ID", "new"); err != nil {
				t.Fatal(err)
			}
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if string(data) != tt.expected {
				t.Errorf("file = %q, want %q", data, tt.expected)
			}
		})
	}
}

func TestSet_CreatesPrivateFile(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), ".env.local")
	if err := Set(path, "A", "1"); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if mode := info.Mode().Perm(); mode != 0o600 {
		t.Errorf("mode = %o, want 600", mode)
	}
}

func ptr(s string) *string { return &s }
