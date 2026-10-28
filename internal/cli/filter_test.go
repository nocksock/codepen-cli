package cli

import (
	"strings"
	"testing"

	"github.com/nocksock/codepen-cli/internal/codepen/codepentest"
	"github.com/nocksock/codepen-cli/internal/penconfig"
)

func (w *workspace) setList(key string, patterns ...string) {
	w.t.Helper()
	if err := penconfig.SetList(w.dir, key, patterns); err != nil {
		w.t.Fatal(err)
	}
}

func TestPush_ExcludedFilesStayLocal(t *testing.T) {
	t.Parallel()
	w, id := connected(t, codepentest.Seed{Files: map[string]string{"a.js": "1"}})
	w.setList(penconfig.Exclude, "*.map")
	w.write("a.js", "2")
	w.write("src/a.js.map", "{}")
	r := w.run("push")
	if r.code != 0 || strings.Contains(r.stdout, ".map") {
		t.Fatalf("code %d, stdout %q, stderr %q", r.code, r.stdout, r.stderr)
	}
	if files := w.api.Files(id); files["/a.js"] != "2" || files["/src/a.js.map"] != "" {
		t.Errorf("remote files = %v, want a.js pushed and the map left out", files)
	}
}

// Excluded files on CodePen are left alone: a push doesn't delete them, and a
// pull doesn't write them.
func TestExclude_LeavesRemoteFilesAlone(t *testing.T) {
	t.Parallel()
	w := newWorkspace(t, nil)
	id := w.api.Seed(codepentest.Seed{Files: map[string]string{"a.js": "1", "dist/x.js": "built"}})
	w.setList(penconfig.Exclude, "dist/")
	if r := w.run("pull", id); r.code != 0 {
		t.Fatalf("pull: %s", r.stderr)
	}
	if w.exists("dist/x.js") {
		t.Error("pull wrote the excluded dist/x.js")
	}
	w.write("a.js", "2")
	if r := w.run("push"); r.code != 0 || strings.Contains(r.stdout, "dist") {
		t.Fatalf("push: code %d, stdout %q, stderr %q", r.code, r.stdout, r.stderr)
	}
	if got := w.api.Files(id)["/dist/x.js"]; got != "built" {
		t.Errorf("remote dist/x.js = %q, want it kept", got)
	}
}

func TestInclude_OnlyMatchingFilesSync(t *testing.T) {
	t.Parallel()
	w := newWorkspace(t, map[string]string{"src/a.js": "1", "index.html": "<p>", "notes.txt": "mine"})
	w.setList(penconfig.Include, "src/", "index.html")
	if r := w.run("push"); r.code != 0 {
		t.Fatalf("push: %s", r.stderr)
	}
	files := w.api.Files(w.penID())
	if files["/src/a.js"] != "1" || files["/index.html"] != "<p>" || files["/notes.txt"] != "" {
		t.Errorf("remote files = %v, want only src/ and index.html", files)
	}
}

// Flags add to the config's lists for one run.
func TestPush_FilterFlags(t *testing.T) {
	t.Parallel()
	w, id := connected(t, codepentest.Seed{Files: map[string]string{"a.js": "1"}})
	w.setList(penconfig.Exclude, "*.map")
	w.write("a.js", "2")
	w.write("a.js.map", "{}")
	w.write("drafts/x.js", "wip")
	r := w.run("push", "--exclude", "drafts/", "--exclude", "nothing-else")
	if r.code != 0 {
		t.Fatalf("code %d: %s", r.code, r.stderr)
	}
	if files := w.api.Files(id); files["/a.js"] != "2" || files["/a.js.map"] != "" || files["/drafts/x.js"] != "" {
		t.Errorf("remote files = %v, want only a.js", files)
	}
	w.write("lib/b.js", "3")
	if r := w.run("push", "--include", "lib/"); r.code != 0 {
		t.Fatalf("push --include: %s", r.stderr)
	}
	files := w.api.Files(id)
	if files["/lib/b.js"] != "3" || files["/a.js"] != "2" {
		t.Errorf("remote files = %v, want lib/b.js added and a.js left alone", files)
	}
}

func TestSync_FilterFlags(t *testing.T) {
	t.Parallel()
	w, id := connected(t, codepentest.Seed{Files: map[string]string{"a.js": "1"}})
	s := w.start("sync", "--exclude", "*.map")
	s.waitFor(t, `watching`)
	w.write("a.js.map", "{}")
	w.write("a.js", "2")
	eventually(t, func() bool { return w.api.Files(id)["/a.js"] == "2" }, func() string { return "remote a.js = 2" })
	if got := w.api.Files(id)["/a.js.map"]; got != "" {
		t.Errorf("remote a.js.map = %q, want it left out", got)
	}
}

func TestFilter_BadPattern(t *testing.T) {
	t.Parallel()
	w, _ := connected(t, codepentest.Seed{Files: map[string]string{"a.js": "1"}})
	w.setList(penconfig.Exclude, "src/[")
	r := w.run("push")
	if r.code != 1 || !strings.Contains(r.stderr, `"src/[" is not a pattern`) {
		t.Errorf("code %d, stderr %q", r.code, r.stderr)
	}
}

func TestInfo_CountsOnlyFilteredChanges(t *testing.T) {
	t.Parallel()
	w, _ := connected(t, codepentest.Seed{Files: map[string]string{"a.js": "1"}})
	w.setList(penconfig.Exclude, "*.map")
	w.write("a.js.map", "{}")
	matches(t, w.run("info").stdout, `^local +in sync$`)
}
