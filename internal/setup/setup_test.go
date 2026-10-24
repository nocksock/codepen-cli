package setup

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/nocksock/codepen-cli/internal/dotfile"
)

func TestScaffold(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	results, err := Scaffold(dir, DefaultKey)
	if err != nil {
		t.Fatal(err)
	}
	expected := []Result{{File: ".env", Status: dotfile.Created}, {File: ".envrc", Status: dotfile.Created}, {File: ".gitignore", Status: dotfile.Created}}
	if len(results) != len(expected) {
		t.Fatalf("results = %+v", results)
	}
	for i := range expected {
		if results[i] != expected[i] {
			t.Errorf("results[%d] = %+v, want %+v", i, results[i], expected[i])
		}
	}
	files := map[string]string{
		".env":       "CODEPEN_API_KEY=\"op://Private/CodePen/API Key\"\n",
		".envrc":     "dotenv_if_exists .env\ndotenv_if_exists .env.local\n",
		".gitignore": ".env\n.env.local\n",
	}
	for name, content := range files {
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil || string(data) != content {
			t.Errorf("%s = %q, %v; want %q", name, data, err, content)
		}
	}
}

func TestScaffold_KeepsExistingFiles(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte("CODEPEN_API_KEY=mine\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".gitignore"), []byte("node_modules"), 0o644); err != nil {
		t.Fatal(err)
	}
	results, err := Scaffold(dir, DefaultKey)
	if err != nil {
		t.Fatal(err)
	}
	expected := []Result{{File: ".env", Status: dotfile.Kept}, {File: ".envrc", Status: dotfile.Created}, {File: ".gitignore", Status: dotfile.Updated}}
	for i := range expected {
		if results[i] != expected[i] {
			t.Errorf("results[%d] = %+v, want %+v", i, results[i], expected[i])
		}
	}
	env, _ := os.ReadFile(filepath.Join(dir, ".env"))
	ignore, _ := os.ReadFile(filepath.Join(dir, ".gitignore"))
	if string(env) != "CODEPEN_API_KEY=mine\n" || string(ignore) != "node_modules\n.env\n.env.local\n" {
		t.Errorf(".env = %q, .gitignore = %q", env, ignore)
	}
}

type runner struct{ err error }

func (r runner) Output(context.Context, string, ...string) ([]byte, error) { return nil, r.err }

func TestAllowDirenv(t *testing.T) {
	t.Parallel()
	if !AllowDirenv(t.Context(), runner{}) {
		t.Error("AllowDirenv() = false when direnv succeeds")
	}
	if AllowDirenv(t.Context(), runner{err: errors.New("not found")}) {
		t.Error("AllowDirenv() = true when direnv is missing")
	}
}
