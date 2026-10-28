package cli

import (
	"fmt"
	"runtime/debug"
	"strings"

	"github.com/spf13/cobra"
)

// version is set when building releases:
// -ldflags "-X github.com/nocksock/codepen-cli/internal/cli.version=v1.2.3"
var version string

const usage = `pen — mirror a local directory to a CodePen 2.0 Pen

  pen init [PEN]         add env files to .gitignore here; PEN pulls it
  pen push [--force]     push local files (creates a Pen if none is connected)
  pen pull [PEN]         download the Pen's files; PEN (id or URL) connects to it
  pen sync [--force]     push, then watch and push on every change
  pen info               show the Pen's details and local sync status
  pen open [TARGET]      open editor (default), preview or live in the browser
  pen completion SHELL   print a completion script (bash, zsh, fish, powershell)

env (.env, .env.local):
  CODEPEN_API_KEY   API key
  CODEPEN_ID        Pen id; overrides the one kept in .codepen.config.json
  CODEPEN_DIR       directory to mirror (default: .)

--force    init/pull: switch to PEN although another Pen is connected, and replace differing local files
           push/sync: skip the version check and overwrite remote edits
--version  print the version`

func usageText() string { return usage }

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
		Use:           "pen",
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
	// Before strictCompletion: Cobra's completion commands capture the writers.
	root.SetOut(a.env.Stdout)
	root.SetErr(a.env.Stderr)
	root.SetFlagErrorFunc(func(_ *cobra.Command, err error) error { return usageError{err: err} })
	root.AddCommand(newInitCmd(a), newPushCmd(a), newPullCmd(a), newSyncCmd(a), newInfoCmd(a), newOpenCmd(a))
	strictCompletion(root)
	strictHelp(root)
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

// strictHelp makes `pen help TOPIC` report an unknown topic as a usage error
// (exit 2), instead of printing the usage and exiting 0.
func strictHelp(root *cobra.Command) {
	root.InitDefaultHelpCmd()
	for _, c := range root.Commands() {
		if c.Name() != "help" {
			continue
		}
		c.RunE = func(_ *cobra.Command, args []string) error {
			topic, _, err := root.Find(args)
			if err != nil || (topic == root && len(args) > 0) {
				return usageError{err: fmt.Errorf("unknown help topic %q", strings.Join(args, " "))}
			}
			return topic.Help()
		}
	}
}

const shells = "bash, zsh, fish or powershell"

// strictCompletion makes Cobra's built-in completion command report bad
// arguments as usage errors (exit 2) like every other command, instead of
// printing help (exit 0) or a plain error (exit 1).
func strictCompletion(root *cobra.Command) {
	root.InitDefaultCompletionCmd()
	for _, c := range root.Commands() {
		if c.Name() != "completion" {
			continue
		}
		c.Args = func(_ *cobra.Command, args []string) error {
			if len(args) > 0 {
				return usageError{err: fmt.Errorf("unknown shell %q: use %s", args[0], shells)}
			}
			return nil
		}
		c.RunE = func(*cobra.Command, []string) error {
			return usageError{err: fmt.Errorf("completion needs a shell: %s", shells)}
		}
		for _, shell := range c.Commands() {
			shell.Args = noArgs
		}
	}
}
