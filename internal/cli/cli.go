// Package cli is cdpn's command line: it parses arguments, wires the packages
// together, and turns their results and errors into output and exit codes.
package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"runtime"
)

// Env is everything cdpn takes from the outside world. Tests build their own.
type Env struct {
	Dir    string                      // working directory, absolute
	Getenv func(string) (string, bool) // process environment
	Stdout io.Writer
	Stderr io.Writer
	Runner Runner // runs op, direnv and the browser
	GOOS   string
}

// Runner runs external programs in Env.Dir, never through a shell.
type Runner interface {
	// Output runs a program to completion and returns its standard output.
	Output(ctx context.Context, name string, args ...string) ([]byte, error)
	// Start launches a program without waiting for it.
	Start(name string, args ...string) error
}

// System is the environment of the running process.
func System() (Env, error) {
	dir, err := os.Getwd()
	if err != nil {
		return Env{}, fmt.Errorf("finding the working directory: %w", err)
	}
	return Env{
		Dir:    dir,
		Getenv: os.LookupEnv,
		Stdout: os.Stdout,
		Stderr: os.Stderr,
		Runner: execRunner{dir: dir},
		GOOS:   runtime.GOOS,
	}, nil
}

// Run executes cdpn with args (without the program name) and returns the exit
// code: 0 success, 1 runtime or API error, 2 usage error.
func Run(ctx context.Context, args []string, env Env) int {
	if args == nil {
		args = []string{} // nil would make Cobra read os.Args
	}
	root := newRoot(&app{env: env})
	root.SetArgs(args)
	return report(env, root.ExecuteContext(ctx))
}
