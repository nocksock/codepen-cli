package cli

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/nocksock/codepen-cli/internal/pen"
	"github.com/nocksock/codepen-cli/internal/tree"
	"github.com/nocksock/codepen-cli/internal/watch"
)

func newSyncCmd(a *app) *cobra.Command {
	var force bool
	cmd := &cobra.Command{
		Use:   "sync",
		Short: "push, then watch and push on every change",
		Args:  noArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return a.syncPen(cmd.Context(), force)
		},
	}
	cmd.Flags().BoolVar(&force, "force", false, "skip the version check and overwrite remote edits")
	return cmd
}

// syncPen pushes, then pushes again whenever the directory changes, until ctx
// is done. Push errors are reported and watching goes on. Watching starts
// before the first push so edits made during it are caught, and all pushes
// run on this goroutine, so they never overlap. Once watching, each line of
// output starts with the time.
func (a *app) syncPen(ctx context.Context, force bool) error {
	isWatching := false
	stdout := &stamper{w: a.progress(), isOn: &isWatching}
	stderr := &stamper{w: a.env.Stderr, isOn: &isWatching}
	m, cfg, err := a.mirrorTo(ctx, printer{w: stdout, err: stderr})
	if err != nil {
		return err
	}
	changes, err := watch.Changes(ctx, cfg.Dir, watch.Options{Skip: tree.IsIgnored})
	if err != nil {
		return err
	}
	push := func() {
		err := m.Push(ctx, pen.PushOptions{Force: force})
		if err != nil && ctx.Err() == nil {
			fmt.Fprintf(stderr, "✗ %s\n", explainSync(err))
		}
	}
	push()
	if ctx.Err() == nil { // false when interrupted during the first push
		fmt.Fprintf(a.progress(), "watching %s\n", a.shown(cfg.Dir))
		isWatching = true
	}
	for range changes { // closed once ctx is done
		push()
	}
	return nil
}

// shown is dir as the user would type it: relative to the working directory
// when inside it.
func (a *app) shown(dir string) string {
	rel, err := filepath.Rel(a.env.Dir, dir)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return dir
	}
	return rel
}

// stamper starts each line written to w with the time, once *isOn.
type stamper struct {
	w         io.Writer
	isOn      *bool
	isMidLine bool
}

func (s *stamper) Write(p []byte) (int, error) {
	if !*s.isOn {
		return s.w.Write(p)
	}
	var b bytes.Buffer
	for _, line := range bytes.SplitAfter(p, []byte("\n")) {
		if len(line) == 0 {
			continue
		}
		if !s.isMidLine {
			b.WriteString(time.Now().Format("15:04:05 "))
		}
		b.Write(line)
		s.isMidLine = line[len(line)-1] != '\n'
	}
	if _, err := s.w.Write(b.Bytes()); err != nil {
		return 0, err
	}
	return len(p), nil
}
