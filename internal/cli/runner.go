package cli

import (
	"fmt"
	"os/exec"
)

// execRunner runs programs with os/exec in dir.
type execRunner struct{ dir string }

func (r execRunner) Start(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Dir = r.dir
	detach(cmd)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	return cmd.Process.Release()
}
