//go:build !unix

package cli

import "os/exec"

// detach does nothing: a started process already outlives cdpn here.
func detach(*exec.Cmd) {}
