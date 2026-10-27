package cli

import (
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
				Writes:  []codepen.Write{{Path: "/a.js"}},
				Deletes: []string{"/b.js"},
				Uploads: []codepen.Upload{{Path: "/c.png"}},
			}},
			want: "  ~ /a.js\n  - /b.js\n  ~ /c.png (binary)\n",
		},
		{
			name: "built with patches and errors",
			event: pen.Built{Build: codepen.Build{
				Status:         "Success",
				AppliedPatches: []codepen.Patch{{Type: "create", Path: "/.codepen/pen.config.json"}},
				Errors: []codepen.BuildError{
					{Path: "/a.js", Message: "bad", Range: &codepen.Range{Start: codepen.Position{Line: 3}}},
					{Path: "/b.js", Message: "worse"},
					{Message: "no path"},
				},
			}},
			want: "  patched create /.codepen/pen.config.json\n" +
				"  ✗ /a.js:3 bad\n" +
				"  ✗ /b.js worse\n" +
				"  ✗  no path\n" +
				"  build Success\n",
		},
		{name: "downloaded", event: pen.Downloaded{Path: "/a.js"}, want: "  ↓ /a.js\n"},
		{
			name: "remote strings are cleaned",
			event: pen.Built{Build: codepen.Build{
				Status: "Failed\x1b[0m",
				Errors: []codepen.BuildError{{Path: "/a\x1b[2J.js", Message: "line\nfake"}},
			}},
			want: "  ✗ /a\\x1b[2J.js line\\nfake\n  build Failed\\x1b[0m\n",
		},
		{name: "created", event: pen.Created{URLs: urls}, want: "created\n"},
		{name: "connected", event: pen.Connected{URLs: urls}, want: "connected\n"},
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
