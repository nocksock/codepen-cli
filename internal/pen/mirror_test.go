package pen

import (
	"errors"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nocksock/codepen-cli/internal/codepen"
	"github.com/nocksock/codepen-cli/internal/codepen/codepentest"
	"github.com/nocksock/codepen-cli/internal/penconfig"
)

type recorder struct {
	mu     sync.Mutex
	events []Event
}

func (r *recorder) Report(e Event) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, e)
}

func open(t *testing.T, api *codepentest.Server, root, id string) (*Mirror, *recorder) {
	t.Helper()
	rec := &recorder{}
	client := codepen.New(codepentest.Key, codepen.WithBaseURL(api.URL()), codepen.WithPollInterval(time.Millisecond))
	return Open(Config{API: client, Root: root, ID: id, Reporter: rec}), rec
}

// localID is the Pen id stored in root's .codepen.config.json.
func localID(t *testing.T, root string) string {
	t.Helper()
	id, err := penconfig.Read(root)
	if err != nil {
		t.Fatalf("reading the local config: %v", err)
	}
	return id
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
	if m.ID() == "" || localID(t, root) != m.ID() {
		t.Fatalf("local config id = %q, mirror id %q", localID(t, root), m.ID())
	}
	if got := api.Files(m.ID())["/index.html"]; got != "<h1>x</h1>" {
		t.Errorf("remote index.html = %q", got)
	}
	if len(rec.events) == 0 {
		t.Fatal("no events reported")
	}
	if _, ok := rec.events[0].(Created); !ok {
		t.Errorf("first event = %#v, want Created", rec.events[0])
	}
	if _, err := os.Stat(filepath.Join(root, ".codepen", "pen.config.json")); err != nil {
		t.Errorf("compiler patch not written back: %v", err)
	}
}

func TestMirror_Push_CreatesPenFromEmptyFilesWithoutStarter(t *testing.T) {
	t.Parallel()
	api := codepentest.Start(t)
	root := t.TempDir()
	writeFile(t, root, "index.html", "")
	m, _ := open(t, api, root, "")

	if err := m.Push(t.Context(), PushOptions{}); err != nil {
		t.Fatal(err)
	}
	if _, ok := api.Files(m.ID())["/index.html"]; ok {
		t.Error("the Pen got starter files although the directory has files")
	}
	data, err := os.ReadFile(filepath.Join(root, "index.html"))
	if err != nil || len(data) != 0 {
		t.Errorf("local index.html = %q, %v; want it left empty", data, err)
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
	root := t.TempDir()
	m, _ := open(t, api, root, first)

	err := m.Pull(t.Context(), PullOptions{Ref: second})
	if err == nil || !strings.Contains(err.Error(), "pass --force") {
		t.Fatalf("Pull() error = %v", err)
	}
	if err := m.Pull(t.Context(), PullOptions{Ref: second, Force: true}); err != nil {
		t.Fatal(err)
	}
	if m.ID() != second || localID(t, root) != second {
		t.Errorf("id = %q, local config id = %q", m.ID(), localID(t, root))
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

func TestMirror_Pull_IntoMissingDirectory(t *testing.T) {
	t.Parallel()
	api := codepentest.Start(t)
	id := api.Seed(codepentest.Seed{Files: map[string]string{"a.js": "1"}})
	root := filepath.Join(t.TempDir(), "new", "work")
	m, _ := open(t, api, root, "")
	if err := m.Pull(t.Context(), PullOptions{Ref: id}); err != nil {
		t.Fatal(err)
	}
	if data, err := os.ReadFile(filepath.Join(root, "a.js")); err != nil || string(data) != "1" {
		t.Errorf("a.js = %q, %v", data, err)
	}
}

func TestMirror_Pull_RestoresStateWhenSaveFails(t *testing.T) {
	t.Parallel()
	api := codepentest.Start(t)
	first := api.Seed(codepentest.Seed{Files: map[string]string{"a.js": "1"}})
	second := api.Seed(codepentest.Seed{Files: map[string]string{"b.js": "2"}})
	root := t.TempDir()
	writeFile(t, root, ".codepen.config.json", "not json") // the id can't be stored
	m, _ := open(t, api, root, first)

	err := m.Pull(t.Context(), PullOptions{Ref: second, Force: true})
	if err == nil || !strings.Contains(err.Error(), "saving Pen id") {
		t.Fatalf("Pull() error = %v", err)
	}
	if m.ID() != first || m.isTracking {
		t.Errorf("id = %q, tracking = %v, want %q and false", m.ID(), m.isTracking, first)
	}
}

func TestMirror_Push_KeepsIgnoredRemoteFiles(t *testing.T) {
	t.Parallel()
	api := codepentest.Start(t)
	id := api.Seed(codepentest.Seed{Files: map[string]string{".envrc": "export X=1", "a.js": "1"}})
	m, _ := open(t, api, t.TempDir(), "")
	if err := m.Pull(t.Context(), PullOptions{Ref: id}); err != nil {
		t.Fatal(err)
	}
	if err := m.Push(t.Context(), PushOptions{}); err != nil {
		t.Fatal(err)
	}
	if got := api.Calls(`^PUT `); got != 0 {
		t.Errorf("PUT calls = %d, want 0", got)
	}
	if _, ok := api.Files(id)["/.envrc"]; !ok {
		t.Error(".envrc was deleted remotely")
	}
}

func TestMirror_Push_AfterCreate(t *testing.T) {
	t.Parallel()
	api := codepentest.Start(t)
	root := t.TempDir()
	writeFile(t, root, "index.html", "<h1>x</h1>")
	m, _ := open(t, api, root, "")
	if err := m.Push(t.Context(), PushOptions{}); err != nil {
		t.Fatal(err)
	}
	if err := m.Push(t.Context(), PushOptions{}); err != nil {
		t.Fatal(err)
	}
	if puts, gets := api.Calls(`^PUT `), api.Calls(`^GET /v2/pens/[^/]+$`); puts != 0 || gets != 0 {
		t.Errorf("PUT = %d, GET pen = %d, want 0 and 0", puts, gets)
	}
	writeFile(t, root, "index.html", "<h1>y</h1>")
	if err := m.Push(t.Context(), PushOptions{}); err != nil {
		t.Fatal(err)
	}
	if got := api.Calls(`^PUT `); got != 1 {
		t.Errorf("PUT calls = %d, want 1", got)
	}
}

func TestMirror_IDStaysLocal(t *testing.T) {
	t.Parallel()
	api := codepentest.Start(t)
	id := api.Seed(codepentest.Seed{Files: map[string]string{"a.js": "1"}})
	root := t.TempDir()
	m, _ := open(t, api, root, "")
	if err := m.Pull(t.Context(), PullOptions{Ref: id}); err != nil {
		t.Fatal(err)
	}
	if got := localID(t, root); got != id {
		t.Fatalf("local config id = %q, want %q", got, id)
	}
	if err := m.Push(t.Context(), PushOptions{}); err != nil {
		t.Fatal(err)
	}
	if got := api.Calls(`^PUT `); got != 0 {
		t.Errorf("PUT calls = %d: the local id must not count as a change", got)
	}
	writeFile(t, root, "a.js", "2")
	if err := m.Push(t.Context(), PushOptions{}); err != nil {
		t.Fatal(err)
	}
	if _, ok := api.Files(id)["/"+penconfig.Name]; ok {
		t.Errorf("%s was pushed", penconfig.Name)
	}
	if got := localID(t, root); got != id {
		t.Errorf("local config id after push = %q, want %q", got, id)
	}
}

func TestMirror_Pull_IgnoresRemoteIDFile(t *testing.T) {
	t.Parallel()
	api := codepentest.Start(t)
	id := api.Seed(codepentest.Seed{Files: map[string]string{
		"a.js":         "1",
		penconfig.Name: `{"id":"someone-elses-pen"}`,
	}})
	root := t.TempDir()
	m, _ := open(t, api, root, "")
	if err := m.Pull(t.Context(), PullOptions{Ref: id}); err != nil {
		t.Fatal(err)
	}
	if got := localID(t, root); got != id {
		t.Errorf("local config id = %q, want %q (a fork's id must not win)", got, id)
	}
}

// failingBuilds fails requests for builds while isFailing is set.
type failingBuilds struct{ isFailing atomic.Bool }

func (f *failingBuilds) RoundTrip(r *http.Request) (*http.Response, error) {
	if f.isFailing.Load() && strings.Contains(r.URL.Path, "/builds/") {
		return nil, errors.New("connection reset")
	}
	return http.DefaultTransport.RoundTrip(r)
}

// A push whose write went through but whose build couldn't be followed leaves
// the tracked head behind. The next push must catch up, not conflict forever.
func TestMirror_Push_RecoversAfterLosingTheBuild(t *testing.T) {
	t.Parallel()
	api := codepentest.Start(t)
	api.QueueBuilds(true)
	id := api.Seed(codepentest.Seed{Files: map[string]string{"a.js": "1"}})
	root := t.TempDir()
	builds := &failingBuilds{}
	client := codepen.New(codepentest.Key,
		codepen.WithBaseURL(api.URL()),
		codepen.WithPollInterval(time.Millisecond),
		codepen.WithBackoff(time.Millisecond),
		codepen.WithHTTPClient(&http.Client{Transport: builds}),
	)
	m := Open(Config{API: client, Root: root, ID: id, Reporter: &recorder{}})
	if err := m.Pull(t.Context(), PullOptions{}); err != nil {
		t.Fatal(err)
	}

	builds.isFailing.Store(true)
	writeFile(t, root, "a.js", "2")
	if err := m.Push(t.Context(), PushOptions{}); err == nil {
		t.Fatal("Push() succeeded although the build couldn't be followed")
	}

	builds.isFailing.Store(false)
	writeFile(t, root, "a.js", "3")
	if err := m.Push(t.Context(), PushOptions{}); err != nil {
		t.Fatalf("Push() after a lost build: %v", err)
	}
	if got := api.Files(id)["/a.js"]; got != "3" {
		t.Errorf("remote a.js = %q, want 3", got)
	}
}

// A conflict means someone else changed the Pen; later pushes must keep
// refusing instead of quietly overwriting their edit.
func TestMirror_Push_KeepsRefusingAfterConflict(t *testing.T) {
	t.Parallel()
	api := codepentest.Start(t)
	id := api.Seed(codepentest.Seed{Files: map[string]string{"a.js": "1"}})
	root := t.TempDir()
	m, _ := open(t, api, root, id)
	if err := m.Pull(t.Context(), PullOptions{}); err != nil {
		t.Fatal(err)
	}
	api.Edit(id, map[string]string{"a.js": "theirs"})
	for _, content := range []string{"2", "3"} {
		writeFile(t, root, "a.js", content)
		if err := m.Push(t.Context(), PushOptions{}); !errors.Is(err, codepen.ErrConflict) {
			t.Fatalf("Push(%s) = %v, want ErrConflict", content, err)
		}
	}
	if got := api.Files(id)["/a.js"]; got != "theirs" {
		t.Errorf("remote a.js = %q, want theirs", got)
	}
}
