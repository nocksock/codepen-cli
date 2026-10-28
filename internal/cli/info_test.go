package cli

import (
	"encoding/json"
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
	r := w.run("info")
	if r.code != 1 {
		t.Errorf("code = %d, want 1", r.code)
	}
	matches(t, r.stderr, `no Pen connected`)
	if calls := w.runner.Calls(); len(calls) != 0 {
		t.Errorf("calls = %q, want none", calls)
	}
}

func TestInfo_Help(t *testing.T) {
	t.Parallel()
	r := newWorkspace(t, nil).run("info", "--help")
	if r.code != 0 || !strings.Contains(r.stdout, "pen info") {
		t.Errorf("code %d, stdout %q, stderr %q", r.code, r.stdout, r.stderr)
	}
}

// Quiet only silences progress; what was asked for still prints.
func TestInfo_Quiet(t *testing.T) {
	t.Parallel()
	w, _ := connected(t, codepentest.Seed{Files: map[string]string{"a.js": "1"}})
	matches(t, w.run("info", "-q").stdout, `^local +in sync$`)
}

func TestInfo_JSON(t *testing.T) {
	t.Parallel()
	w, id := connected(t, codepentest.Seed{Title: "Demo", Tags: []string{"api"}, Files: map[string]string{"a.js": "12345", "b.js": "1"}})
	w.write("a.js", "changed")
	w.remove("b.js")
	r := w.run("info", "--json")
	if r.code != 0 {
		t.Fatalf("code %d: %s", r.code, r.stderr)
	}
	var got struct {
		ID       string   `json:"id"`
		Title    string   `json:"title"`
		Tags     []string `json:"tags"`
		Versions int      `json:"versions"`
		Build    struct {
			Status string `json:"status"`
		} `json:"build"`
		Files struct {
			Count int `json:"count"`
		} `json:"files"`
		Local struct {
			Upload []string `json:"upload"`
			Delete []string `json:"delete"`
		} `json:"local"`
		DeployedURL *string `json:"deployed_url"`
		URLs        struct {
			Editor string `json:"editor"`
		} `json:"urls"`
	}
	if err := json.Unmarshal([]byte(r.stdout), &got); err != nil {
		t.Fatalf("stdout is not JSON: %v\n%s", err, r.stdout)
	}
	if got.ID != id || got.Title != "Demo" || len(got.Tags) != 1 || got.Versions != 1 || got.Build.Status != "Success" || got.Files.Count != 3 {
		t.Errorf("info = %+v", got)
	}
	if len(got.Local.Upload) != 1 || got.Local.Upload[0] != "/a.js" || len(got.Local.Delete) != 1 || got.Local.Delete[0] != "/b.js" {
		t.Errorf("local = %+v, want /a.js to upload and /b.js to delete", got.Local)
	}
	if got.DeployedURL != nil || !strings.HasPrefix(got.URLs.Editor, "https://codepen.io/editor/") {
		t.Errorf("deployed %v, urls %+v", got.DeployedURL, got.URLs)
	}
}
