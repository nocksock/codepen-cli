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
	if r.code != 0 {
		t.Errorf("code = %d, want 0; stderr %q", r.code, r.stderr)
	}
}
