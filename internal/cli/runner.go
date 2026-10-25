package cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
)

// execRunner runs programs with os/exec in dir.
type execRunner struct{ dir string }

func (r execRunner) Output(ctx context.Context, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = r.dir
	out, err := cmd.Output()
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) && len(exitErr.Stderr) > 0 {
		return out, fmt.Errorf("%s: %w: %s", name, err, bytes.TrimSpace(exitErr.Stderr))
	}
	if err != nil {
		return out, fmt.Errorf("%s: %w", name, err)
	}
	return out, nil
}

func (r execRunner) Start(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Dir = r.dir
	detach(cmd)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	return cmd.Process.Release()
}
