package cli

import (
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/nocksock/codepen-cli/internal/codepen/codepentest"
)

func TestInit_CreatesFilesAndAllowsDirenv(t *testing.T) {
	t.Parallel()
	w := newWorkspace(t, nil)
	r := w.run("init")
	if r.code != 0 {
		t.Fatalf("code %d: %s", r.code, r.stderr)
	}
	matches(t, r.stdout, `created \.env$`)
	matches(t, r.stdout, `direnv allowed`)
	expected := map[string]string{
		".env":       "CODEPEN_API_KEY=\"op://Private/CodePen/API Key\"\n",
		".envrc":     "dotenv_if_exists .env\ndotenv_if_exists .env.local\n",
		".gitignore": ".env\n.env.local\n",
	}
	for file, content := range expected {
		if got := w.read(file); got != content {
			t.Errorf("%s = %q, want %q", file, got, content)
		}
	}
	if calls := w.runner.Calls(); !slices.Equal(calls, []string{"direnv allow ."}) {
		t.Errorf("calls = %q", calls)
	}
}

func TestInit_KeySetsNewEnv(t *testing.T) {
	t.Parallel()
	w := newWorkspace(t, nil)
	w.run("init", "--key", "cp_user_abc")
	if got := w.read(".env"); got != "CODEPEN_API_KEY=\"cp_user_abc\"\n" {
		t.Errorf(".env = %q", got)
	}
}

func TestInit_TwiceChangesNothing(t *testing.T) {
	t.Parallel()
	w := newWorkspace(t, nil)
	w.run("init")
	files := []string{".env", ".envrc", ".gitignore"}
	before := []string{}
	for _, f := range files {
		before = append(before, w.read(f))
	}
	r := w.run("init")
	if got := strings.Count(r.stdout, "kept"); got != 3 {
		t.Errorf("kept %d files, want 3:\n%s", got, r.stdout)
	}
	for i, f := range files {
		if got := w.read(f); got != before[i] {
			t.Errorf("%s changed to %q", f, got)
		}
	}
}

func TestInit_AppendsAndKeepsExistingEnv(t *testing.T) {
	t.Parallel()
	w := newWorkspace(t, map[string]string{
		".env":       "CODEPEN_API_KEY=mine\n",
		".envrc":     "export FOO=1",
		".gitignore": "node_modules\n.env\n",
	})
	r := w.run("init", "--key", "ignored")
	if !regexp.MustCompile(`kept {4}\.env\n`).MatchString(r.stdout) {
		t.Errorf("stdout = %q", r.stdout)
	}
	matches(t, r.stdout, `updated \.envrc`)
	if got := w.read(".env"); got != "CODEPEN_API_KEY=mine\n" {
		t.Errorf(".env = %q", got)
	}
	if got := w.read(".envrc"); got != "export FOO=1\ndotenv_if_exists .env\ndotenv_if_exists .env.local\n" {
		t.Errorf(".envrc = %q", got)
	}
	if got := w.read(".gitignore"); got != "node_modules\n.env\n.env.local\n" {
		t.Errorf(".gitignore = %q", got)
	}
}

func TestInit_WithoutDirenv(t *testing.T) {
	t.Parallel()
	w := newWorkspace(t, nil)
	w.runner.missing["direnv"] = true
	r := w.run("init")
	if r.code != 0 || strings.Contains(r.stdout, "direnv allowed") {
		t.Errorf("code %d, stdout %q", r.code, r.stdout)
	}
}

func TestInit_PenConnectsAndPulls(t *testing.T) {
	t.Parallel()
	w := newWorkspace(t, nil)
	id := w.api.Seed(codepentest.Seed{Files: map[string]string{"index.html": "<p>hi</p>"}})
	if r := w.run("init", id); r.code != 0 {
		t.Fatalf("code %d: %s", r.code, r.stderr)
	}
	if got := w.read("index.html"); got != "<p>hi</p>" {
		t.Errorf("index.html = %q", got)
	}
	if got := w.read(".env.local"); got != "CODEPEN_ID="+id+"\n" {
		t.Errorf(".env.local = %q", got)
	}
}
