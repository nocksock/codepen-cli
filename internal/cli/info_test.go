package cli

import (
	"regexp"
	"strings"
	"testing"

	"github.com/nocksock/codepen-cli/internal/codepen"
	"github.com/nocksock/codepen-cli/internal/codepen/codepentest"
)

const live = "https://cheerful-term-baboon.codepen.app/"

func TestInfo_InSync(t *testing.T) {
	t.Parallel()
	w, id := connected(t, codepentest.Seed{Title: "Demo", Tags: []string{"api", "demo"}, Files: map[string]string{"a.js": "12345"}})
	r := w.run("info")
	if r.code != 0 {
		t.Fatalf("code %d: %s", r.code, r.stderr)
	}
	for _, line := range []string{
		`^title +Demo$`,
		`^id +` + regexp.QuoteMeta(id) + ` \(V2\)$`,
		`^tags +api, demo$`,
		`^versions +1, head `,
		`^build +Success$`,
		`^files +2 files, 18 B text$`,
		`^local +in sync$`,
		`^deployed +no$`,
		`^editor +https://codepen\.io/editor/`,
	} {
		matches(t, r.stdout, line)
	}
}

func TestInfo_CountsPendingChanges(t *testing.T) {
	t.Parallel()
	w, _ := connected(t, codepentest.Seed{Files: map[string]string{"a.js": "1", "b.js": "2"}})
	w.write("a.js", "changed")
	w.write("c.js", "new")
	w.remove("b.js")
	matches(t, w.run("info").stdout, `^local +2 to upload, 1 to delete \(pen push\)$`)
}

func TestInfo_DeploymentURL(t *testing.T) {
	t.Parallel()
	w, _ := connected(t, codepentest.Seed{Files: map[string]string{"a.js": "1"}, Deployment: &codepen.Deployment{URL: live}})
	matches(t, w.run("info").stdout, `^deployed +`+regexp.QuoteMeta(live)+`$`)
}

func TestInfo_NeedsConnectedPen(t *testing.T) {
	t.Parallel()
	w := newWorkspace(t, nil)
	w.vars["CODEPEN_API_KEY"] = "op://Private/CodePen/API Key"
	r := w.run("info")
	if r.code != 1 {
		t.Errorf("code = %d, want 1", r.code)
	}
	matches(t, r.stderr, `no Pen connected`)
	if calls := w.runner.Calls(); len(calls) != 0 {
		t.Errorf("calls = %q, want none (no key lookup without a Pen)", calls)
	}
}

func TestInfo_Help(t *testing.T) {
	t.Parallel()
	r := newWorkspace(t, nil).run("info", "--help")
	if r.code != 0 || !strings.Contains(r.stdout, "pen info") {
		t.Errorf("code %d, stdout %q, stderr %q", r.code, r.stdout, r.stderr)
	}
}
