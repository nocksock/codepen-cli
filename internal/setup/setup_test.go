package setup

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/nocksock/codepen-cli/internal/dotfile"
)

func TestScaffold(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	results, err := Scaffold(dir)
	if err != nil {
		t.Fatal(err)
	}
	expected := []Result{{File: ".gitignore", Status: dotfile.Created}}
	if len(results) != len(expected) {
		t.Fatalf("results = %+v", results)
	}
	for i := range expected {
		if results[i] != expected[i] {
			t.Errorf("results[%d] = %+v, want %+v", i, results[i], expected[i])
		}
	}
	data, err := os.ReadFile(filepath.Join(dir, ".gitignore"))
	if err != nil || string(data) != ".env\n.env.local\n" {
		t.Errorf(".gitignore = %q, %v", data, err)
	}
	for _, name := range []string{".env", ".envrc"} {
		if _, err := os.Stat(filepath.Join(dir, name)); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("%s was created: %v", name, err)
		}
	}
}

func TestScaffold_KeepsExistingFiles(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte("CODEPEN_API_KEY=mine\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".envrc"), []byte("export FOO=1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".gitignore"), []byte("node_modules"), 0o644); err != nil {
		t.Fatal(err)
	}
	results, err := Scaffold(dir)
	if err != nil {
		t.Fatal(err)
	}
	expected := []Result{{File: ".gitignore", Status: dotfile.Updated}}
	for i := range expected {
		if results[i] != expected[i] {
			t.Errorf("results[%d] = %+v, want %+v", i, results[i], expected[i])
		}
	}
	for name, want := range map[string]string{
		".env":       "CODEPEN_API_KEY=mine\n",
		".envrc":     "export FOO=1\n",
		".gitignore": "node_modules\n.env\n.env.local\n",
	} {
		data, _ := os.ReadFile(filepath.Join(dir, name))
		if string(data) != want {
			t.Errorf("%s = %q, want %q", name, data, want)
		}
	}
}
