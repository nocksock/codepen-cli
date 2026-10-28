package cli

import (
	"fmt"
	"strings"
	"testing"

	"github.com/nocksock/codepen-cli/internal/codepen/codepentest"
)

func TestSync_PushesChanges(t *testing.T) {
	t.Parallel()
	w, id := connected(t, codepentest.Seed{Files: map[string]string{"a.js": "1"}})
	s := w.start("sync")
	s.waitFor(t, `watching`)
	matches(t, s.out.String(), `^already up to date$`)
	matches(t, s.out.String(), `^watching \.$`)

	w.write("a.js", "2")
	s.waitFor(t, `~ /a\.js`)
	eventually(t, func() bool { return w.api.Files(id)["/a.js"] == "2" }, func() string { return "remote a.js = 2" })

	w.write("src/new.js", "3")
	s.waitFor(t, `\+ /src/new\.js`)
	eventually(t, func() bool { return w.api.Files(id)["/src/new.js"] == "3" }, func() string { return "remote src/new.js = 3" })
	// Later pushes are stamped with the time, so a long session reads as a log.
	matches(t, s.out.String(), `^\d\d:\d\d:\d\d   \+ /src/new\.js$`)
	matches(t, s.out.String(), `^\d\d:\d\d:\d\d pushed 1 file to "`)

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

// A line written in pieces gets one stamp; output before watching gets none.
func TestStamper(t *testing.T) {
	t.Parallel()
	var out strings.Builder
	isOn := false
	s := &stamper{w: &out, isOn: &isOn}
	fmt.Fprint(s, "before\n")
	isOn = true
	fmt.Fprint(s, "a")
	fmt.Fprint(s, "b\nc\n")
	matches(t, out.String(), `\Abefore\n\d\d:\d\d:\d\d ab\n\d\d:\d\d:\d\d c\n\z`)
}
