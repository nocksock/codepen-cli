package cli

import (
	"github.com/spf13/cobra"

	"github.com/nocksock/codepen-cli/internal/pen"
)

func newPullCmd(a *app) *cobra.Command {
	var force bool
	cmd := &cobra.Command{
		Use:   "pull [PEN]",
		Short: "download the Pen's files; PEN (id or URL) connects to it",
		Args:  usageArgs(cobra.MaximumNArgs(1)),
		RunE: func(cmd *cobra.Command, args []string) error {
			m, _, err := a.mirror(cmd.Context())
			if err != nil {
				return err
			}
			opts := pen.PullOptions{Force: force}
			if len(args) == 1 {
				opts.Ref = args[0]
			}
			return m.Pull(cmd.Context(), opts)
		},
	}
	cmd.Flags().BoolVar(&force, "force", false, "switch to PEN although another Pen is connected")
	return cmd
}
