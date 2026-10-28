package cli

import (
	"context"
	"fmt"

	"github.com/nocksock/codepen-cli/internal/codepen"
	"github.com/nocksock/codepen-cli/internal/config"
	"github.com/nocksock/codepen-cli/internal/pen"
	"github.com/nocksock/codepen-cli/internal/penstate"
)

// app builds what the commands need from Env.
type app struct{ env Env }

func (a *app) config() (config.Config, error) {
	return config.Load(a.env.Dir, a.env.Getenv)
}

// client returns an API client for cfg.
func (a *app) client(_ context.Context, cfg config.Config) (*codepen.Client, error) {
	key, err := cfg.Key()
	if err != nil {
		return nil, err
	}
	opts := []codepen.Option{}
	if cfg.APIURL != "" {
		opts = append(opts, codepen.WithBaseURL(cfg.APIURL))
	}
	if cfg.PollInterval > 0 {
		opts = append(opts, codepen.WithPollInterval(cfg.PollInterval))
	}
	return codepen.New(key, opts...), nil
}

// clientConfig loads the settings and an API client for them.
func (a *app) clientConfig(ctx context.Context) (*codepen.Client, config.Config, error) {
	cfg, err := a.config()
	if err != nil {
		return nil, cfg, err
	}
	api, err := a.client(ctx, cfg)
	if err != nil {
		return nil, cfg, err
	}
	return api, cfg, nil
}

// mirror opens the Mirror for the working directory.
func (a *app) mirror(ctx context.Context) (*pen.Mirror, config.Config, error) {
	return a.mirrorTo(ctx, printer{w: a.env.Stdout, err: a.env.Stderr})
}

// mirrorTo is mirror with its progress going to reporter.
func (a *app) mirrorTo(ctx context.Context, reporter pen.Reporter) (*pen.Mirror, config.Config, error) {
	api, cfg, err := a.clientConfig(ctx)
	if err != nil {
		return nil, cfg, err
	}
	m := pen.Open(pen.Config{
		API:      api,
		Root:     cfg.Dir,
		ID:       cfg.PenID,
		Reporter: reporter,
		State:    penstate.Store{Dir: penstate.DefaultDir(a.env.GOOS, a.env.Getenv)},
	})
	return m, cfg, nil
}

// connected returns a client and config for commands that need a connected Pen.
// It checks for one before requiring the key, so the missing Pen is reported first.
func (a *app) connected(ctx context.Context) (*codepen.Client, config.Config, error) {
	cfg, err := a.config()
	if err != nil {
		return nil, cfg, err
	}
	if cfg.PenID == "" {
		return nil, cfg, errNotConnected
	}
	api, err := a.client(ctx, cfg)
	if err != nil {
		return nil, cfg, err
	}
	return api, cfg, nil
}

func (a *app) printf(format string, args ...any) {
	fmt.Fprintf(a.env.Stdout, format, args...)
}
