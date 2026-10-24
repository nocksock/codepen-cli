// Package setup prepares a directory for cdpn (`cdpn init`).
package setup

import (
	"context"
	"path/filepath"

	"github.com/nocksock/codepen-cli/internal/config"
	"github.com/nocksock/codepen-cli/internal/dotfile"
)

// DefaultKey is the API key a new .env points at: a 1Password reference.
const DefaultKey = "op://Private/CodePen/API Key"

// Result is what Scaffold did to one file.
type Result struct {
	File   string
	Status dotfile.Status
}

// Runner runs a program in the directory being set up.
type Runner interface {
	Output(ctx context.Context, name string, args ...string) ([]byte, error)
}

type step struct {
	file   string
	ensure func(path string) (dotfile.Status, error)
}

// Scaffold makes sure dir has a .env with an API key, an .envrc that loads
// both env files (state last, so it wins in direnv) and a .gitignore that keeps
// them out of git. Nothing that exists is overwritten.
func Scaffold(dir, key string) ([]Result, error) {
	steps := []step{
		{file: config.SecretsFile, ensure: func(p string) (dotfile.Status, error) {
			return dotfile.Ensure(p, "CODEPEN_API_KEY=\""+key+"\"\n", 0o600)
		}},
		{file: ".envrc", ensure: func(p string) (dotfile.Status, error) {
			return dotfile.EnsureLines(p, []string{"dotenv_if_exists " + config.SecretsFile, "dotenv_if_exists " + config.StateFile})
		}},
		{file: ".gitignore", ensure: func(p string) (dotfile.Status, error) {
			return dotfile.EnsureLines(p, []string{config.SecretsFile, config.StateFile})
		}},
	}
	results := make([]Result, 0, len(steps))
	for _, s := range steps {
		status, err := s.ensure(filepath.Join(dir, s.file))
		if err != nil {
			return results, err
		}
		results = append(results, Result{File: s.file, Status: status})
	}
	return results, nil
}

// AllowDirenv runs `direnv allow .` and reports whether direnv accepted the
// .envrc. A missing direnv just reports false.
func AllowDirenv(ctx context.Context, run Runner) bool {
	_, err := run.Output(ctx, "direnv", "allow", ".")
	return err == nil
}
