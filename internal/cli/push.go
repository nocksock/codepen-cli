package cli

import (
	"github.com/spf13/cobra"

	"github.com/nocksock/codepen-cli/internal/pen"
)

func newPushCmd(a *app) *cobra.Command {
	var force, isDryRun bool
	cmd := &cobra.Command{
		Use:   "push",
		Short: "upload local changes; creates a Pen if the folder has none",
		Long: `Upload what changed in this folder to the connected Pen: new and changed files
go up, and files deleted here are deleted on CodePen.

Without a connected Pen, push creates one named after the folder. An empty
folder starts from CodePen's starter files.

Push refuses when the Pen changed on CodePen since your last push or pull, so
it can't overwrite edits it hasn't seen. Run pen pull first, or pass --force.`,
		Example: `  pen push
  pen push --dry-run   # list what would change
  pen push --force     # overwrite edits made on CodePen`,
		Args: noArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			m, _, err := a.mirror(cmd.Context())
			if err != nil {
				return err
			}
			return m.Push(cmd.Context(), pen.PushOptions{Force: force, DryRun: isDryRun})
		},
	}
	cmd.Flags().BoolVar(&force, "force", false, "overwrite the Pen even if it changed on CodePen since your last sync")
	cmd.Flags().BoolVar(&isDryRun, "dry-run", false, "list what would change, change nothing")
	a.filterFlags(cmd)
	return cmd
}
