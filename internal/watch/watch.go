// Package watch tells when files below a directory change. Events are only a
// hint: they are debounced into one signal, and a periodic rescan of file
// sizes and mtimes catches anything the OS failed to report.
package watch

import (
	"context"
	"errors"
	"fmt"
	"hash/fnv"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/fsnotify/fsnotify"
)

// Options tune Changes.
type Options struct {
	Skip     func(path string) bool // paths ("/a/b", relative to the directory) to ignore
	Debounce time.Duration          // quiet time before a signal (default 400ms)
	Rescan   time.Duration          // fingerprint interval (default 5s)
}

func (o Options) withDefaults() Options {
	if o.Skip == nil {
		o.Skip = func(string) bool { return false }
	}
	if o.Debounce <= 0 {
		o.Debounce = 400 * time.Millisecond
	}
	if o.Rescan <= 0 {
		o.Rescan = 5 * time.Second
	}
	return o
}

// Changes watches dir and its subdirectories until ctx is done, then closes the
// returned channel. Each value means "something changed"; bursts collapse into
// one value, and one is kept while the receiver is busy.
func Changes(ctx context.Context, dir string, opts Options) (<-chan struct{}, error) {
	root, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return nil, fmt.Errorf("resolving %q: %w", dir, err)
	}
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("starting the file watcher: %w", err)
	}
	t := &tracker{dir: root, opts: opts.withDefaults(), watcher: watcher}
	if err := t.addTree(root); err != nil {
		_ = watcher.Close() // the addTree error is the one worth reporting
		return nil, err
	}
	t.last = t.fingerprint()
	out := make(chan struct{}, 1)
	go t.run(ctx, out)
	return out, nil
}

type tracker struct {
	dir     string
	opts    Options
	watcher *fsnotify.Watcher
	last    uint64 // fingerprint at the last signal
}

func (t *tracker) run(ctx context.Context, out chan<- struct{}) {
	defer close(out)
	defer func() { _ = t.watcher.Close() }() // nothing to do about a close error on shutdown
	debounce := time.NewTimer(t.opts.Debounce)
	debounce.Stop()
	defer debounce.Stop()
	rescan := time.NewTicker(t.opts.Rescan)
	defer rescan.Stop()
	events, errs := t.watcher.Events, t.watcher.Errors
	for {
		select {
		case <-ctx.Done():
			return
		case event, ok := <-events:
			if !ok {
				events = nil // watcher gone: rescans carry on
				continue
			}
			if t.isRelevant(event) {
				debounce.Reset(t.opts.Debounce)
			}
		case _, ok := <-errs:
			// Overflow or a failed watch: events may be missing, rescans catch up.
			if !ok {
				errs = nil
				continue
			}
			_ = t.addTree(t.dir) // a lost Create may have left a directory unwatched
			debounce.Reset(t.opts.Debounce)
		case <-debounce.C:
			t.last = t.fingerprint()
			notify(out)
		case <-rescan.C:
			if fp := t.fingerprint(); fp != t.last {
				t.last = fp
				debounce.Stop()      // this signal already covers any pending event
				_ = t.addTree(t.dir) // best effort: picks up directories events missed
				notify(out)
			}
		}
	}
}

// isRelevant reports whether event should trigger a signal, and starts
// watching directories as they appear.
func (t *tracker) isRelevant(event fsnotify.Event) bool {
	if event.Op == fsnotify.Chmod || t.isSkipped(event.Name) {
		return false
	}
	if event.Has(fsnotify.Create) {
		if info, err := os.Stat(event.Name); err == nil && info.IsDir() {
			_ = t.addTree(event.Name) // a failure here is covered by the rescan
		}
	}
	return true
}

// notify sends a signal unless one is already waiting.
func notify(out chan<- struct{}) {
	select {
	case out <- struct{}{}:
	default:
	}
}

// addTree watches root and every directory below it that isn't skipped.
func (t *tracker) addTree(root string) error {
	return filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			if path == root {
				return fmt.Errorf("watching %q: %w", path, err)
			}
			return nil //nolint:nilerr // vanished while walking; the rescan catches up
		}
		if !entry.IsDir() {
			return nil
		}
		if path != t.dir && t.isSkipped(path) {
			return fs.SkipDir
		}
		if err := t.watcher.Add(path); err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return fs.SkipDir // vanished between the walk and the watch
			}
			return fmt.Errorf("watching %q: %w", path, err)
		}
		return nil
	})
}

func (t *tracker) isSkipped(file string) bool {
	rel, err := filepath.Rel(t.dir, file)
	if err != nil || rel == "." {
		return false
	}
	return t.opts.Skip("/" + filepath.ToSlash(rel))
}

// fingerprint hashes the paths, sizes and mtimes of all files that aren't skipped.
func (t *tracker) fingerprint() uint64 {
	h := fnv.New64a()
	// The callback never returns an error, so neither does WalkDir.
	_ = filepath.WalkDir(t.dir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil || path == t.dir {
			return nil //nolint:nilerr // unreadable entries are left out of the fingerprint
		}
		if t.isSkipped(path) {
			if entry.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return nil //nolint:nilerr // vanished mid-walk; the next scan sees it gone
		}
		_, _ = fmt.Fprintf(h, "%s\x00%d\x00%d\n", path, info.Size(), info.ModTime().UnixNano())
		return nil
	})
	return h.Sum64()
}
