package cli

import (
	"strings"
	"testing"

	"github.com/nocksock/codepen-cli/internal/codepen"
	"github.com/nocksock/codepen-cli/internal/pen"
)

func TestRender(t *testing.T) {
	t.Parallel()
	urls := codepen.URLs{Editor: "https://codepen.io/editor/x"}
	tests := []struct {
		name  string
		event pen.Event
		want  string
	}{
		{
			name: "planned",
			event: pen.Planned{Changes: codepen.ChangeSet{
				Writes:  []codepen.Write{{Path: "/a.js"}, {Path: "/new.js"}},
				Deletes: []string{"/b.js"},
				Uploads: []codepen.Upload{{Path: "/c.png"}, {Path: "/new.png"}},
			}, New: map[string]bool{"/new.js": true, "/new.png": true}},
			want: "  ~ /a.js\n  + /new.js\n  ~ /c.png (binary)\n  + /new.png (binary)\n  - /b.js\n",
		},
		{
			name: "built with patches",
			event: pen.Built{Build: codepen.Build{
				Status:         "Success",
				AppliedPatches: []codepen.Patch{{Type: "create", Path: "/.codepen/pen.config.json"}},
			}},
			want: "  patched create /.codepen/pen.config.json\n",
		},
		{name: "downloaded", event: pen.Downloaded{Path: "/a.js"}, want: "  ↓ /a.js\n"},
		{name: "kept", event: pen.Kept{Path: "/a.js"}, want: "  kept /a.js (local edit; push uploads it)\n"},
		{name: "created", event: pen.Created{Title: "Demo", URLs: urls}, want: "created \"Demo\"\n  editor  https://codepen.io/editor/x\n"},
		{name: "connected", event: pen.Connected{Title: "Demo", ID: "p1", URLs: urls}, want: "connected to \"Demo\" (p1)\n  editor  https://codepen.io/editor/x\n"},
		{name: "pushed one", event: pen.Pushed{Title: "Demo", Files: 1}, want: "pushed 1 file to \"Demo\"\n"},
		{name: "pushed many", event: pen.Pushed{Title: "Demo", Files: 3}, want: "pushed 3 files to \"Demo\"\n"},
		{name: "forked", event: pen.Forked{From: "Demo", Title: "Demo", ID: "p2"}, want: "forked \"Demo\" → \"Demo\" (p2)\n"},
		{name: "pulled", event: pen.Pulled{Files: 2}, want: "pulled 2 files\n"},
		{name: "up to date", event: pen.UpToDate{}, want: "already up to date\n"},
		{name: "title is cleaned", event: pen.Pushed{Title: "a\x1b[2J", Files: 1}, want: "pushed 1 file to \"a\\x1b[2J\"\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := render(tt.event); got != tt.want {
				t.Errorf("render = %q, want %q", got, tt.want)
			}
		})
	}
}

// Build errors are problems, so they go to stderr with the other errors.
func TestPrinter_BuildErrorsGoToStderr(t *testing.T) {
	t.Parallel()
	var out, errOut strings.Builder
	printer{w: &out, err: &errOut}.Report(pen.Built{Build: codepen.Build{
		Status: codepen.BuildFailed,
		Errors: []codepen.BuildError{
			{Path: "/a.js", Message: "bad", Range: &codepen.Range{Start: codepen.Position{Line: 3}}},
			{Path: "/b\x1b[2J.js", Message: "line\nfake"},
		},
	}})
	if out.String() != "" {
		t.Errorf("stdout = %q, want nothing", out.String())
	}
	if want := "  ✗ /a.js:3 bad\n  ✗ /b\\x1b[2J.js line\\nfake\n"; errOut.String() != want {
		t.Errorf("stderr = %q, want %q", errOut.String(), want)
	}
}
