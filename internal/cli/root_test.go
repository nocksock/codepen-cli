package cli

import (
	"bytes"
	"context"
	"os"
	"strings"
	"testing"

	"github.com/nocksock/codepen-cli/internal/codepen/codepentest"
)

func TestRun_Help(t *testing.T) {
	t.Parallel()
	r := newWorkspace(t, nil).run("--help")
	if r.code != 0 {
		t.Fatalf("code = %d, want 0", r.code)
	}
	for _, command := range []string{"init", "push", "pull", "sync", "info", "open"} {
		if !strings.Contains(r.stdout, "pen "+command) {
			t.Errorf("usage lacks %q:\n%s", "pen "+command, r.stdout)
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
		{name: "unknown completion shell", args: []string{"completion", "bogus"}, message: `unknown shell "bogus"`},
		{name: "extra completion arg", args: []string{"completion", "zsh", "extra"}, message: "pen completion zsh takes no arguments"},
		{name: "push takes no args", args: []string{"push", "x"}, message: "pen push takes no arguments"},
		{name: "sync takes no args", args: []string{"sync", "x"}, message: "pen sync takes no arguments"},
		{name: "info takes no args", args: []string{"info", "x"}, message: "pen info takes no arguments"},
		{name: "completion without shell", args: []string{"completion"}, message: "completion needs a shell"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			r := newWorkspace(t, nil).run(tt.args...)
			if r.code != 2 {
				t.Errorf("code = %d, want 2", r.code)
			}
			if !strings.Contains(r.stderr, "pen push") || !strings.Contains(r.stderr, tt.message) {
				t.Errorf("stderr lacks usage or %q:\n%s", tt.message, r.stderr)
			}
		})
	}
}

// Not parallel: it swaps os.Args, which Cobra reads when handed nil args.
func TestRun_NoCommandIgnoresProcessArgs(t *testing.T) {
	saved := os.Args
	os.Args = []string{"pen", "bogus"}
	t.Cleanup(func() { os.Args = saved })
	r := newWorkspace(t, nil).run()
	if want := usageText() + "\n"; r.code != 2 || r.stderr != want {
		t.Errorf("code %d, stderr %q, want exactly the usage text", r.code, r.stderr)
	}
}

func TestRun_Version(t *testing.T) {
	t.Parallel()
	r := newWorkspace(t, nil).run("--version")
	if r.code != 0 || !strings.Contains(r.stdout, "pen version") {
		t.Errorf("code %d, stdout %q", r.code, r.stdout)
	}
}

func TestRun_CompletionBash(t *testing.T) {
	t.Parallel()
	r := newWorkspace(t, nil).run("completion", "bash")
	if r.code != 0 || strings.TrimSpace(r.stdout) == "" {
		t.Errorf("code %d, stdout %q", r.code, r.stdout)
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

// An interrupted command exits 130, as shells report Ctrl-C, and stays quiet
// instead of printing the cancellation as an error.
func TestRun_Interrupted(t *testing.T) {
	t.Parallel()
	w, _ := connected(t, codepentest.Seed{Files: map[string]string{"a.js": "1"}})
	w.write("a.js", "2")
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	var stdout, stderr bytes.Buffer
	if code := Run(ctx, []string{"push"}, w.env(&stdout, &stderr)); code != 130 || stderr.Len() != 0 {
		t.Errorf("push = exit %d, stderr %q; want 130 and nothing", code, stderr.String())
	}
}
