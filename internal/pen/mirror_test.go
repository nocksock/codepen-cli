package pen

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nocksock/codepen-cli/internal/codepen"
	"github.com/nocksock/codepen-cli/internal/codepen/codepentest"
)

type recorder struct {
	mu     sync.Mutex
	events []Event
	saved  []string
}

func (r *recorder) Report(e Event) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, e)
}

func (r *recorder) saveID(id string) error {
	r.saved = append(r.saved, id)
	return nil
}

func open(t *testing.T, api *codepentest.Server, root, id string) (*Mirror, *recorder) {
	t.Helper()
	rec := &recorder{}
	client := codepen.New(codepentest.Key, codepen.WithBaseURL(api.URL()), codepen.WithPollInterval(time.Millisecond))
	return Open(Config{API: client, Root: root, ID: id, SaveID: rec.saveID, Reporter: rec}), rec
}

func writeFile(t *testing.T, root, path, content string) {
	t.Helper()
	file := filepath.Join(root, filepath.FromSlash(path))
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestMirror_Push_CreatesPen(t *testing.T) {
	t.Parallel()
	api := codepentest.Start(t)
	root := t.TempDir()
	writeFile(t, root, "index.html", "<h1>x</h1>")
	m, rec := open(t, api, root, "")

	if err := m.Push(t.Context(), PushOptions{}); err != nil {
		t.Fatal(err)
	}
	if len(rec.saved) != 1 || m.ID() != rec.saved[0] {
		t.Fatalf("saved ids = %q, mirror id %q", rec.saved, m.ID())
	}
	if got := api.Files(m.ID())["/index.html"]; got != "<h1>x</h1>" {
		t.Errorf("remote index.html = %q", got)
	}
	if _, ok := rec.events[0].(Created); !ok {
		t.Errorf("first event = %#v, want Created", rec.events[0])
	}
	if _, err := os.Stat(filepath.Join(root, ".codepen", "pen.config.json")); err != nil {
		t.Errorf("compiler patch not written back: %v", err)
	}
}

func TestMirror_Push_NothingChanged(t *testing.T) {
	t.Parallel()
	api := codepentest.Start(t)
	id := api.Seed(codepentest.Seed{Files: map[string]string{"a.js": "1"}})
	root := t.TempDir()
	m, _ := open(t, api, root, "")
	if err := m.Pull(t.Context(), PullOptions{Ref: id}); err != nil {
		t.Fatal(err)
	}
	if err := m.Push(t.Context(), PushOptions{}); err != nil {
		t.Fatal(err)
	}
	if got := api.Calls(`^PUT `); got != 0 {
		t.Errorf("PUT calls = %d, want 0", got)
	}
}

func TestMirror_Push_RefusesEmptyDirectory(t *testing.T) {
	t.Parallel()
	api := codepentest.Start(t)
	id := api.Seed(codepentest.Seed{Files: map[string]string{"a.js": "1"}})
	m, _ := open(t, api, t.TempDir(), id)
	err := m.Push(t.Context(), PushOptions{})
	if err == nil || !strings.Contains(err.Error(), "refusing to push") {
		t.Errorf("Push() error = %v", err)
	}
}

func TestMirror_Pull_SwitchingNeedsForce(t *testing.T) {
	t.Parallel()
	api := codepentest.Start(t)
	first := api.Seed(codepentest.Seed{Files: map[string]string{"a.js": "1"}})
	second := api.Seed(codepentest.Seed{Files: map[string]string{"b.js": "2"}})
	m, rec := open(t, api, t.TempDir(), first)

	err := m.Pull(t.Context(), PullOptions{Ref: second})
	if err == nil || !strings.Contains(err.Error(), "pass --force") {
		t.Fatalf("Pull() error = %v", err)
	}
	if err := m.Pull(t.Context(), PullOptions{Ref: second, Force: true}); err != nil {
		t.Fatal(err)
	}
	if m.ID() != second || len(rec.saved) != 1 || rec.saved[0] != second {
		t.Errorf("id = %q, saved = %q", m.ID(), rec.saved)
	}
}

func TestMirror_Pull_RestoresIDWhenFetchFails(t *testing.T) {
	t.Parallel()
	api := codepentest.Start(t)
	id := api.Seed(codepentest.Seed{Files: map[string]string{"a.js": "1"}})
	m, _ := open(t, api, t.TempDir(), id)
	err := m.Pull(t.Context(), PullOptions{Ref: "01a120bd-0000-7000-8000-000000000000", Force: true})
	if !errors.Is(err, codepen.ErrNotFound) {
		t.Fatalf("Pull() error = %v, want not found", err)
	}
	if m.ID() != id {
		t.Errorf("id = %q after a failed pull, want %q", m.ID(), id)
	}
}

func TestMirror_Pull_RefusesPathsOutsideRoot(t *testing.T) {
	t.Parallel()
	api := codepentest.Start(t)
	id := api.Seed(codepentest.Seed{Files: map[string]string{"../escape.txt": "x"}})
	parent := t.TempDir()
	m, _ := open(t, api, filepath.Join(parent, "work"), "")

	err := m.Pull(t.Context(), PullOptions{Ref: id})
	if err == nil || !strings.Contains(err.Error(), "refusing to write") {
		t.Errorf("Pull() error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(parent, "escape.txt")); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("a remote path escaped the directory")
	}
}

func TestMirror_Pull_SkipsIgnoredRemotePaths(t *testing.T) {
	t.Parallel()
	api := codepentest.Start(t)
	id := api.Seed(codepentest.Seed{Files: map[string]string{".envrc": "export X=1", "a.js": "1"}})
	root := t.TempDir()
	m, _ := open(t, api, root, "")

	if err := m.Pull(t.Context(), PullOptions{Ref: id}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "a.js")); err != nil {
		t.Errorf("a.js not written: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, ".envrc")); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf(".envrc was written (stat error = %v)", err)
	}
}
