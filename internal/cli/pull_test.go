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
	matches(t, r.stdout, `connected`)
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

func TestPull_OverwritesLocalEdits(t *testing.T) {
	t.Parallel()
	w, id := connected(t, codepentest.Seed{Files: map[string]string{"a.js": "1"}})
	w.api.Edit(id, map[string]string{"a.js": "remote"})
	w.write("a.js", "local")
	r := w.run("pull")
	matches(t, r.stdout, `↓ /a\.js`)
	if got := w.read("a.js"); got != "remote" {
		t.Errorf("a.js = %q", got)
	}
}

func TestPull_WithoutPenFails(t *testing.T) {
	t.Parallel()
	r := newWorkspace(t, nil).run("pull")
	if r.code != 1 || !strings.Contains(r.stderr, "no Pen to pull") {
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
	if got := w.read(".env.local"); got != "CODEPEN_ID="+other+"\n" {
		t.Errorf(".env.local = %q", got)
	}
	if got := w.read("b.js"); got != "2" {
		t.Errorf("b.js = %q", got)
	}
}
