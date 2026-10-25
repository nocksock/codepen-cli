package cli

import (
	"slices"
	"strings"
	"testing"

	"github.com/nocksock/codepen-cli/internal/codepen"
	"github.com/nocksock/codepen-cli/internal/codepen/codepentest"
)

func TestOpen_LaunchesTargets(t *testing.T) {
	t.Parallel()
	w, id := connected(t, codepentest.Seed{Files: map[string]string{"a.js": "1"}, Deployment: &codepen.Deployment{URL: live}})
	urls := w.api.URLs(id)
	tests := []struct {
		args     []string
		expected string
	}{
		{args: nil, expected: urls.Editor},
		{args: []string{"preview"}, expected: urls.Preview},
		{args: []string{"live"}, expected: live},
	}
	for _, tt := range tests {
		r := w.run(append([]string{"open"}, tt.args...)...)
		if r.code != 0 || strings.TrimSpace(r.stdout) != tt.expected {
			t.Errorf("open %v: code %d, stdout %q, want %q", tt.args, r.code, r.stdout, tt.expected)
		}
	}
	expected := []string{"xdg-open " + urls.Editor, "xdg-open " + urls.Preview, "xdg-open " + live}
	if calls := w.runner.Calls(); !slices.Equal(calls, expected) {
		t.Errorf("calls = %q, want %q", calls, expected)
	}
}

func TestOpen_Failures(t *testing.T) {
	t.Parallel()
	w, _ := connected(t, codepentest.Seed{Files: map[string]string{"a.js": "1"}})
	undeployed := w.run("open", "live")
	if undeployed.code != 1 || !strings.Contains(undeployed.stderr, "no live URL") {
		t.Errorf("open live: code %d, stderr %q", undeployed.code, undeployed.stderr)
	}
	bogus := w.run("open", "bogus")
	if bogus.code != 2 || !strings.Contains(bogus.stderr, `unknown target "bogus"`) {
		t.Errorf("open bogus: code %d, stderr %q", bogus.code, bogus.stderr)
	}
	if calls := w.runner.Calls(); len(calls) != 0 {
		t.Errorf("calls = %q, want none", calls)
	}
}

func TestOpen_NeedsConnectedPen(t *testing.T) {
	t.Parallel()
	r := newWorkspace(t, nil).run("open")
	if r.code != 1 || !strings.Contains(r.stderr, "no Pen connected") {
		t.Errorf("code %d, stderr %q", r.code, r.stderr)
	}
}

func TestOpen_Help(t *testing.T) {
	t.Parallel()
	r := newWorkspace(t, nil).run("open", "--help")
	if r.code != 0 || !strings.Contains(r.stdout, "open [editor|preview|live]") {
		t.Errorf("code %d, stdout %q, stderr %q", r.code, r.stdout, r.stderr)
	}
}

func TestOpen_BrowserLaunchFails(t *testing.T) {
	t.Parallel()
	w, id := connected(t, codepentest.Seed{Files: map[string]string{"a.js": "1"}})
	w.runner.missing["xdg-open"] = true
	url := w.api.URLs(id).Editor
	r := w.run("open")
	want := url + "\ncould not launch a browser; open " + url + "\n"
	if r.code != 0 || r.stdout != want || r.stderr != "" {
		t.Errorf("code %d, stdout %q, stderr %q, want stdout %q", r.code, r.stdout, r.stderr, want)
	}
}

func TestOpen_RefusesNonHTTPURLs(t *testing.T) {
	t.Parallel()
	w, _ := connected(t, codepentest.Seed{Files: map[string]string{"a.js": "1"}, Deployment: &codepen.Deployment{URL: "file:///etc/passwd"}})
	r := w.run("open", "live")
	if r.code != 1 || !strings.Contains(r.stderr, "not an http(s) url") || r.stdout != "" {
		t.Errorf("code %d, stdout %q, stderr %q", r.code, r.stdout, r.stderr)
	}
	if calls := w.runner.Calls(); len(calls) != 0 {
		t.Errorf("calls = %q, want none", calls)
	}
}
