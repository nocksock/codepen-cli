package cli

import (
	"context"
	"fmt"

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
		Args:  usageArgs(cobra.NoArgs),
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
// run on this goroutine, so they never overlap.
func (a *app) syncPen(ctx context.Context, force bool) error {
	m, cfg, err := a.mirror(ctx)
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
			fmt.Fprintf(a.env.Stderr, "✗ %s\n", explain(err))
		}
	}
	push()
	if urls := m.URLs(); urls.Editor != "" {
		a.printf("editor  %s\npreview %s\n", urls.Editor, urls.Preview)
	}
	a.printf("watching %s\n", cfg.Dir)
	for range changes {
		push()
	}
	return nil
}
