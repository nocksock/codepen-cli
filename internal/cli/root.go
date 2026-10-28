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

const usage = `pen — edit a CodePen in your own editor, by mirroring a folder to a Pen

usage: pen <command> [flags]

  pen push              upload local changes; creates a Pen if the folder has none
  pen pull [PEN]        download changes from CodePen; PEN connects the folder to that Pen
  pen sync              push, then push again on every change
  pen fork [SOURCE]     copy a Pen and connect a folder to the copy
  pen config            show or change which files sync
  pen info              show the Pen's details, build status and changes waiting to push
  pen open [TARGET]     open the editor (default), preview or live site in the browser
  pen completion SHELL  print a completion script for bash, zsh, fish or powershell

flags:
  -q, --quiet           print errors only
  -h, --help            show help; pen help <command> shows a command's options and examples
  -v, --version         print the version

settings, from the shell, .env.local or .env:
  CODEPEN_API_KEY   your API key (required)
  CODEPEN_DIR       the folder to mirror, if not the current one
  CODEPEN_ID        the Pen to use, as an id or URL; overrides the connected one`

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
	root.PersistentFlags().BoolVarP(&a.isQuiet, "quiet", "q", false, "print errors only")
	root.SetFlagErrorFunc(func(_ *cobra.Command, err error) error { return usageError{err: err} })
	root.AddCommand(newPushCmd(a), newPullCmd(a), newSyncCmd(a), newForkCmd(a), newConfigCmd(a), newInfoCmd(a), newOpenCmd(a))
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
		c.Short = "show a command's options and examples"
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
		c.Short = "print a completion script for " + shells
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
