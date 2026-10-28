package cli

import (
	"strings"
	"testing"

	"github.com/nocksock/codepen-cli/internal/penconfig"
)

func TestConfig_Show(t *testing.T) {
	t.Parallel()
	w := newWorkspace(t, nil)
	delete(w.vars, "CODEPEN_API_KEY") // settings are local; no key needed
	if r := w.run("config"); r.code != 0 || r.stdout != "id       (none)\ninclude  (none)\nexclude  (none)\n" {
		t.Errorf("code %d, stdout %q, stderr %q", r.code, r.stdout, r.stderr)
	}
	if err := penconfig.Save(w.dir, "P1"); err != nil {
		t.Fatal(err)
	}
	w.setList(penconfig.Exclude, "*.map", "dist/")
	if r := w.run("config"); r.stdout != "id       P1\ninclude  (none)\nexclude  *.map, dist/\n" {
		t.Errorf("stdout %q", r.stdout)
	}
}

func TestConfig_Get(t *testing.T) {
	t.Parallel()
	w := newWorkspace(t, nil)
	w.setList(penconfig.Include, "src/", "index.html")
	if r := w.run("config", "get", "include"); r.code != 0 || r.stdout != "src/\nindex.html\n" {
		t.Errorf("code %d, stdout %q", r.code, r.stdout)
	}
	if r := w.run("config", "get", "exclude"); r.code != 0 || r.stdout != "" {
		t.Errorf("unset: code %d, stdout %q", r.code, r.stdout)
	}
}

func list(t *testing.T, w *workspace, key string) []string {
	t.Helper()
	got, err := penconfig.List(w.dir, key)
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func TestConfig_SetAddRemove(t *testing.T) {
	t.Parallel()
	w := newWorkspace(t, nil)
	if r := w.run("config", "set", "exclude", "*.map", "dist/"); r.code != 0 || r.stdout != "exclude  *.map, dist/\n" {
		t.Errorf("set: code %d, stdout %q, stderr %q", r.code, r.stdout, r.stderr)
	}
	w.run("config", "add", "exclude", "drafts/", "*.map")
	if got := strings.Join(list(t, w, "exclude"), " "); got != "*.map dist/ drafts/" {
		t.Errorf("after add: %q, want drafts/ added once", got)
	}
	w.run("config", "remove", "exclude", "dist/")
	if got := strings.Join(list(t, w, "exclude"), " "); got != "*.map drafts/" {
		t.Errorf("after remove: %q", got)
	}
	if r := w.run("config", "set", "exclude"); r.code != 0 || len(list(t, w, "exclude")) != 0 {
		t.Errorf("clearing: code %d, list %q", r.code, list(t, w, "exclude"))
	}
}

// Settings live in the mirrored folder's config, wherever CODEPEN_DIR points.
func TestConfig_UsesCodepenDir(t *testing.T) {
	t.Parallel()
	w := newWorkspace(t, nil)
	w.vars["CODEPEN_DIR"] = "site"
	w.run("config", "set", "include", "src/")
	got, err := penconfig.List(w.path("site"), penconfig.Include)
	if err != nil || len(got) != 1 || got[0] != "src/" {
		t.Errorf("site's include = %q, %v", got, err)
	}
}

func TestConfig_Refusals(t *testing.T) {
	t.Parallel()
	w := newWorkspace(t, nil)
	w.setList(penconfig.Exclude, "*.map")
	tests := []struct {
		args    []string
		code    int
		message string
	}{
		{args: []string{"config", "set", "id", "x"}, code: 2, message: `unknown setting "id": use include or exclude`},
		{args: []string{"config", "get", "bogus"}, code: 2, message: `unknown setting "bogus"`},
		{args: []string{"config", "add", "exclude"}, code: 2, message: "requires at least 2 arg"},
		{args: []string{"config", "add", "exclude", "src/["}, code: 1, message: `"src/[" is not a pattern`},
		{args: []string{"config", "remove", "exclude", "nope"}, code: 1, message: `"nope" isn't in exclude`},
	}
	for _, tt := range tests {
		r := w.run(tt.args...)
		if r.code != tt.code || !strings.Contains(r.stderr, tt.message) {
			t.Errorf("%v: code %d, stderr %q; want %d, %q", tt.args, r.code, r.stderr, tt.code, tt.message)
		}
	}
	if got := strings.Join(list(t, w, "exclude"), " "); got != "*.map" {
		t.Errorf("exclude = %q, want it unchanged", got)
	}
}
