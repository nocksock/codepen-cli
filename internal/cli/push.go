package cli

import (
	"github.com/spf13/cobra"

	"github.com/nocksock/codepen-cli/internal/pen"
)

func newPushCmd(a *app) *cobra.Command {
	var force bool
	cmd := &cobra.Command{
		Use:   "push",
		Short: "push local files (creates a Pen if none is connected)",
		Args:  noArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			m, _, err := a.mirror(cmd.Context())
			if err != nil {
				return err
			}
			if err := m.Push(cmd.Context(), pen.PushOptions{Force: force}); err != nil {
				return err
			}
			a.showURLs(m.URLs())
			return nil
		},
	}
	cmd.Flags().BoolVar(&force, "force", false, "skip the version check and overwrite remote edits")
	return cmd
}
