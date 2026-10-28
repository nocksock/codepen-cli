package cli

import (
	"bytes"
	"encoding/hex"
	"strings"
	"testing"

	"github.com/nocksock/codepen-cli/internal/codepen"
	"github.com/nocksock/codepen-cli/internal/codepen/codepentest"
)

var png, _ = hex.DecodeString("89504e470d0a1a0a0000000d49484452000000010000000108060000001f15c489")

func writes(w *workspace, id string) int {
	return w.api.Calls(`^PUT /v2/pens/` + id + `/files$`)
}

func TestPush_CreatesPen(t *testing.T) {
	t.Parallel()
	w := newWorkspace(t, map[string]string{"index.html": "<h1>x</h1>", "src/app.js": "1"})
	r := w.run("push")
	if r.code != 0 {
		t.Fatalf("code %d: %s", r.code, r.stderr)
	}
	for _, line := range []string{`^  \+ /index\.html$`, `^  \+ /src/app\.js$`, `^created "`, `^  editor +https://codepen\.io/editor/`} {
		matches(t, r.stdout, line)
	}
	id := w.penID()
	if id == "" {
		t.Fatalf("no Pen id stored in .codepen.config.json: %q", w.read(".codepen.config.json"))
	}
	if w.exists(".env.local") {
		t.Error("push wrote .env.local; the id belongs in .codepen.config.json")
	}
	files := w.api.Files(id)
	if files["/index.html"] != "<h1>x</h1>" || files["/src/app.js"] != "1" {
		t.Errorf("remote files = %v", files)
	}
	if !w.exists(".codepen/pen.config.json") {
		t.Error("the compiler's patch did not come back down")
	}
}

func TestPush_EmptyDirectoryGetsStarterFiles(t *testing.T) {
	t.Parallel()
	w := newWorkspace(t, nil)
	if r := w.run("push"); r.code != 0 {
		t.Fatalf("code %d: %s", r.code, r.stderr)
	}
	if got := w.read("index.html"); got != "<h1>starter</h1>" {
		t.Errorf("index.html = %q", got)
	}
}

func TestPush_NothingChanged(t *testing.T) {
	t.Parallel()
	w, id := connected(t, codepentest.Seed{Files: map[string]string{"a.js": "1"}})
	r := w.run("push")
	if r.code != 0 || writes(w, id) != 0 || r.stdout != "already up to date\n" {
		t.Errorf("code %d, stdout %q, writes %d; want only the up-to-date line", r.code, r.stdout, writes(w, id))
	}
}

func TestPush_UploadsAndDeletes(t *testing.T) {
	t.Parallel()
	w, id := connected(t, codepentest.Seed{Files: map[string]string{"a.js": "1", "b.js": "2"}})
	w.write("a.js", "changed")
	w.write("c.js", "new")
	w.remove("b.js")
	r := w.run("push")
	if r.code != 0 {
		t.Fatalf("code %d: %s", r.code, r.stderr)
	}
	for _, line := range []string{`^  ~ /a\.js$`, `^  \+ /c\.js$`, `^  - /b\.js$`, `^pushed 3 files to "`} {
		matches(t, r.stdout, line)
	}
	files := w.api.Files(id)
	if files["/a.js"] != "changed" || files["/c.js"] != "new" {
		t.Errorf("remote files = %v", files)
	}
	if _, ok := files["/b.js"]; ok {
		t.Error("/b.js was not deleted")
	}
}

func TestPush_BinaryFiles(t *testing.T) {
	t.Parallel()
	w, id := connected(t, codepentest.Seed{Files: map[string]string{"a.js": "1"}})
	w.writeBytes("img/dot.png", png)
	first := w.run("push")
	matches(t, first.stdout, `\+ /img/dot\.png \(binary\)`)
	data, isBinary, ok := w.api.File(id, "/img/dot.png")
	if !ok || !isBinary || !bytes.Equal(data, png) {
		t.Errorf("remote dot.png = %v, binary %v, found %v", data, isBinary, ok)
	}
	second := w.run("push")
	if second.stdout != "already up to date\n" || writes(w, id) != 1 {
		t.Errorf("second push stdout %q, writes %d; want no write", second.stdout, writes(w, id))
	}
}

func TestPush_SkipsEmptyAndIgnoredFiles(t *testing.T) {
	t.Parallel()
	w, id := connected(t, codepentest.Seed{Files: map[string]string{"a.js": "1"}})
	w.write("empty.js", "")
	w.write("node_modules/dep/index.js", "x")
	w.write(".env.production", "SECRET=1")
	w.write(".envrc", "dotenv")
	r := w.run("push")
	if r.code != 0 || writes(w, id) != 0 || r.stdout != "already up to date\n" {
		t.Errorf("code %d, stdout %q, writes %d", r.code, r.stdout, writes(w, id))
	}
}

func TestPush_RefusesToEmptyPen(t *testing.T) {
	t.Parallel()
	w, id := connected(t, codepentest.Seed{Files: map[string]string{"a.js": "1"}})
	w.remove("a.js")
	w.remove(".codepen")
	r := w.run("push")
	if r.code != 1 || !strings.Contains(r.stderr, "refusing to push") || writes(w, id) != 0 {
		t.Errorf("code %d, stderr %q, writes %d", r.code, r.stderr, writes(w, id))
	}
}

func TestPush_PollsRunningBuild(t *testing.T) {
	t.Parallel()
	w, id := connected(t, codepentest.Seed{Files: map[string]string{"a.js": "1"}})
	w.write("a.js", "2")
	w.api.QueueBuilds(true)
	r := w.run("push")
	if r.code != 0 {
		t.Fatalf("code %d: %s", r.code, r.stderr)
	}
	matches(t, r.stdout, `^pushed 1 file to "`)
	if w.api.Calls(`^GET /v2/builds/`) == 0 {
		t.Error("the build was never polled")
	}
	if got := w.api.Files(id)["/a.js"]; got != "2" {
		t.Errorf("remote a.js = %q", got)
	}
}

// A failed build still saved the files, but the command fails, so scripts notice.
func TestPush_FailedBuild(t *testing.T) {
	t.Parallel()
	w, id := connected(t, codepentest.Seed{Files: map[string]string{"a.js": "1"}})
	w.write("a.js", "2")
	w.api.FailBuilds(codepen.BuildError{Path: "/a.js", Message: "bad", Range: &codepen.Range{Start: codepen.Position{Line: 3}}})
	r := w.run("push")
	if r.code != 1 {
		t.Errorf("code = %d, want 1", r.code)
	}
	matches(t, r.stdout, `^pushed 1 file to "`)
	matches(t, r.stderr, `^  ✗ /a\.js:3 bad$`)
	matches(t, r.stderr, `^✗ the build failed with 1 error; your files are saved on CodePen$`)
	if got := w.api.Files(id)["/a.js"]; got != "2" {
		t.Errorf("remote a.js = %q, want the pushed file", got)
	}
}

// Quiet leaves stdout empty, but errors still reach stderr.
func TestPush_Quiet(t *testing.T) {
	t.Parallel()
	w := newWorkspace(t, map[string]string{"a.js": "1"})
	for _, flag := range []string{"-q", "--quiet"} {
		w.write("a.js", flag)
		if r := w.run("push", flag); r.code != 0 || r.stdout != "" {
			t.Errorf("push %s: code %d, stdout %q; want nothing", flag, r.code, r.stdout)
		}
	}
	w.api.FailBuilds(codepen.BuildError{Path: "/a.js", Message: "bad"})
	w.write("a.js", "broken")
	r := w.run("-q", "push")
	if r.code != 1 || r.stdout != "" {
		t.Errorf("failed build: code %d, stdout %q; want 1 and nothing", r.code, r.stdout)
	}
	matches(t, r.stderr, `✗ /a\.js bad`)
}

func TestPush_DryRun(t *testing.T) {
	t.Parallel()
	w, id := connected(t, codepentest.Seed{Files: map[string]string{"a.js": "1", "b.js": "2"}})
	w.write("a.js", "changed")
	w.write("c.js", "new")
	w.remove("b.js")
	r := w.run("push", "--dry-run")
	if r.code != 0 || writes(w, id) != 0 {
		t.Fatalf("code %d, writes %d, stderr %q; want no write", r.code, writes(w, id), r.stderr)
	}
	for _, line := range []string{`^  ~ /a\.js$`, `^  \+ /c\.js$`, `^  - /b\.js$`, `^would push 3 files to "`} {
		matches(t, r.stdout, line)
	}
	if files := w.api.Files(id); files["/a.js"] != "1" || files["/b.js"] != "2" {
		t.Errorf("remote files = %v, want them untouched", files)
	}
}

func TestPush_DryRunWithoutPen(t *testing.T) {
	t.Parallel()
	w := newWorkspace(t, map[string]string{"a.js": "1", "b.js": "2"})
	r := w.run("push", "--dry-run")
	if r.code != 0 || w.api.Calls(`^POST /v2/pens`) != 0 || w.exists(".codepen.config.json") {
		t.Fatalf("code %d, creates %d, stderr %q; want nothing created", r.code, w.api.Calls(`^POST /v2/pens`), r.stderr)
	}
	matches(t, r.stdout, `^  \+ /a\.js$`)
	matches(t, r.stdout, `^would create a Pen with 2 files$`)
}
