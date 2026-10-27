package cli

import (
	"os"
	"strings"
	"testing"

	"github.com/nocksock/codepen-cli/internal/codepen/codepentest"
)

func TestInit_CreatesGitignore(t *testing.T) {
	t.Parallel()
	w := newWorkspace(t, nil)
	r := w.run("init")
	if r.code != 0 {
		t.Fatalf("code %d: %s", r.code, r.stderr)
	}
	matches(t, r.stdout, `created \.gitignore$`)
	matches(t, r.stdout, `CODEPEN_API_KEY`)
	if got := w.read(".gitignore"); got != ".env\n.env.local\n" {
		t.Errorf(".gitignore = %q", got)
	}
	for _, name := range []string{".env", ".envrc"} {
		if w.exists(name) {
			t.Errorf("init created %s", name)
		}
	}
	if calls := w.runner.Calls(); len(calls) != 0 {
		t.Errorf("calls = %q, want none", calls)
	}
}

func TestInit_TwiceChangesNothing(t *testing.T) {
	t.Parallel()
	w := newWorkspace(t, nil)
	w.run("init")
	before := w.read(".gitignore")
	r := w.run("init")
	if got := strings.Count(r.stdout, "kept"); got != 1 {
		t.Errorf("kept %d files, want 1:\n%s", got, r.stdout)
	}
	if got := w.read(".gitignore"); got != before {
		t.Errorf(".gitignore changed to %q", got)
	}
}

func TestInit_AppendsAndKeepsExistingFiles(t *testing.T) {
	t.Parallel()
	w := newWorkspace(t, map[string]string{
		".env":       "CODEPEN_API_KEY=mine\n",
		".envrc":     "export FOO=1\n",
		".gitignore": "node_modules\n.env\n",
	})
	r := w.run("init")
	matches(t, r.stdout, `updated \.gitignore`)
	if got := w.read(".env"); got != "CODEPEN_API_KEY=mine\n" {
		t.Errorf(".env = %q", got)
	}
	if got := w.read(".envrc"); got != "export FOO=1\n" {
		t.Errorf(".envrc = %q", got)
	}
	if got := w.read(".gitignore"); got != "node_modules\n.env\n.env.local\n" {
		t.Errorf(".gitignore = %q", got)
	}
}

func TestInit_PenConnectsAndPulls(t *testing.T) {
	t.Parallel()
	w := newWorkspace(t, nil)
	id := w.api.Seed(codepentest.Seed{Files: map[string]string{"index.html": "<p>hi</p>"}})
	r := w.run("init", id)
	if r.code != 0 {
		t.Fatalf("code %d: %s", r.code, r.stderr)
	}
	matches(t, r.stdout, `^editor +https://codepen\.io/editor/`)
	if got := w.read("index.html"); got != "<p>hi</p>" {
		t.Errorf("index.html = %q", got)
	}
	if got := w.penID(); got != id {
		t.Errorf("stored Pen id = %q, want %q", got, id)
	}
}

func TestInit_Help(t *testing.T) {
	t.Parallel()
	w := newWorkspace(t, nil)
	r := w.run("init", "--help")
	if r.code != 0 || !strings.Contains(r.stdout, "pen init [PEN]") {
		t.Errorf("code %d, stdout %q", r.code, r.stdout)
	}
	for _, name := range []string{".env", ".envrc", ".gitignore"} {
		if w.exists(name) {
			t.Errorf("--help created %s", name)
		}
	}
	if calls := w.runner.Calls(); len(calls) != 0 {
		t.Errorf("calls = %q, want none", calls)
	}
}

func TestInit_ScaffoldError(t *testing.T) {
	t.Parallel()
	w := newWorkspace(t, nil)
	if err := os.Mkdir(w.path(".gitignore"), 0o755); err != nil {
		t.Fatal(err)
	}
	r := w.run("init")
	if r.code != 1 || r.stdout != "" || !strings.Contains(r.stderr, "is a directory") {
		t.Errorf("code %d, stdout %q, stderr %q", r.code, r.stdout, r.stderr)
	}
	if calls := w.runner.Calls(); len(calls) != 0 {
		t.Errorf("calls = %q, want none", calls)
	}
}

func TestInit_PenNeedsKey(t *testing.T) {
	t.Parallel()
	w := newWorkspace(t, nil)
	delete(w.vars, "CODEPEN_API_KEY")
	id := w.api.Seed(codepentest.Seed{Files: map[string]string{"index.html": "<p>hi</p>"}})
	r := w.run("init", id)
	if r.code != 1 || !strings.Contains(r.stderr, "CODEPEN_API_KEY is not set") {
		t.Errorf("code %d, stderr %q", r.code, r.stderr)
	}
}

func TestInit_ForceSwitchesPen(t *testing.T) {
	t.Parallel()
	w, first := connected(t, codepentest.Seed{Files: map[string]string{"a.js": "1"}})
	other := w.api.Seed(codepentest.Seed{Files: map[string]string{"b.js": "2"}})
	if r := w.run("init", other); r.code != 1 {
		t.Fatalf("without --force: code %d, want 1", r.code)
	}
	if r := w.run("init", "--force", other); r.code != 0 {
		t.Fatalf("code %d: %s", r.code, r.stderr)
	}
	if got := w.penID(); got != other || first == other {
		t.Errorf("stored Pen id = %q, want %q", got, other)
	}
}
