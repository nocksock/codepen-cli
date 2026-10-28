package cli

import (
	"github.com/spf13/cobra"

	"github.com/nocksock/codepen-cli/internal/pen"
)

func newPullCmd(a *app) *cobra.Command {
	var force bool
	cmd := &cobra.Command{
		Use:   "pull [PEN]",
		Short: "download changes from CodePen; PEN connects the folder to that Pen",
		Long: `Download what changed on CodePen. Files you haven't touched since the last push
or pull are updated and your local edits are kept. A file changed on both sides
stops the pull, which then changes nothing, until you pass --force. Pull never
deletes local files.

With PEN, an id or any CodePen URL that contains one, the folder connects to
that Pen first. The API only reaches your own Pens.`,
		Example: `  pen pull
  pen pull https://codepen.io/editor/you/pen/01J…
  pen pull --force     # take CodePen's version of files changed on both sides`,
		Args: usageArgs(cobra.MaximumNArgs(1)),
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
	cmd.Flags().BoolVar(&force, "force", false, "take CodePen's version of files changed here too, and switch away from a connected Pen")
	return cmd
}
