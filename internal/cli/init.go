package cli

import (
	"context"

	"github.com/spf13/cobra"

	"github.com/nocksock/codepen-cli/internal/pen"
	"github.com/nocksock/codepen-cli/internal/setup"
)

func newInitCmd(a *app) *cobra.Command {
	var force bool
	cmd := &cobra.Command{
		Use:   "init [PEN]",
		Short: "add env files to .gitignore here; PEN pulls it",
		Args:  usageArgs(cobra.MaximumNArgs(1)),
		RunE: func(cmd *cobra.Command, args []string) error {
			penRef := ""
			if len(args) == 1 {
				penRef = args[0]
			}
			return a.initDir(cmd.Context(), initOptions{penRef: penRef, force: force})
		},
	}
	cmd.Flags().BoolVar(&force, "force", false, "switch to PEN although another Pen is connected")
	return cmd
}

type initOptions struct {
	penRef string
	force  bool
}

// initDir adds the env files to .gitignore and, given a Pen, connects to it and
// pulls it.
func (a *app) initDir(ctx context.Context, opts initOptions) error {
	results, err := setup.Scaffold(a.env.Dir)
	for _, r := range results {
		a.printf("  %-7s %s\n", r.Status, r.File)
	}
	if err != nil {
		return err
	}
	if opts.penRef == "" {
		a.printf("next: set CODEPEN_API_KEY (environment or .env), then `pen push`; or `pen pull <PEN>` to connect one\n")
		return nil
	}
	m, _, err := a.mirror(ctx)
	if err != nil {
		return err
	}
	if err := m.Pull(ctx, pen.PullOptions{Ref: opts.penRef, Force: opts.force}); err != nil {
		return err
	}
	a.showURLs(m.URLs())
	return nil
}
