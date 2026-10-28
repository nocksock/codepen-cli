package cli

import (
	"github.com/spf13/cobra"

	"github.com/nocksock/codepen-cli/internal/pen"
)

func newPushCmd(a *app) *cobra.Command {
	var force, isDryRun bool
	cmd := &cobra.Command{
		Use:   "push",
		Short: "push local files (creates a Pen if none is connected)",
		Args:  noArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			m, _, err := a.mirror(cmd.Context())
			if err != nil {
				return err
			}
			return m.Push(cmd.Context(), pen.PushOptions{Force: force, DryRun: isDryRun})
		},
	}
	cmd.Flags().BoolVar(&force, "force", false, "skip the version check and overwrite remote edits")
	cmd.Flags().BoolVar(&isDryRun, "dry-run", false, "list what would change, change nothing")
	return cmd
}
