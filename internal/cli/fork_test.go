package cli

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/nocksock/codepen-cli/internal/codepen/codepentest"
	"github.com/nocksock/codepen-cli/internal/penconfig"
)

func creates(w *workspace) int { return w.api.Calls(`^POST /v2/pens$`) }

// CodePen's own fork, which links the copy to its source, isn't available
// yet; until it is, fork says so instead of quietly doing something else.
func TestFork_NeedsDetach(t *testing.T) {
	t.Parallel()
	w, _ := connected(t, codepentest.Seed{Files: map[string]string{"a.js": "1"}})
	r := w.run("fork")
	if r.code != 1 || !strings.Contains(r.stderr, "--detach") || creates(w) != 0 {
		t.Errorf("code %d, stderr %q, creates %d; want a pointer to --detach", r.code, r.stderr, creates(w))
	}
}

func TestFork_NothingToFork(t *testing.T) {
	t.Parallel()
	w := newWorkspace(t, map[string]string{"a.js": "1"})
	r := w.run("fork", "--detach")
	if r.code != 1 || !strings.Contains(r.stderr, "nothing to fork: `pen push` creates a Pen") {
		t.Errorf("code %d, stderr %q", r.code, r.stderr)
	}
}

// By default the copy is what you see: the folder, unpushed edits included.
func TestFork_Detach(t *testing.T) {
	t.Parallel()
	w, original := connected(t, codepentest.Seed{Title: "Demo", Files: map[string]string{"a.js": "1"}})
	w.write("a.js", "local")
	r := w.run("fork", "--detach")
	if r.code != 0 {
		t.Fatalf("code %d: %s", r.code, r.stderr)
	}
	copied := w.penID()
	if copied == original {
		t.Fatal("the folder is still connected to the original")
	}
	matches(t, r.stdout, `^forked "Demo" → "Demo" \(`+copied+`\)$`)
	matches(t, r.stdout, `^  editor +https://codepen\.io/editor/`)
	if got := w.api.Files(copied)["/a.js"]; got != "local" {
		t.Errorf("copy's a.js = %q, want the local edit", got)
	}
	if got := w.api.Files(original)["/a.js"]; got != "1" {
		t.Errorf("original's a.js = %q, want it untouched", got)
	}
}

// remote copies the Pen as it is on CodePen; local edits stay, ready to push.
func TestFork_DetachRemote(t *testing.T) {
	t.Parallel()
	w, original := connected(t, codepentest.Seed{Files: map[string]string{"a.js": "1", "b.js": "1"}})
	w.api.Edit(original, map[string]string{"b.js": "remote"})
	w.write("a.js", "local")
	r := w.run("fork", "remote", "--detach")
	if r.code != 0 {
		t.Fatalf("code %d: %s", r.code, r.stderr)
	}
	copied := w.penID()
	if files := w.api.Files(copied); files["/a.js"] != "1" || files["/b.js"] != "remote" {
		t.Errorf("copy's files = %v, want the Pen as it was on CodePen", files)
	}
	if a, b := w.read("a.js"), w.read("b.js"); a != "local" || b != "remote" {
		t.Errorf("a.js = %q, b.js = %q; want the local edit kept and the remote one pulled", a, b)
	}
	if r := w.run("push"); r.code != 0 || w.api.Files(copied)["/a.js"] != "local" {
		t.Errorf("push after fork: code %d, copy's a.js %q", r.code, w.api.Files(copied)["/a.js"])
	}
}

// A file changed on both sides stops the fork before any Pen is made.
func TestFork_DetachRemoteRefusesConflicts(t *testing.T) {
	t.Parallel()
	w, original := connected(t, codepentest.Seed{Files: map[string]string{"a.js": "1"}})
	w.api.Edit(original, map[string]string{"a.js": "remote"})
	w.write("a.js", "local")
	r := w.run("fork", "remote", "--detach")
	if r.code != 1 || !strings.Contains(r.stderr, "/a.js") || creates(w) != 0 {
		t.Errorf("code %d, stderr %q, creates %d; want a refusal before creating", r.code, r.stderr, creates(w))
	}
	if w.penID() != original {
		t.Error("the folder switched Pens despite the refusal")
	}
}

// --into puts the copy in a new folder and leaves this one as it was.
func TestFork_DetachInto(t *testing.T) {
	t.Parallel()
	w, original := connected(t, codepentest.Seed{Files: map[string]string{"a.js": "1"}})
	other := w.api.Seed(codepentest.Seed{Title: "Other", Files: map[string]string{"b.js": "2"}})
	r := w.run("fork", "https://codepen.io/editor/me/pen/"+other, "--detach", "--into", "variants/b")
	if r.code != 0 {
		t.Fatalf("code %d: %s", r.code, r.stderr)
	}
	if got := w.read("variants/b/b.js"); got != "2" {
		t.Errorf("variants/b/b.js = %q", got)
	}
	copied, err := penconfig.Read(filepath.Join(w.dir, "variants", "b"))
	if err != nil || copied == "" || copied == other {
		t.Errorf("variants/b is connected to %q, %v; want the copy", copied, err)
	}
	if w.penID() != original {
		t.Error("this folder switched Pens")
	}
}

func TestFork_IntoNeedsAnEmptyFolder(t *testing.T) {
	t.Parallel()
	w, _ := connected(t, codepentest.Seed{Files: map[string]string{"a.js": "1"}})
	w.write("taken/x.txt", "x")
	r := w.run("fork", "--detach", "--into", "taken")
	if r.code != 1 || !strings.Contains(r.stderr, "isn't empty") || creates(w) != 0 {
		t.Errorf("code %d, stderr %q, creates %d", r.code, r.stderr, creates(w))
	}
}
