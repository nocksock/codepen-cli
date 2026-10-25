package cli

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/nocksock/codepen-cli/internal/browser"
	"github.com/nocksock/codepen-cli/internal/codepen"
)

// targetNames lists the targets in the order they are offered.
var targetNames = []string{"editor", "preview", "live"}

// targets pick a URL of a Pen; "" means the Pen has none.
var targets = map[string]func(codepen.Pen) string{
	"editor":  func(p codepen.Pen) string { return p.URLs.Editor },
	"preview": func(p codepen.Pen) string { return p.URLs.Preview },
	"live": func(p codepen.Pen) string {
		if p.Deployment == nil {
			return ""
		}
		return p.Deployment.URL
	},
}

func newOpenCmd(a *app) *cobra.Command {
	return &cobra.Command{
		Use:       "open [" + strings.Join(targetNames, "|") + "]",
		Short:     "open editor (default), preview or live in the browser",
		ValidArgs: targetNames,
		Args:      usageArgs(cobra.MatchAll(cobra.MaximumNArgs(1), knownTarget)),
		RunE: func(cmd *cobra.Command, args []string) error {
			target := "editor"
			if len(args) == 1 {
				target = args[0]
			}
			return a.openPen(cmd.Context(), target)
		},
	}
}

func knownTarget(_ *cobra.Command, args []string) error {
	if len(args) == 1 && targets[args[0]] == nil {
		return fmt.Errorf("unknown target %q: use %s", args[0], strings.Join(targetNames, ", "))
	}
	return nil
}

// openPen prints the target URL and opens it in the browser without waiting.
func (a *app) openPen(ctx context.Context, target string) error {
	api, cfg, err := a.connected(ctx)
	if err != nil {
		return err
	}
	p, err := api.Pen(ctx, cfg.PenID)
	if err != nil {
		return err
	}
	href := targets[target](p)
	if href == "" {
		return fmt.Errorf("the Pen has no %s URL (not deployed?)", target)
	}
	name, args, err := browser.Command(a.env.GOOS, href)
	if err != nil {
		return err
	}
	a.printf("%s\n", href)
	if err := a.env.Runner.Start(name, args...); err != nil {
		a.printf("could not launch a browser; open %s\n", href)
	}
	return nil
}
