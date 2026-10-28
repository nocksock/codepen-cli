package cli

import (
	"context"
	"time"

	"github.com/spf13/cobra"

	"github.com/nocksock/codepen-cli/internal/info"
)

func newInfoCmd(a *app) *cobra.Command {
	var isJSON bool
	cmd := &cobra.Command{
		Use:   "info",
		Short: "show the Pen's details and local sync status",
		Args:  noArgs,
		RunE:  func(cmd *cobra.Command, _ []string) error { return a.showInfo(cmd.Context(), isJSON) },
	}
	cmd.Flags().BoolVar(&isJSON, "json", false, "print the details as JSON")
	return cmd
}

func (a *app) showInfo(ctx context.Context, isJSON bool) error {
	api, cfg, err := a.connected(ctx)
	if err != nil {
		return err
	}
	status, err := info.Gather(ctx, api, cfg.PenID, cfg.Dir)
	if err != nil {
		return err
	}
	if !isJSON {
		a.printf("%s\n", info.Render(status, time.Now()))
		return nil
	}
	data, err := info.JSON(status)
	if err != nil {
		return err
	}
	a.printf("%s\n", data)
	return nil
}
