// Package config gathers cdpn's settings from the process environment and the
// env files in the working directory.
package config

import (
	"fmt"
	"path/filepath"
	"strconv"
	"time"

	"github.com/nocksock/codepen-cli/internal/envfile"
	"github.com/nocksock/codepen-cli/internal/ref"
)

const (
	// SecretsFile holds the API key. cdpn only ever reads it: it may be a
	// 1Password Environments mount.
	SecretsFile = ".env"
	// StateFile holds local state such as CODEPEN_ID and wins over SecretsFile.
	StateFile = ".env.local"
)

// Config is cdpn's settings for one working directory.
type Config struct {
	KeyRef       string        // CODEPEN_API_KEY: a key or an op:// reference
	Dir          string        // CODEPEN_DIR as an absolute path (default: the working directory)
	PenID        string        // CODEPEN_ID, parsed; "" when no Pen is connected
	APIURL       string        // CODEPEN_API_URL; "" for the production API
	PollInterval time.Duration // CODEPEN_POLL_MS; 0 for the default
}

// Load reads the settings for the working directory dir (absolute). Values come
// from getenv first, then StateFile, then SecretsFile; empty values count as unset.
func Load(dir string, getenv func(string) (string, bool)) (Config, error) {
	lookup, err := sources(dir, getenv)
	if err != nil {
		return Config{}, err
	}
	cfg := Config{
		KeyRef: lookup("CODEPEN_API_KEY"),
		Dir:    resolve(dir, lookup("CODEPEN_DIR")),
		APIURL: lookup("CODEPEN_API_URL"),
	}
	if cfg.PenID, err = penID(lookup("CODEPEN_ID")); err != nil {
		return Config{}, err
	}
	if cfg.PollInterval, err = millis(lookup("CODEPEN_POLL_MS")); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

// Key returns the API key reference, or an error saying where to set it.
func (c Config) Key() (string, error) {
	if c.KeyRef == "" {
		return "", fmt.Errorf("CODEPEN_API_KEY is not set (check %s / %s)", StateFile, SecretsFile)
	}
	return c.KeyRef, nil
}

func sources(dir string, getenv func(string) (string, bool)) (func(string) string, error) {
	state, err := envfile.Read(filepath.Join(dir, StateFile))
	if err != nil {
		return nil, err
	}
	secrets, err := envfile.Read(filepath.Join(dir, SecretsFile))
	if err != nil {
		return nil, err
	}
	return func(name string) string {
		if value, ok := getenv(name); ok && value != "" {
			return value
		}
		if value := state[name]; value != "" {
			return value
		}
		return secrets[name]
	}, nil
}

func resolve(dir, path string) string {
	if path == "" {
		return dir
	}
	if filepath.IsAbs(path) {
		return filepath.Clean(path)
	}
	return filepath.Join(dir, path)
}

func penID(value string) (string, error) {
	if value == "" {
		return "", nil
	}
	id, err := ref.Parse(value)
	if err != nil {
		return "", fmt.Errorf("CODEPEN_ID: %w", err)
	}
	return id, nil
}

func millis(value string) (time.Duration, error) {
	if value == "" {
		return 0, nil
	}
	n, err := strconv.Atoi(value)
	if err != nil || n <= 0 {
		return 0, fmt.Errorf("CODEPEN_POLL_MS: %q is not a positive number", value)
	}
	return time.Duration(n) * time.Millisecond, nil
}
