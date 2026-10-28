package cli

import (
	"errors"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/nocksock/codepen-cli/internal/pen"
)

// errLinkedFork stands in for CodePen's own fork until its API is documented.
var errLinkedFork = errors.New("forks linked to their source aren't available yet; pass --detach for a copy without the link")

func newForkCmd(a *app) *cobra.Command {
	var into string
	var isDetached, force bool
	cmd := &cobra.Command{
		Use:   "fork [SOURCE]",
		Short: "copy a Pen and connect a folder to the copy",
		Long: `Copy a Pen into a new one and connect a folder to the copy. The original Pen
is left as it is.

SOURCE is . (the default) for this folder, unpushed edits included; remote for
the connected Pen as it is on CodePen; or the id or URL of another of your Pens.
Copying a Pen into this folder keeps your local edits, as pen pull does.

The copy goes to this folder, or with --into to a new or empty one, while this
folder stays connected to the original.

For now every fork needs --detach: the copy is a new Pen with the same files,
without CodePen's link to the original. Forks count toward CodePen's daily
limit on new Pens.`,
		Example: `  pen fork --detach                      # copy what you see, switch to the copy
  pen fork remote --detach               # copy the Pen as it is on CodePen
  pen fork <PEN> --detach --into ../copy # copy another Pen into a new folder`,
		Args: usageArgs(cobra.MaximumNArgs(1)),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !isDetached {
				return errLinkedFork
			}
			m, _, err := a.mirror(cmd.Context())
			if err != nil {
				return err
			}
			opts := pen.ForkOptions{Force: force}
			if len(args) == 1 {
				opts.Source = args[0]
			}
			if into != "" {
				opts.Into = filepath.Join(a.env.Dir, into)
				if filepath.IsAbs(into) {
					opts.Into = filepath.Clean(into)
				}
			}
			return m.Fork(cmd.Context(), opts)
		},
	}
	cmd.Flags().StringVar(&into, "into", "", "put the copy in this new or empty folder, and keep this one as it is")
	cmd.Flags().BoolVar(&isDetached, "detach", false, "make a new Pen with the same files, without CodePen's link to the original")
	cmd.Flags().BoolVar(&force, "force", false, "take CodePen's version of files changed here too")
	return cmd
}
