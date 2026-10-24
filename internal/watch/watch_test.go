package watch

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

var fast = Options{Debounce: 30 * time.Millisecond, Rescan: time.Hour}

func start(t *testing.T, dir string, opts Options) <-chan struct{} {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	changes, err := Changes(ctx, dir, opts)
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cancel()
		for range changes { // wait until the watcher has shut down
		}
	})
	return changes
}

func expectSignal(t *testing.T, changes <-chan struct{}) {
	t.Helper()
	select {
	case _, ok := <-changes:
		if !ok {
			t.Fatal("channel closed")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("no change signalled")
	}
}

func expectQuiet(t *testing.T, changes <-chan struct{}, d time.Duration) {
	t.Helper()
	select {
	case <-changes:
		t.Fatal("unexpected change signalled")
	case <-time.After(d):
	}
}

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestChanges(t *testing.T) {
	t.Parallel()

	t.Run("signals a new file", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		changes := start(t, dir, fast)
		write(t, filepath.Join(dir, "a.js"), "1")
		expectSignal(t, changes)
	})

	t.Run("watches new subdirectories", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		changes := start(t, dir, fast)
		if err := os.Mkdir(filepath.Join(dir, "src"), 0o755); err != nil {
			t.Fatal(err)
		}
		expectSignal(t, changes)
		write(t, filepath.Join(dir, "src", "a.js"), "1")
		expectSignal(t, changes)
	})

	t.Run("ignores skipped paths", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		if err := os.Mkdir(filepath.Join(dir, "node_modules"), 0o755); err != nil {
			t.Fatal(err)
		}
		opts := fast
		opts.Skip = func(p string) bool { return strings.Contains(p, "node_modules") || p == "/.env.local" }
		changes := start(t, dir, opts)
		write(t, filepath.Join(dir, "node_modules", "dep.js"), "x")
		write(t, filepath.Join(dir, ".env.local"), "CODEPEN_ID=x")
		expectQuiet(t, changes, 300*time.Millisecond)
	})

	t.Run("collapses a burst into one signal", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		changes := start(t, dir, Options{Debounce: 100 * time.Millisecond, Rescan: time.Hour})
		for _, name := range []string{"a", "b", "c", "d", "e"} {
			write(t, filepath.Join(dir, name+".js"), name)
		}
		expectSignal(t, changes)
		expectQuiet(t, changes, 300*time.Millisecond)
	})

	t.Run("closes the channel when ctx is done", func(t *testing.T) {
		t.Parallel()
		ctx, cancel := context.WithCancel(context.Background())
		changes, err := Changes(ctx, t.TempDir(), fast)
		if err != nil {
			t.Fatal(err)
		}
		cancel()
		select {
		case _, ok := <-changes:
			if ok {
				t.Fatal("got a signal instead of a close")
			}
		case <-time.After(3 * time.Second):
			t.Fatal("channel not closed")
		}
	})

	t.Run("fails for a missing directory", func(t *testing.T) {
		t.Parallel()
		if _, err := Changes(t.Context(), filepath.Join(t.TempDir(), "missing"), fast); err == nil {
			t.Fatal("Changes(missing) succeeded")
		}
	})
}

func TestTracker_Fingerprint(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	write(t, filepath.Join(dir, "a.js"), "1")
	tr := &tracker{dir: dir, opts: Options{Skip: func(p string) bool { return p == "/ignored.js" }}.withDefaults()}
	before := tr.fingerprint()

	write(t, filepath.Join(dir, "ignored.js"), "x")
	if tr.fingerprint() != before {
		t.Error("a skipped file changed the fingerprint")
	}
	write(t, filepath.Join(dir, "a.js"), "22")
	if tr.fingerprint() == before {
		t.Error("a changed file left the fingerprint as it was")
	}
}
