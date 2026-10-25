// Package config gathers the pen CLI's settings from the process environment and the
// env files in the working directory.
package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/nocksock/codepen-cli/internal/envfile"
	"github.com/nocksock/codepen-cli/internal/penconfig"
	"github.com/nocksock/codepen-cli/internal/ref"
)

const (
	// SecretsFile holds the API key. The CLI only ever reads it: it may be a
	// 1Password Environments mount.
	SecretsFile = ".env"
	// StateFile holds local settings such as CODEPEN_DIR and wins over SecretsFile.
	StateFile = ".env.local"
)

// Config is the CLI's settings for one working directory.
type Config struct {
	KeyRef       string        // CODEPEN_API_KEY: a key or an op:// reference
	Dir          string        // CODEPEN_DIR as an absolute path (default: the working directory)
	PenID        string        // CODEPEN_ID, else the id in Dir/.codepen/pen.config.json; "" when none
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
	if cfg.PenID == "" {
		if cfg.PenID, err = storedPenID(cfg.Dir); err != nil {
			return Config{}, err
		}
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

// storedPenID reads the Pen id kept in dir's .codepen/pen.config.json; a
// missing file means no Pen is connected.
func storedPenID(dir string) (string, error) {
	file := filepath.Join(dir, filepath.FromSlash(penconfig.Path))
	data, err := os.ReadFile(file)
	if errors.Is(err, fs.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("reading %q: %w", file, err)
	}
	return penconfig.ID(data), nil
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
