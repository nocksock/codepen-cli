package cli

import (
	"os"
	"testing"
)

func TestExecRunner_StartMissingProgram(t *testing.T) {
	t.Parallel()
	err := execRunner{dir: t.TempDir()}.Start("pen-no-such-program")
	if err == nil {
		t.Error("expected an error for a missing program")
	}
}

func TestExecRunner_StartRunsProgram(t *testing.T) {
	t.Parallel()
	exe, err := os.Executable() // always present, and exits quickly with -test.list
	if err != nil {
		t.Fatal(err)
	}
	if err := (execRunner{dir: t.TempDir()}).Start(exe, "-test.list=^$"); err != nil {
		t.Errorf("Start: %v", err)
	}
}
