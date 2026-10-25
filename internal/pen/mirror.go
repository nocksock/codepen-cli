// Package pen mirrors a local directory to a CodePen Pen: push sends local
// changes up, pull brings remote files down.
package pen

import (
	"bytes"
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
	"github.com/nocksock/codepen-cli/internal/penconfig"
	"github.com/nocksock/codepen-cli/internal/ref"
	"github.com/nocksock/codepen-cli/internal/tree"
)

// Config is what a Mirror needs.
type Config struct {
	API      *codepen.Client
	Root     string   // absolute directory to mirror
	ID       string   // connected Pen; "" when none
	Reporter Reporter // required
}

// Mirror is a local directory mirrored to a Pen. Between calls it remembers the
// remote head, so repeated pushes only talk to the API when something changed.
// The connected Pen's id is kept in the directory's .codepen/pen.config.json
// (see penconfig); it is never pushed. It is not safe for concurrent use.
type Mirror struct {
	cfg        Config
	id         string
	head       string // version id of the tracked head
	urls       codepen.URLs
	remote     map[string]codepen.File
	isTracking bool
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
	Force bool // skip the version check and overwrite remote edits
}

// Push makes the Pen match the directory: changed and new files go up, files
// missing locally are deleted remotely. Without a connected Pen it creates one.
func (m *Mirror) Push(ctx context.Context, opts PushOptions) error {
	if m.id == "" {
		return m.create(ctx)
	}
	if !m.isTracking {
		if err := m.fetch(ctx); err != nil {
			return err
		}
	}
	local, err := m.readShared()
	if err != nil {
		return err
	}
	if len(local) == 0 {
		return fmt.Errorf("refusing to push: %q is empty (would delete every file)", m.cfg.Root)
	}
	changes := diff.Between(local, m.remote)
	if changes.IsEmpty() {
		return nil
	}
	m.cfg.Reporter.Report(Planned{Changes: changes})
	expected := m.head
	if opts.Force {
		expected = ""
	}
	build, err := m.cfg.API.WriteFiles(ctx, m.id, changes, expected)
	if err != nil {
		return err
	}
	if err := m.settle(ctx, build); err != nil {
		return err
	}
	return m.writeBack(ctx, local)
}

// create makes a Pen from the directory; an empty one gets CodePen's starter files.
func (m *Mirror) create(ctx context.Context) error {
	raw, err := tree.Read(m.cfg.Root)
	if err != nil {
		return err
	}
	local := penconfig.Local(raw)
	created, err := m.cfg.API.CreatePen(ctx, codepen.NewPen{
		Title:   filepath.Base(m.cfg.Root),
		Changes: diff.Between(local, nil),
	})
	if err != nil {
		return err
	}
	m.id, m.urls = created.ID, created.URLs
	m.cfg.Reporter.Report(Created{URLs: created.URLs})
	// Recorded before the build settles, so a failure later can't lose the new Pen.
	if err := m.recordID(raw); err != nil {
		return err
	}
	if err := m.settle(ctx, created.Build); err != nil {
		return err
	}
	return m.writeBack(ctx, local)
}

// PullOptions change how Pull behaves.
type PullOptions struct {
	Ref   string // Pen id or URL to connect to; "" for the connected Pen
	Force bool   // allow switching away from another connected Pen
}

// Pull writes the Pen's files that differ locally. It never deletes local files.
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
	raw, err := readOrEmpty(m.cfg.Root)
	if err != nil {
		*m = before
		return err
	}
	if err := m.recordID(raw); err != nil {
		*m = before
		return err
	}
	if before.id != id {
		m.cfg.Reporter.Report(Connected{URLs: m.urls})
	}
	return m.writeBack(ctx, penconfig.Local(raw))
}

func (m *Mirror) target(penRef string) (string, error) {
	if penRef != "" {
		return ref.Parse(penRef)
	}
	if m.id != "" {
		return m.id, nil
	}
	return "", errors.New("no Pen to pull: pass a Pen id or URL")
}

// fetch loads the Pen's head and tracks it.
func (m *Mirror) fetch(ctx context.Context) error {
	meta, files, err := m.fetchHead(ctx)
	if err != nil {
		return err
	}
	m.urls = meta.URLs
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
	m.cfg.Reporter.Report(Built{Build: build})
	version, err := m.cfg.API.Version(ctx, build.VersionID)
	if err != nil {
		return err
	}
	m.track(version.ID, version.Files)
	return nil
}

// track remembers head and its files, with the config reduced to its shared part.
func (m *Mirror) track(head string, files []codepen.File) {
	m.head = head
	m.remote = diff.ByPath(penconfig.Remote(files))
	m.isTracking = true
}

// readShared reads the directory as it is compared with the Pen.
func (m *Mirror) readShared() (tree.Snapshot, error) {
	raw, err := tree.Read(m.cfg.Root)
	if err != nil {
		return nil, err
	}
	return penconfig.Local(raw), nil
}

// recordID stores the connected Pen's id in the local config, given the
// directory's current (raw) files. It writes nothing when the id is there.
func (m *Mirror) recordID(raw tree.Snapshot) error {
	current := raw[penconfig.Path]
	if penconfig.ID(current) == m.id {
		return nil
	}
	return m.writeConfig(current)
}

// writeConfig writes data as the local config, with the connected Pen's id.
func (m *Mirror) writeConfig(data []byte) error {
	withID, err := penconfig.WithID(data, m.id)
	if err != nil {
		return fmt.Errorf("saving Pen id %s: %w", m.id, err)
	}
	if err := tree.Write(m.cfg.Root, penconfig.Path, bytes.NewReader(withID)); err != nil {
		return fmt.Errorf("saving Pen id %s: %w", m.id, err)
	}
	return nil
}

// writeBack writes every tracked remote file that local (as returned by
// readShared) lacks or differs from. Ignored paths (.git, .env*, ...) are
// never written, and the config keeps the local id.
func (m *Mirror) writeBack(ctx context.Context, local tree.Snapshot) error {
	for _, p := range slices.Sorted(maps.Keys(m.remote)) {
		if tree.IsIgnored(p) {
			continue
		}
		file := m.remote[p]
		if data, ok := local[p]; ok && diff.IsSame(data, file) {
			continue
		}
		if err := m.download(ctx, file); err != nil {
			return err
		}
		m.cfg.Reporter.Report(Downloaded{Path: p})
	}
	return nil
}

func (m *Mirror) download(ctx context.Context, file codepen.File) error {
	if file.Path == penconfig.Path && file.Content != nil {
		return m.writeConfig([]byte(*file.Content))
	}
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
