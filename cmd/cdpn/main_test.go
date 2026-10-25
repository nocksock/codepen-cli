package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

var binary string

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "cdpn-smoke-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	binary = filepath.Join(dir, "cdpn")
	build := exec.Command("go", "build", "-o", binary, ".")
	build.Stderr = os.Stderr
	if err := build.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "building cdpn:", err)
		os.Exit(1)
	}
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

func TestExitCodes(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		args   []string
		code   int
		output string
	}{
		{name: "help", args: []string{"--help"}, code: 0, output: "cdpn push"},
		{name: "unknown command", args: []string{"bogus"}, code: 2, output: `unknown command "bogus"`},
		{name: "missing key", args: []string{"push"}, code: 1, output: "CODEPEN_API_KEY is not set"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			cmd := exec.Command(binary, tt.args...)
			cmd.Dir = t.TempDir()
			cmd.Env = []string{"HOME=" + cmd.Dir} // nothing from the developer's environment
			out, err := cmd.CombinedOutput()
			code := 0
			var exitErr *exec.ExitError
			switch {
			case errors.As(err, &exitErr):
				code = exitErr.ExitCode()
			case err != nil:
				t.Fatal(err)
			}
			if code != tt.code || !strings.Contains(string(out), tt.output) {
				t.Errorf("cdpn %v: exit %d, want %d; output:\n%s", tt.args, code, tt.code, out)
			}
		})
	}
}
