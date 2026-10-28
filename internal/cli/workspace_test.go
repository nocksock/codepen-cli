package cli

import (
	"bytes"
	"context"
	"io"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"go.uber.org/goleak"

	"github.com/nocksock/codepen-cli/internal/codepen/codepentest"
	"github.com/nocksock/codepen-cli/internal/penconfig"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m,
		// Idle keep-alive connections of the API client wind down on their own.
		goleak.IgnoreAnyFunction("net/http.(*persistConn).readLoop"),
		goleak.IgnoreAnyFunction("net/http.(*persistConn).writeLoop"),
	)
}

// workspace is a temporary working directory plus a fresh fake API.
type workspace struct {
	t      *testing.T
	dir    string
	api    *codepentest.Server
	vars   map[string]string
	runner *recorder
}

func newWorkspace(t *testing.T, files map[string]string) *workspace {
	t.Helper()
	api := codepentest.Start(t)
	w := &workspace{
		t:      t,
		dir:    t.TempDir(),
		api:    api,
		runner: &recorder{missing: map[string]bool{}},
		vars: map[string]string{
			"CODEPEN_API_KEY": codepentest.Key,
			"CODEPEN_API_URL": api.URL(),
			"CODEPEN_POLL_MS": "5",
		},
	}
	for p, content := range files {
		w.write(p, content)
	}
	return w
}

// connected returns a workspace connected to a fresh Pen via `pen pull`.
func connected(t *testing.T, seed codepentest.Seed) (*workspace, string) {
	t.Helper()
	w := newWorkspace(t, nil)
	id := w.api.Seed(seed)
	if r := w.run("pull", id); r.code != 0 {
		t.Fatalf("setup pull failed: %s", r.stderr)
	}
	return w, id
}

type result struct {
	code   int
	stdout string
	stderr string
}

func (w *workspace) env(stdout, stderr io.Writer) Env {
	vars := maps.Clone(w.vars) // a running session must not read a map the test mutates
	return Env{
		Dir: w.dir,
		Getenv: func(name string) (string, bool) {
			v, ok := vars[name]
			return v, ok
		},
		Stdout: stdout,
		Stderr: stderr,
		Runner: w.runner,
		GOOS:   "linux",
	}
}

func (w *workspace) run(args ...string) result {
	w.t.Helper()
	var stdout, stderr bytes.Buffer
	code := Run(w.t.Context(), args, w.env(&stdout, &stderr))
	return result{code: code, stdout: stdout.String(), stderr: stderr.String()}
}

func (w *workspace) path(p string) string { return filepath.Join(w.dir, filepath.FromSlash(p)) }

func (w *workspace) write(p, content string) { w.writeBytes(p, []byte(content)) }

func (w *workspace) writeBytes(p string, data []byte) {
	w.t.Helper()
	if err := os.MkdirAll(filepath.Dir(w.path(p)), 0o755); err != nil {
		w.t.Fatal(err)
	}
	if err := os.WriteFile(w.path(p), data, 0o644); err != nil {
		w.t.Fatal(err)
	}
}

func (w *workspace) read(p string) string {
	w.t.Helper()
	data, err := os.ReadFile(w.path(p))
	if err != nil {
		w.t.Fatal(err)
	}
	return string(data)
}

func (w *workspace) remove(p string) {
	w.t.Helper()
	if err := os.RemoveAll(w.path(p)); err != nil {
		w.t.Fatal(err)
	}
}

// penID is the Pen id stored in the workspace's .codepen.config.json.
func (w *workspace) penID() string {
	w.t.Helper()
	id, err := penconfig.ID([]byte(w.read(penconfig.Name)))
	if err != nil {
		w.t.Fatal(err)
	}
	return id
}

func (w *workspace) exists(p string) bool {
	_, err := os.Stat(w.path(p))
	return err == nil
}

// recorder stands in for the browser, recording each call.
type recorder struct {
	mu      sync.Mutex
	calls   []string
	missing map[string]bool // programs that are "not installed"
}

func (r *recorder) Start(name string, args ...string) error { return r.record(name, args) }

func (r *recorder) record(name string, args []string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls = append(r.calls, strings.Join(append([]string{name}, args...), " "))
	if r.missing[name] {
		return exec.ErrNotFound
	}
	return nil
}

func (r *recorder) Calls() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string{}, r.calls...)
}

// session is a long-running command (sync) with its combined output.
type session struct {
	out    *lockedBuffer
	cancel context.CancelFunc
	done   chan int
	once   sync.Once
	code   int
	t      *testing.T
}

func (w *workspace) start(args ...string) *session {
	ctx, cancel := context.WithCancel(context.Background())
	s := &session{out: &lockedBuffer{}, cancel: cancel, done: make(chan int, 1), t: w.t}
	go func() { s.done <- Run(ctx, args, w.env(s.out, s.out)) }()
	w.t.Cleanup(func() { s.stop() })
	return s
}

// stop cancels the command and returns its exit code.
func (s *session) stop() int {
	s.once.Do(func() {
		s.cancel()
		select {
		case s.code = <-s.done:
		case <-time.After(5 * time.Second):
			s.t.Error("command did not stop within 5s of cancellation")
		}
	})
	return s.code
}

func (s *session) waitFor(t *testing.T, pattern string) {
	t.Helper()
	re := regexp.MustCompile(pattern)
	eventually(t, func() bool { return re.MatchString(s.out.String()) },
		func() string { return "output matching " + pattern + "; got:\n" + s.out.String() })
}

func eventually(t *testing.T, condition func() bool, what func() string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !condition() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what())
		}
		time.Sleep(10 * time.Millisecond)
	}
}

type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// matches fails unless text has a line matching pattern.
func matches(t *testing.T, text, pattern string) {
	t.Helper()
	if !regexp.MustCompile("(?m)" + pattern).MatchString(text) {
		t.Errorf("no match for %q in:\n%s", pattern, text)
	}
}
