package cli

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/nocksock/codepen-cli/internal/codepen"
	"github.com/nocksock/codepen-cli/internal/config"
	"github.com/nocksock/codepen-cli/internal/envfile"
	"github.com/nocksock/codepen-cli/internal/pen"
	"github.com/nocksock/codepen-cli/internal/secret"
)

// app builds what the commands need from Env.
type app struct{ env Env }

func (a *app) config() (config.Config, error) {
	return config.Load(a.env.Dir, a.env.Getenv)
}

// client returns an API client for cfg, resolving an op:// key first.
func (a *app) client(ctx context.Context, cfg config.Config) (*codepen.Client, error) {
	keyRef, err := cfg.Key()
	if err != nil {
		return nil, err
	}
	key, err := secret.Resolve(ctx, keyRef, a.env.Runner)
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

// mirror opens the Mirror for the working directory.
func (a *app) mirror(ctx context.Context) (*pen.Mirror, config.Config, error) {
	cfg, err := a.config()
	if err != nil {
		return nil, cfg, err
	}
	api, err := a.client(ctx, cfg)
	if err != nil {
		return nil, cfg, err
	}
	m := pen.Open(pen.Config{
		API:      api,
		Root:     cfg.Dir,
		ID:       cfg.PenID,
		SaveID:   a.saveID,
		Reporter: printer{w: a.env.Stdout},
	})
	return m, cfg, nil
}

// connected returns a client and config for commands that need a connected Pen.
func (a *app) connected(ctx context.Context) (*codepen.Client, config.Config, error) {
	cfg, err := a.config()
	if err != nil {
		return nil, cfg, err
	}
	api, err := a.client(ctx, cfg)
	if err != nil {
		return nil, cfg, err
	}
	if cfg.PenID == "" {
		return nil, cfg, errNotConnected
	}
	return api, cfg, nil
}

// saveID records the connected Pen in the state file of the working directory.
func (a *app) saveID(id string) error {
	return envfile.Set(filepath.Join(a.env.Dir, config.StateFile), "CODEPEN_ID", id)
}

func (a *app) printf(format string, args ...any) {
	fmt.Fprintf(a.env.Stdout, format, args...)
}
