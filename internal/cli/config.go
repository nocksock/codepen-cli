package cli

import (
	"fmt"
	"slices"
	"strings"

	"github.com/spf13/cobra"

	"github.com/nocksock/codepen-cli/internal/filter"
	"github.com/nocksock/codepen-cli/internal/human"
	"github.com/nocksock/codepen-cli/internal/penconfig"
)

// listKeys are the settings pen config can change.
var listKeys = []string{penconfig.Include, penconfig.Exclude}

func newConfigCmd(a *app) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "show or change which files sync",
		Long: `Show or change the settings in .codepen.config.json: the connected Pen's id,
and the include and exclude lists of patterns that choose which files sync.

With include set, only matching files sync; exclude leaves matching files out.
Patterns work like .gitignore: * within a name, ** across folders, a trailing
/ for folders, and a leading / to match from the top only.`,
		Example: `  pen config
  pen config set include src/ index.html
  pen config add exclude '*.map'
  pen config remove exclude '*.map'
  pen config set exclude               # clear the list`,
		Args: noArgs,
		RunE: func(*cobra.Command, []string) error { return a.showConfig() },
	}
	cmd.AddCommand(
		&cobra.Command{
			Use:   "get KEY",
			Short: "print a setting, one pattern per line",
			Args:  usageArgs(cobra.ExactArgs(1)),
			RunE: func(_ *cobra.Command, args []string) error {
				return a.getConfig(args[0])
			},
		},
		&cobra.Command{
			Use:   "set KEY [PATTERN...]",
			Short: "replace a list; no patterns clears it",
			Args:  usageArgs(cobra.MinimumNArgs(1)),
			RunE: func(_ *cobra.Command, args []string) error {
				return a.changeList(args[0], func([]string) ([]string, error) { return args[1:], nil })
			},
		},
		&cobra.Command{
			Use:   "add KEY PATTERN...",
			Short: "add patterns to a list",
			Args:  usageArgs(cobra.MinimumNArgs(2)),
			RunE: func(_ *cobra.Command, args []string) error {
				return a.changeList(args[0], func(list []string) ([]string, error) {
					for _, pattern := range args[1:] {
						if !slices.Contains(list, pattern) {
							list = append(list, pattern)
						}
					}
					return list, nil
				})
			},
		},
		&cobra.Command{
			Use:   "remove KEY PATTERN...",
			Short: "remove patterns from a list",
			Args:  usageArgs(cobra.MinimumNArgs(2)),
			RunE: func(_ *cobra.Command, args []string) error {
				return a.changeList(args[0], func(list []string) ([]string, error) {
					for _, pattern := range args[1:] {
						i := slices.Index(list, pattern)
						if i < 0 {
							return nil, fmt.Errorf("%q isn't in %s", pattern, args[0])
						}
						list = slices.Delete(list, i, i+1)
					}
					return list, nil
				})
			},
		},
	)
	return cmd
}

func (a *app) showConfig() error {
	cfg, err := a.config()
	if err != nil {
		return err
	}
	id, err := penconfig.Read(cfg.Dir)
	if err != nil {
		return err
	}
	a.printf("%s\n", human.Table([]human.Row{
		{Label: "id", Value: orNone(id)},
		{Label: penconfig.Include, Value: orNone(strings.Join(cfg.Include, ", "))},
		{Label: penconfig.Exclude, Value: orNone(strings.Join(cfg.Exclude, ", "))},
	}))
	return nil
}

func (a *app) getConfig(key string) error {
	cfg, err := a.config()
	if err != nil {
		return err
	}
	if key == "id" {
		id, err := penconfig.Read(cfg.Dir)
		if err == nil && id != "" {
			a.printf("%s\n", id)
		}
		return err
	}
	if err := checkKey(key); err != nil {
		return err
	}
	list, err := penconfig.List(cfg.Dir, key)
	for _, pattern := range list {
		a.printf("%s\n", human.Clean(pattern))
	}
	return err
}

// changeList replaces the list under key with what change makes of it, after
// checking every pattern, and prints the result.
func (a *app) changeList(key string, change func([]string) ([]string, error)) error {
	if err := checkKey(key); err != nil {
		return err
	}
	cfg, err := a.config()
	if err != nil {
		return err
	}
	list, err := penconfig.List(cfg.Dir, key)
	if err != nil {
		return err
	}
	if list, err = change(list); err != nil {
		return err
	}
	for _, pattern := range list {
		if err := filter.Check(pattern); err != nil {
			return err
		}
	}
	if err := penconfig.SetList(cfg.Dir, key, list); err != nil {
		return err
	}
	fmt.Fprintf(a.progress(), "%s\n", human.Table([]human.Row{{Label: key, Value: orNone(strings.Join(list, ", "))}}))
	return nil
}

func checkKey(key string) error {
	if !slices.Contains(listKeys, key) {
		return usageError{err: fmt.Errorf("unknown setting %q: use %s", key, strings.Join(listKeys, " or "))}
	}
	return nil
}

func orNone(value string) string {
	if value == "" {
		return "(none)"
	}
	return value
}
