//go:build unix

package cli

import (
	"os/exec"
	"syscall"
)

// detach puts cmd in its own session so it outlives the CLI and its terminal.
func detach(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
}
