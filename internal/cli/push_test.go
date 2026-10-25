package cli

import (
	"bytes"
	"encoding/hex"
	"regexp"
	"strings"
	"testing"

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
	if !strings.HasPrefix(r.stdout, "created https://codepen.io/editor/") {
		t.Errorf("stdout = %q", r.stdout)
	}
	match := regexp.MustCompile(`(?m)^CODEPEN_ID=(.+)$`).FindStringSubmatch(w.read(".env.local"))
	if match == nil {
		t.Fatalf(".env.local = %q", w.read(".env.local"))
	}
	files := w.api.Files(match[1])
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
	if r.code != 0 || r.stdout != "" || writes(w, id) != 0 {
		t.Errorf("code %d, stdout %q, writes %d", r.code, r.stdout, writes(w, id))
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
	for _, line := range []string{`~ /a\.js`, `~ /c\.js`, `- /b\.js`} {
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
	matches(t, first.stdout, `~ /img/dot\.png \(binary\)`)
	data, isBinary, ok := w.api.File(id, "/img/dot.png")
	if !ok || !isBinary || !bytes.Equal(data, png) {
		t.Errorf("remote dot.png = %v, binary %v, found %v", data, isBinary, ok)
	}
	second := w.run("push")
	if second.stdout != "" || writes(w, id) != 1 {
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
	if r.code != 0 || r.stdout != "" || writes(w, id) != 0 {
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
	matches(t, r.stdout, `build Success`)
	if w.api.Calls(`^GET /v2/builds/`) == 0 {
		t.Error("the build was never polled")
	}
	if got := w.api.Files(id)["/a.js"]; got != "2" {
		t.Errorf("remote a.js = %q", got)
	}
}
