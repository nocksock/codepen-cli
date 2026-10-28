package cli

import (
	"testing"

	"github.com/nocksock/codepen-cli/internal/codepen/codepentest"
)

func TestSync_PushesChanges(t *testing.T) {
	t.Parallel()
	w, id := connected(t, codepentest.Seed{Files: map[string]string{"a.js": "1"}})
	s := w.start("sync")
	s.waitFor(t, `watching`)
	matches(t, s.out.String(), `^editor +https://codepen\.io/editor/`)

	w.write("a.js", "2")
	s.waitFor(t, `~ /a\.js`)
	eventually(t, func() bool { return w.api.Files(id)["/a.js"] == "2" }, func() string { return "remote a.js = 2" })

	w.write("src/new.js", "3")
	s.waitFor(t, `~ /src/new\.js`)
	eventually(t, func() bool { return w.api.Files(id)["/src/new.js"] == "3" }, func() string { return "remote src/new.js = 3" })

	if code := s.stop(); code != 130 {
		t.Errorf("exit code after cancel = %d, want 130", code)
	}
}

func TestSync_ReportsRemoteEdit(t *testing.T) {
	t.Parallel()
	w, id := connected(t, codepentest.Seed{Files: map[string]string{"a.js": "1"}})
	s := w.start("sync")
	s.waitFor(t, `watching`)
	w.api.Edit(id, map[string]string{"a.js": "from the editor"})
	w.write("a.js", "local")
	s.waitFor(t, "✗ the Pen changed on CodePen since your last sync: stop sync, run `pen pull` \\(it keeps your edits\\), then `pen sync` again")
	if got := w.api.Files(id)["/a.js"]; got != "from the editor" {
		t.Errorf("remote a.js = %q, the remote edit was overwritten", got)
	}
	if code := s.stop(); code != 130 {
		t.Errorf("exit code after cancel = %d, want 130", code)
	}
}

func TestSync_ForceOverwritesRemoteEdit(t *testing.T) {
	t.Parallel()
	w, id := connected(t, codepentest.Seed{Files: map[string]string{"a.js": "1"}})
	s := w.start("sync", "--force")
	s.waitFor(t, `watching`)
	w.api.Edit(id, map[string]string{"a.js": "from the editor"})
	w.write("a.js", "local")
	s.waitFor(t, `~ /a\.js`)
	eventually(t, func() bool { return w.api.Files(id)["/a.js"] == "local" }, func() string { return "remote a.js = local" })
}

func TestSync_RejectsArguments(t *testing.T) {
	t.Parallel()
	w := newWorkspace(t, nil)
	if r := w.run("sync", "extra"); r.code != 2 {
		t.Errorf("exit code = %d, want 2", r.code)
	}
}
