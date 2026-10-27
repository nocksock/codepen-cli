// Package setup prepares a directory for pen (`pen init`).
package setup

import (
	"path/filepath"

	"github.com/nocksock/codepen-cli/internal/config"
	"github.com/nocksock/codepen-cli/internal/dotfile"
)

// Result is what Scaffold did to one file.
type Result struct {
	File   string
	Status dotfile.Status
}

// Scaffold adds the env files to dir's .gitignore. It leaves .env and .envrc to
// the user. Nothing that exists is overwritten.
func Scaffold(dir string) ([]Result, error) {
	status, err := dotfile.EnsureLines(
		filepath.Join(dir, ".gitignore"),
		[]string{config.SecretsFile, config.StateFile},
	)
	if err != nil {
		return nil, err
	}
	return []Result{{File: ".gitignore", Status: status}}, nil
}
