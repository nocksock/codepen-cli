package cli

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/nocksock/codepen-cli/internal/codepen/codepentest"
)

func TestPull_EditorURLDownloadsTextAndBinary(t *testing.T) {
	t.Parallel()
	w := newWorkspace(t, map[string]string{"keep.txt": "local only"})
	id := w.api.Seed(codepentest.Seed{Files: map[string]string{"index.html": "<p>hi</p>"}, Binaries: map[string][]byte{"dot.png": png}})
	r := w.run("pull", "https://codepen.io/editor/me/pen/"+id+"/abc123")
	if r.code != 0 {
		t.Fatalf("code %d: %s", r.code, r.stderr)
	}
	matches(t, r.stdout, `^connected to ".*" \(`+id+`\)$`)
	matches(t, r.stdout, `^  editor +https://codepen\.io/editor/`)
	matches(t, r.stdout, `^pulled 3 files$`)
	if got := w.read("index.html"); got != "<p>hi</p>" {
		t.Errorf("index.html = %q", got)
	}
	data, err := os.ReadFile(w.path("dot.png"))
	if err != nil || !bytes.Equal(data, png) {
		t.Errorf("dot.png = %v, %v", data, err)
	}
	if got := w.read("keep.txt"); got != "local only" {
		t.Errorf("pull touched keep.txt: %q", got)
	}
}

// Pull brings in what changed on CodePen and keeps what changed here.
func TestPull_KeepsLocalEdits(t *testing.T) {
	t.Parallel()
	w, id := connected(t, codepentest.Seed{Files: map[string]string{"a.js": "1", "b.js": "1"}})
	w.api.Edit(id, map[string]string{"b.js": "remote"})
	w.write("a.js", "local")
	r := w.run("pull")
	if r.code != 0 {
		t.Fatalf("code %d: %s", r.code, r.stderr)
	}
	matches(t, r.stdout, `↓ /b\.js`)
	matches(t, r.stdout, `kept /a\.js`)
	matches(t, r.stdout, `^pulled 1 file$`)
	if strings.Contains(r.stdout, "editor") {
		t.Errorf("a plain pull printed the URLs:\n%s", r.stdout)
	}
	if a, b := w.read("a.js"), w.read("b.js"); a != "local" || b != "remote" {
		t.Errorf("a.js = %q, b.js = %q; want the local edit and the remote one", a, b)
	}
}

func TestPull_RefusesEditsOnBothSides(t *testing.T) {
	t.Parallel()
	w, id := connected(t, codepentest.Seed{Files: map[string]string{"a.js": "1", "b.js": "1"}})
	w.api.Edit(id, map[string]string{"a.js": "remote", "b.js": "remote"})
	w.write("a.js", "local")
	r := w.run("pull")
	if r.code != 1 || !strings.Contains(r.stderr, "/a.js") || !strings.Contains(r.stderr, "--force") {
		t.Errorf("code %d, stderr %q; want a refusal naming /a.js and --force", r.code, r.stderr)
	}
	if a, b := w.read("a.js"), w.read("b.js"); a != "local" || b != "1" {
		t.Errorf("a.js = %q, b.js = %q; want nothing written", a, b)
	}
	forced := w.run("pull", "--force")
	if forced.code != 0 || w.read("a.js") != "remote" {
		t.Errorf("--force: code %d, a.js %q; want the remote version", forced.code, w.read("a.js"))
	}
}

// Without a record of the last sync (a fresh clone, a moved folder), pull
// can't tell who changed a file, so it replaces none.
func TestPull_WithoutLastSyncReplacesNothing(t *testing.T) {
	t.Parallel()
	w, id := connected(t, codepentest.Seed{Files: map[string]string{"a.js": "1"}})
	w.api.Edit(id, map[string]string{"a.js": "remote"})
	w.write("a.js", "local")
	w.vars["XDG_STATE_HOME"] = w.t.TempDir()
	if r := w.run("pull"); r.code != 1 || !strings.Contains(r.stderr, "/a.js") {
		t.Errorf("code %d, stderr %q; want a refusal naming /a.js", r.code, r.stderr)
	}
	if got := w.read("a.js"); got != "local" {
		t.Errorf("a.js = %q, want the local version", got)
	}
}

// A push records the synced files too, so a later pull knows they're untouched.
func TestPull_AfterPush(t *testing.T) {
	t.Parallel()
	w := newWorkspace(t, map[string]string{"a.js": "1"})
	if r := w.run("push"); r.code != 0 {
		t.Fatalf("push: %s", r.stderr)
	}
	w.api.Edit(w.penID(), map[string]string{"a.js": "remote"})
	if r := w.run("pull"); r.code != 0 || w.read("a.js") != "remote" {
		t.Errorf("pull: code %d, a.js %q, stderr %q", r.code, w.read("a.js"), r.stderr)
	}
}

func TestPull_WithoutPenFails(t *testing.T) {
	t.Parallel()
	r := newWorkspace(t, nil).run("pull")
	if r.code != 1 || !strings.Contains(r.stderr, "no Pen connected: run `pen push` or `pen pull <PEN>`") {
		t.Errorf("code %d, stderr %q", r.code, r.stderr)
	}
}

func TestPull_SwitchingNeedsForce(t *testing.T) {
	t.Parallel()
	w, _ := connected(t, codepentest.Seed{Files: map[string]string{"a.js": "1"}})
	other := w.api.Seed(codepentest.Seed{Files: map[string]string{"b.js": "2"}})
	refused := w.run("pull", other)
	if refused.code != 1 || !strings.Contains(refused.stderr, "pass --force") {
		t.Errorf("refused: code %d, stderr %q", refused.code, refused.stderr)
	}
	forced := w.run("pull", other, "--force")
	if forced.code != 0 {
		t.Fatalf("forced: code %d: %s", forced.code, forced.stderr)
	}
	if got := w.penID(); got != other {
		t.Errorf("stored Pen id = %q, want %q", got, other)
	}
	if got := w.read("b.js"); got != "2" {
		t.Errorf("b.js = %q", got)
	}
}

func TestPull_NothingNew(t *testing.T) {
	t.Parallel()
	w, _ := connected(t, codepentest.Seed{Files: map[string]string{"a.js": "1"}})
	if r := w.run("pull"); r.code != 0 || r.stdout != "already up to date\n" {
		t.Errorf("code %d, stdout %q; want only the up-to-date line", r.code, r.stdout)
	}
}
