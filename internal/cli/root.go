package cli

import (
	"fmt"
	"runtime/debug"

	"github.com/spf13/cobra"

	"github.com/nocksock/codepen-cli/internal/setup"
)

// version is set when building releases:
// -ldflags "-X github.com/nocksock/codepen-cli/internal/cli.version=v1.2.3"
var version string

const usageFormat = `cdpn — mirror a local directory to a CodePen 2.0 Pen

  cdpn init [PEN]         set up .env, .envrc and .gitignore here; PEN pulls it
  cdpn push [--force]     push local files (creates a Pen if CODEPEN_ID is unset)
  cdpn pull [PEN]         download the Pen's files; PEN (id or URL) connects to it
  cdpn sync [--force]     push, then watch and push on every change
  cdpn info               show the Pen's details and local sync status
  cdpn open [TARGET]      open editor (default), preview or live in the browser
  cdpn completion SHELL   print a completion script (bash, zsh, fish, powershell)

env (.env, .env.local):
  CODEPEN_API_KEY   key or 1Password ref, e.g. "op://Private/CodePen/API Key"
  CODEPEN_ID        Pen id, written automatically on first push
  CODEPEN_DIR       directory to mirror (default: .)

--key      init: API key or 1Password ref for a new .env (default: %q)
--force    init/pull: switch to PEN although another Pen is connected
           push/sync: skip the version check and overwrite remote edits
--version  print the version`

func usageText() string {
	return fmt.Sprintf(usageFormat, setup.DefaultKey)
}

func buildVersion() string {
	if version != "" {
		return version
	}
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" && info.Main.Version != "(devel)" {
		return info.Main.Version
	}
	return "dev"
}

func newRoot(a *app) *cobra.Command {
	root := &cobra.Command{
		Use:           "cdpn",
		Short:         "mirror a local directory to a CodePen 2.0 Pen",
		Version:       buildVersion(),
		SilenceUsage:  true,
		SilenceErrors: true,
		Args: func(_ *cobra.Command, args []string) error {
			if len(args) > 0 {
				return usageError{err: fmt.Errorf("unknown command %q", args[0])}
			}
			return nil
		},
		RunE: func(*cobra.Command, []string) error { return usageError{} },
	}
	root.SetFlagErrorFunc(func(_ *cobra.Command, err error) error { return usageError{err: err} })
	root.AddCommand(newInitCmd(a), newPushCmd(a), newPullCmd(a), newInfoCmd(a), newOpenCmd(a))
	help := root.HelpFunc()
	root.SetHelpFunc(func(c *cobra.Command, args []string) {
		if c != root {
			help(c, args)
			return
		}
		fmt.Fprintln(c.OutOrStdout(), usageText())
	})
	return root
}
