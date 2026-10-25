package cli

import (
	"strings"
	"testing"
)

func TestRun_Help(t *testing.T) {
	t.Parallel()
	r := newWorkspace(t, nil).run("--help")
	if r.code != 0 {
		t.Fatalf("code = %d, want 0", r.code)
	}
	for _, command := range []string{"init", "push", "pull", "sync", "info", "open"} {
		if !strings.Contains(r.stdout, "cdpn "+command) {
			t.Errorf("usage lacks %q:\n%s", "cdpn "+command, r.stdout)
		}
	}
}

func TestRun_UsageErrors(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		args    []string
		message string
	}{
		{name: "no command", args: nil, message: ""},
		{name: "unknown command", args: []string{"bogus"}, message: `unknown command "bogus"`},
		{name: "unknown flag", args: []string{"push", "--bogus"}, message: "unknown flag: --bogus"},
		{name: "too many args", args: []string{"pull", "a", "b"}, message: "accepts at most 1 arg"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			r := newWorkspace(t, nil).run(tt.args...)
			if r.code != 2 {
				t.Errorf("code = %d, want 2", r.code)
			}
			if !strings.Contains(r.stderr, "cdpn push") || !strings.Contains(r.stderr, tt.message) {
				t.Errorf("stderr lacks usage or %q:\n%s", tt.message, r.stderr)
			}
		})
	}
}

func TestRun_MissingAPIKey(t *testing.T) {
	t.Parallel()
	w := newWorkspace(t, nil)
	delete(w.vars, "CODEPEN_API_KEY")
	r := w.run("push")
	if r.code != 1 || !strings.Contains(r.stderr, "CODEPEN_API_KEY is not set") {
		t.Errorf("code %d, stderr %q", r.code, r.stderr)
	}
}

func TestRun_InvalidAPIKey(t *testing.T) {
	t.Parallel()
	w := newWorkspace(t, map[string]string{"a.js": "1"})
	w.vars["CODEPEN_API_KEY"] = "cp_user_wrong"
	r := w.run("push")
	if r.code != 1 || !strings.Contains(r.stderr, "401 UNAUTHENTICATED") {
		t.Errorf("code %d, stderr %q", r.code, r.stderr)
	}
}

func TestRun_UnknownPenExplainsScope(t *testing.T) {
	t.Parallel()
	r := newWorkspace(t, nil).run("pull", "01a120bd-0000-7000-8000-000000000000")
	if r.code != 1 || !strings.Contains(r.stderr, "only reaches your own Pens") {
		t.Errorf("code %d, stderr %q", r.code, r.stderr)
	}
}
