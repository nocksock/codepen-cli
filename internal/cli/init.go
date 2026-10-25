package cli

import (
	"context"

	"github.com/spf13/cobra"

	"github.com/nocksock/codepen-cli/internal/pen"
	"github.com/nocksock/codepen-cli/internal/setup"
)

func newInitCmd(a *app) *cobra.Command {
	var key string
	cmd := &cobra.Command{
		Use:   "init [PEN]",
		Short: "set up .env, .envrc and .gitignore here; PEN pulls it",
		Args:  usageArgs(cobra.MaximumNArgs(1)),
		RunE: func(cmd *cobra.Command, args []string) error {
			penRef := ""
			if len(args) == 1 {
				penRef = args[0]
			}
			return a.initDir(cmd.Context(), key, penRef)
		},
	}
	cmd.Flags().StringVar(&key, "key", setup.DefaultKey, "API key or 1Password ref for a new .env")
	return cmd
}

// initDir scaffolds the working directory, allows direnv and, given a Pen,
// connects to it and pulls it.
func (a *app) initDir(ctx context.Context, key, penRef string) error {
	results, err := setup.Scaffold(a.env.Dir, key)
	for _, r := range results {
		a.printf("  %-7s %s\n", r.Status, r.File)
	}
	if err != nil {
		return err
	}
	if setup.AllowDirenv(ctx, a.env.Runner) {
		a.printf("  direnv allowed\n")
	}
	if penRef == "" {
		a.printf("next: `cdpn push` creates a Pen, `cdpn pull <PEN>` connects an existing one\n")
		return nil
	}
	m, _, err := a.mirror(ctx)
	if err != nil {
		return err
	}
	return m.Pull(ctx, pen.PullOptions{Ref: penRef})
}
