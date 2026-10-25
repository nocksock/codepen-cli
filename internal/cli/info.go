package cli

import (
	"context"
	"time"

	"github.com/spf13/cobra"

	"github.com/nocksock/codepen-cli/internal/info"
)

func newInfoCmd(a *app) *cobra.Command {
	return &cobra.Command{
		Use:   "info",
		Short: "show the Pen's details and local sync status",
		Args:  usageArgs(cobra.NoArgs),
		RunE:  func(cmd *cobra.Command, _ []string) error { return a.showInfo(cmd.Context()) },
	}
}

func (a *app) showInfo(ctx context.Context) error {
	api, cfg, err := a.connected(ctx)
	if err != nil {
		return err
	}
	status, err := info.Gather(ctx, api, cfg.PenID, cfg.Dir)
	if err != nil {
		return err
	}
	a.printf("%s\n", info.Render(status, time.Now()))
	return nil
}
