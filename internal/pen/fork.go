package pen

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/nocksock/codepen-cli/internal/codepen"
	"github.com/nocksock/codepen-cli/internal/diff"
	"github.com/nocksock/codepen-cli/internal/ref"
	"github.com/nocksock/codepen-cli/internal/tree"
)

// ErrNothingToFork is a fork of the connected Pen, in a folder without one.
var ErrNothingToFork = errors.New("nothing to fork: `pen push` creates a Pen")

// ForkOptions change how Fork behaves.
type ForkOptions struct {
	Source string // "." (or "") for this folder, "remote" for the connected Pen, or a Pen id or URL
	Into   string // absolute folder for the copy; "" for this one
	Force  bool   // replace local files changed both here and on CodePen
}

// Fork makes a new Pen from a source, unlinked from it, and connects a folder
// to the copy: this one, or opts.Into, which must be new or empty.
//
// The source "." is this folder, unpushed edits included. Any other source is
// a Pen as it is on CodePen; pulled into this folder, it keeps local edits as
// a pull would, and a file changed on both sides stops the fork before a Pen
// is made.
func (m *Mirror) Fork(ctx context.Context, opts ForkOptions) error {
	isLocal := opts.Source == "" || opts.Source == "."
	sourceID := m.id
	if !isLocal && opts.Source != "remote" {
		id, err := ref.Parse(opts.Source)
		if err != nil {
			return err
		}
		sourceID = id
	}
	if sourceID == "" {
		return ErrNothingToFork
	}
	if opts.Into != "" {
		if err := checkEmpty(opts.Into); err != nil {
			return err
		}
	}
	source := Open(Config{API: m.cfg.API, Root: m.cfg.Root, ID: sourceID, Reporter: m.cfg.Reporter, State: m.cfg.State})
	if err := source.fetch(ctx); err != nil {
		return err
	}
	var local tree.Snapshot
	var changes codepen.ChangeSet
	var err error
	if isLocal {
		if local, err = tree.Read(m.cfg.Root); err != nil {
			return err
		}
		if changes = diff.Between(local, nil); changes.IsEmpty() {
			return fmt.Errorf("refusing to fork: %q has no files to copy", m.cfg.Root)
		}
	} else {
		if opts.Into == "" && !opts.Force {
			if err := source.checkMerge(); err != nil {
				return err
			}
		}
		if changes, err = source.copyFiles(ctx); err != nil {
			return err
		}
	}
	created, err := m.cfg.API.CreatePen(ctx, codepen.NewPen{Title: source.title, Changes: changes})
	if err != nil {
		return err
	}
	m.cfg.Reporter.Report(Forked{From: source.title, Title: created.Title, ID: created.ID})
	if isLocal && opts.Into == "" {
		m.id, m.title, m.editor, m.urls = created.ID, created.Title, "V2", created.URLs
		if err := m.recordID(); err != nil {
			return err
		}
		m.cfg.Reporter.Report(Connected{Title: m.title, ID: m.id, URLs: m.urls})
		if err := m.settle(ctx, created.Build); err != nil {
			return err
		}
		if err := m.writeBack(ctx, local); err != nil {
			return err
		}
		return m.buildResult()
	}
	target := m
	if opts.Into != "" {
		target = Open(Config{API: m.cfg.API, Root: opts.Into, Reporter: m.cfg.Reporter, State: m.cfg.State})
	}
	// The copy's own build settles first, so the pull gets its patched files.
	build, err := m.cfg.API.Settle(ctx, created.Build)
	if err != nil {
		return err
	}
	m.cfg.Reporter.Report(Built{Build: build})
	err = target.Pull(ctx, PullOptions{Ref: created.ID, Force: opts.Force, isSwitching: true, baseFrom: sourceID})
	if err != nil {
		return err
	}
	target.build = build
	return target.buildResult()
}

// checkMerge fails when pulling the tracked files into the folder would
// replace a file changed on both sides.
func (m *Mirror) checkMerge() error {
	local, err := readOrEmpty(m.cfg.Root)
	if err != nil {
		return err
	}
	state, err := m.cfg.State.Load(m.cfg.Root)
	if err != nil {
		return err
	}
	base := map[string]string{}
	if state.Pen == m.id {
		base = state.Files
	}
	if _, _, conflicts := merge(local, m.remote, base); len(conflicts) > 0 {
		return fmt.Errorf("forking would replace %s here, which changed both here and on CodePen; pass --force to take CodePen's version", strings.Join(conflicts, ", "))
	}
	return nil
}

// copyFiles turns the tracked files into a change set that recreates them,
// downloading binary ones.
func (m *Mirror) copyFiles(ctx context.Context) (codepen.ChangeSet, error) {
	snapshot := tree.Snapshot{}
	for p, file := range m.remote {
		if file.Content != nil {
			snapshot[p] = []byte(*file.Content)
			continue
		}
		data, err := m.fetchBinary(ctx, file)
		if err != nil {
			return codepen.ChangeSet{}, err
		}
		snapshot[p] = data
	}
	return diff.Between(snapshot, nil), nil
}

func (m *Mirror) fetchBinary(ctx context.Context, file codepen.File) ([]byte, error) {
	body, err := m.cfg.API.Download(ctx, file.URL)
	if err != nil {
		return nil, err
	}
	defer body.Close()
	return io.ReadAll(body)
}

// checkEmpty fails unless dir is missing or empty.
func checkEmpty(dir string) error {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if len(entries) > 0 {
		return fmt.Errorf("%s isn't empty: fork --into needs a new or empty folder", dir)
	}
	return nil
}
