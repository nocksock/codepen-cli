// Package pen mirrors a local directory to a CodePen Pen: push sends local
// changes up, pull brings remote files down.
package pen

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"path/filepath"
	"slices"
	"strings"

	"golang.org/x/sync/errgroup"

	"github.com/nocksock/codepen-cli/internal/codepen"
	"github.com/nocksock/codepen-cli/internal/diff"
	"github.com/nocksock/codepen-cli/internal/human"
	"github.com/nocksock/codepen-cli/internal/penconfig"
	"github.com/nocksock/codepen-cli/internal/penstate"
	"github.com/nocksock/codepen-cli/internal/ref"
	"github.com/nocksock/codepen-cli/internal/tree"
)

// Config is what a Mirror needs.
type Config struct {
	API      *codepen.Client
	Root     string         // absolute directory to mirror
	ID       string         // connected Pen; "" when none
	Reporter Reporter       // required
	State    penstate.Store // where the last sync is remembered
}

// Mirror is a local directory mirrored to a Pen. Between calls it remembers the
// remote head, so repeated pushes only talk to the API when something changed.
// The connected Pen's id is kept in the directory's .codepen.config.json (see
// penconfig), which is never pushed. It is not safe for concurrent use.
type Mirror struct {
	cfg        Config
	id         string
	head       string // version id of the tracked head
	title      string
	editor     string // "V2" for 2.0 Pens; Classic Pens can't be written
	urls       codepen.URLs
	remote     map[string]codepen.File
	isTracking bool
	build      codepen.Build // the last settled build
}

// ErrNotConnected is a pull or push that needs a Pen, in a folder without one.
var ErrNotConnected = errors.New("no Pen connected: run `pen push` or `pen pull <PEN>`")

// BuildFailedError is a push whose files were saved, but whose build failed.
type BuildFailedError struct{ Errors int }

func (e *BuildFailedError) Error() string {
	return fmt.Sprintf("the build failed with %s; your files are saved on CodePen", human.Plural(e.Errors, "error"))
}

// Open returns a Mirror for cfg. It makes no requests.
func Open(cfg Config) *Mirror {
	return &Mirror{cfg: cfg, id: cfg.ID, remote: map[string]codepen.File{}}
}

// ID is the connected Pen, "" when there is none.
func (m *Mirror) ID() string { return m.id }

// URLs are the connected Pen's URLs, once known.
func (m *Mirror) URLs() codepen.URLs { return m.urls }

// PushOptions change how Push behaves.
type PushOptions struct {
	Force  bool // skip the version check and overwrite remote edits
	DryRun bool // report what would change, change nothing
}

// Push makes the Pen match the directory: changed and new files go up, files
// missing locally are deleted remotely. Without a connected Pen it creates one.
func (m *Mirror) Push(ctx context.Context, opts PushOptions) error {
	if m.id == "" {
		return m.create(ctx, opts.DryRun)
	}
	if !m.isTracking {
		if err := m.fetch(ctx); err != nil {
			return err
		}
	}
	if m.editor != "" && m.editor != "V2" {
		return fmt.Errorf("%s is a Classic Pen: it can be pulled, but only 2.0 Pens can be pushed", m.id)
	}
	local, err := tree.Read(m.cfg.Root)
	if err != nil {
		return err
	}
	if len(local) == 0 {
		return fmt.Errorf("refusing to push: %q is empty, which would delete every file in the Pen; add files, or run `pen pull`", m.cfg.Root)
	}
	changes := diff.Between(local, m.remote)
	if changes.IsEmpty() {
		m.cfg.Reporter.Report(UpToDate{})
		return nil
	}
	m.cfg.Reporter.Report(Planned{Changes: changes, New: m.added(changes)})
	if opts.DryRun {
		m.cfg.Reporter.Report(WouldPush{Title: m.title, Files: changes.Len()})
		return nil
	}
	expected := m.head
	if opts.Force {
		expected = ""
	}
	build, err := m.cfg.API.WriteFiles(ctx, m.id, changes, expected)
	if err == nil {
		err = m.settle(ctx, build)
	}
	if err != nil {
		// The write may have gone through, leaving the tracked head behind, so
		// the next push fetches it again. A conflict is the exception: the head
		// moved by someone else's edit, and pushes keep refusing until a pull.
		if !errors.Is(err, codepen.ErrConflict) {
			m.isTracking = false
		}
		return err
	}
	if err := m.writeBack(ctx, local); err != nil {
		return err
	}
	m.cfg.Reporter.Report(Pushed{Title: m.title, Files: changes.Len()})
	return m.buildResult()
}

// added marks the changed paths the Pen doesn't have yet.
func (m *Mirror) added(changes codepen.ChangeSet) map[string]bool {
	isNew := map[string]bool{}
	for _, w := range changes.Writes {
		if _, exists := m.remote[w.Path]; !exists {
			isNew[w.Path] = true
		}
	}
	for _, u := range changes.Uploads {
		if _, exists := m.remote[u.Path]; !exists {
			isNew[u.Path] = true
		}
	}
	return isNew
}

// buildResult is the error a failed last build makes of an otherwise done push.
func (m *Mirror) buildResult() error {
	if m.build.Status == codepen.BuildFailed {
		return &BuildFailedError{Errors: len(m.build.Errors)}
	}
	return nil
}

// create makes a Pen from the directory; an empty one gets CodePen's starter files.
func (m *Mirror) create(ctx context.Context, isDryRun bool) error {
	local, err := tree.Read(m.cfg.Root)
	if err != nil {
		return err
	}
	changes := diff.Between(local, nil)
	if !changes.IsEmpty() {
		m.cfg.Reporter.Report(Planned{Changes: changes, New: m.added(changes)})
	}
	if isDryRun {
		m.cfg.Reporter.Report(WouldCreate{Files: changes.Len()})
		return nil
	}
	created, err := m.cfg.API.CreatePen(ctx, codepen.NewPen{
		Title:   filepath.Base(m.cfg.Root),
		Changes: changes,
		Starter: len(local) == 0,
	})
	if err != nil {
		return err
	}
	m.id, m.title, m.urls = created.ID, created.Title, created.URLs
	m.cfg.Reporter.Report(Created{Title: created.Title, URLs: created.URLs})
	// Recorded before the build settles, so a failure later can't lose the new Pen.
	if err := m.recordID(); err != nil {
		return err
	}
	if err := m.settle(ctx, created.Build); err != nil {
		return err
	}
	if err := m.writeBack(ctx, local); err != nil {
		return err
	}
	return m.buildResult()
}

// PullOptions change how Pull behaves.
type PullOptions struct {
	Ref   string // Pen id or URL to connect to; "" for the connected Pen
	Force bool   // allow switching away from another connected Pen, and replacing local files when connecting
}

// Pull writes the Pen's files that differ locally. It never deletes local files.
// Connecting to another Pen refuses to replace local files, unless forced.
func (m *Mirror) Pull(ctx context.Context, opts PullOptions) error {
	id, err := m.target(opts.Ref)
	if err != nil {
		return err
	}
	if m.id != "" && m.id != id && !opts.Force {
		return fmt.Errorf("this directory is connected to %s; pass --force to switch to %s", m.id, id)
	}
	before := *m
	m.id = id
	if err := m.fetch(ctx); err != nil {
		*m = before
		return err
	}
	local, err := readOrEmpty(m.cfg.Root)
	if err != nil {
		*m = before
		return err
	}
	state, err := m.cfg.State.Load(m.cfg.Root)
	if err != nil {
		*m = before
		return err
	}
	base := map[string]string{}
	if state.Pen == id {
		base = state.Files
	}
	writes, kept, conflicts := m.merge(local, base)
	if len(conflicts) > 0 && !opts.Force {
		*m = before
		return refusal(id, before.id != id, len(base) > 0, conflicts)
	}
	writes = slices.Sorted(slices.Values(append(writes, conflicts...)))
	if err := m.checkPaths(); err != nil {
		*m = before
		return err
	}
	if err := m.recordID(); err != nil {
		*m = before
		return err
	}
	if before.id != id {
		m.cfg.Reporter.Report(Connected{Title: m.title, ID: m.id, URLs: m.urls})
	}
	for _, p := range kept {
		m.cfg.Reporter.Report(Kept{Path: p})
	}
	if err := m.write(ctx, writes); err != nil {
		return err
	}
	if err := m.saveState(); err != nil {
		return err
	}
	if len(writes) == 0 {
		m.cfg.Reporter.Report(UpToDate{})
	} else {
		m.cfg.Reporter.Report(Pulled{Files: len(writes)})
	}
	return nil
}

// merge sorts the remote files that differ locally, judged against base, the
// files as of the last sync: writes are files missing here or untouched since,
// kept are local edits to files unchanged remotely, and conflicts changed on
// both sides, or can't be told apart without a base.
func (m *Mirror) merge(local tree.Snapshot, base map[string]string) (writes, kept, conflicts []string) {
	for _, p := range m.stale(local) {
		data, exists := local[p]
		synced, hasBase := base[p]
		switch {
		case !exists || (hasBase && penstate.Sum(data) == synced):
			writes = append(writes, p)
		case hasBase && penstate.FileSum(m.remote[p]) == synced:
			kept = append(kept, p)
		default:
			conflicts = append(conflicts, p)
		}
	}
	return writes, kept, conflicts
}

func refusal(id string, isConnecting, hasBase bool, conflicts []string) error {
	paths := strings.Join(conflicts, ", ")
	switch {
	case isConnecting:
		return fmt.Errorf("connecting to %s would replace %s here; pass --force to replace them", id, paths)
	case hasBase:
		return fmt.Errorf("%s changed both here and on CodePen since the last sync; pass --force to take CodePen's version", paths)
	default:
		return fmt.Errorf("%s differ from the Pen, and there's no record of the last sync to tell which side changed; pass --force to take the Pen's version", paths)
	}
}

func (m *Mirror) target(penRef string) (string, error) {
	if penRef != "" {
		return ref.Parse(penRef)
	}
	if m.id != "" {
		return m.id, nil
	}
	return "", ErrNotConnected
}

// fetch loads the Pen's head and tracks it.
func (m *Mirror) fetch(ctx context.Context) error {
	meta, files, err := m.fetchHead(ctx)
	if err != nil {
		return err
	}
	m.title, m.editor, m.urls = meta.Title, meta.Editor, meta.URLs
	head := ""
	if meta.SavedVersion != nil {
		head = meta.SavedVersion.ID
	}
	m.track(head, files)
	return nil
}

// fetchHead requests the Pen's metadata and head files concurrently.
func (m *Mirror) fetchHead(ctx context.Context) (codepen.Pen, []codepen.File, error) {
	var (
		meta  codepen.Pen
		files []codepen.File
	)
	g, gctx := errgroup.WithContext(ctx)
	g.Go(func() (err error) {
		meta, err = m.cfg.API.Pen(gctx, m.id)
		return err
	})
	g.Go(func() (err error) {
		files, err = m.cfg.API.Files(gctx, m.id)
		return err
	})
	return meta, files, g.Wait()
}

// settle waits for a build and tracks the version it produced. That version may
// differ from what was sent (compiler patches), so it is the source of truth.
func (m *Mirror) settle(ctx context.Context, build codepen.Build) error {
	build, err := m.cfg.API.Settle(ctx, build)
	if err != nil {
		return err
	}
	m.build = build
	m.cfg.Reporter.Report(Built{Build: build})
	version, err := m.cfg.API.Version(ctx, build.VersionID)
	if err != nil {
		return err
	}
	m.track(version.ID, version.Files)
	return nil
}

// track remembers head and its files.
func (m *Mirror) track(head string, files []codepen.File) {
	m.head = head
	m.remote = diff.ByPath(files)
	m.isTracking = true
}

// recordID stores the connected Pen's id in the directory, unless it is there.
func (m *Mirror) recordID() error {
	stored, err := penconfig.Read(m.cfg.Root)
	if err == nil && stored == m.id {
		return nil
	}
	if err == nil {
		err = penconfig.Save(m.cfg.Root, m.id)
	}
	if err != nil {
		return fmt.Errorf("saving Pen id %s: %w", m.id, err)
	}
	return nil
}

// writeBack writes every file in stale(local), after checking that the
// directory can hold all of the Pen's files, and remembers the sync.
func (m *Mirror) writeBack(ctx context.Context, local tree.Snapshot) error {
	if err := m.checkPaths(); err != nil {
		return err
	}
	if err := m.write(ctx, m.stale(local)); err != nil {
		return err
	}
	return m.saveState()
}

// checkPaths fails unless the directory can hold every one of the Pen's files.
func (m *Mirror) checkPaths() error {
	paths := slices.DeleteFunc(slices.Collect(maps.Keys(m.remote)), tree.IsIgnored)
	slices.Sort(paths)
	return tree.CheckPaths(paths)
}

// write downloads the given remote files into the directory.
func (m *Mirror) write(ctx context.Context, paths []string) error {
	for _, p := range paths {
		if err := m.download(ctx, m.remote[p]); err != nil {
			return err
		}
		m.cfg.Reporter.Report(Downloaded{Path: p})
	}
	return nil
}

// saveState remembers the tracked head as the last sync.
func (m *Mirror) saveState() error {
	files := map[string]string{}
	for p, file := range m.remote {
		if !tree.IsIgnored(p) {
			files[p] = penstate.FileSum(file)
		}
	}
	return m.cfg.State.Save(m.cfg.Root, penstate.State{Pen: m.id, Version: m.head, Files: files})
}

// stale lists the tracked remote files that local lacks or differs from, in
// order. Ignored paths (.git, .env*, ...) are never listed, so never written.
func (m *Mirror) stale(local tree.Snapshot) []string {
	var paths []string
	for _, p := range slices.Sorted(maps.Keys(m.remote)) {
		if tree.IsIgnored(p) {
			continue
		}
		if data, ok := local[p]; ok && diff.IsSame(data, m.remote[p]) {
			continue
		}
		paths = append(paths, p)
	}
	return paths
}

func (m *Mirror) download(ctx context.Context, file codepen.File) error {
	if file.Content != nil {
		return tree.Write(m.cfg.Root, file.Path, strings.NewReader(*file.Content))
	}
	body, err := m.cfg.API.Download(ctx, file.URL)
	if err != nil {
		return err
	}
	defer body.Close()
	return tree.Write(m.cfg.Root, file.Path, body)
}

// readOrEmpty reads root; a directory that doesn't exist yet is empty.
func readOrEmpty(root string) (tree.Snapshot, error) {
	local, err := tree.Read(root)
	if errors.Is(err, fs.ErrNotExist) {
		return tree.Snapshot{}, nil
	}
	return local, err
}
