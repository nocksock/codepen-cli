//go:build unix

package cli

import (
	"os/exec"
	"testing"
)

func TestDetach_StartsNewSession(t *testing.T) {
	t.Parallel()
	cmd := exec.Command("true")
	detach(cmd)
	if cmd.SysProcAttr == nil || !cmd.SysProcAttr.Setsid {
		t.Errorf("SysProcAttr = %+v, want Setsid", cmd.SysProcAttr)
	}
}
