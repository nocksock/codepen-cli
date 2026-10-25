package cli

import (
	"errors"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/nocksock/codepen-cli/internal/codepen"
	"github.com/nocksock/codepen-cli/internal/human"
)

// usageError is a mistake in how the pen CLI was called: exit 2 with the usage text.
type usageError struct{ err error }

func (e usageError) Error() string {
	if e.err == nil {
		return "usage"
	}
	return e.err.Error()
}

func (e usageError) Unwrap() error { return e.err }

var errNotConnected = errors.New("no Pen connected: run `pen push` or `pen pull <PEN>`")

// noArgs refuses any argument, with the same wording for every command.
func noArgs(cmd *cobra.Command, args []string) error {
	if len(args) > 0 {
		return usageError{err: fmt.Errorf("%s takes no arguments", cmd.CommandPath())}
	}
	return nil
}

// usageArgs turns the errors of an argument check into usage errors.
func usageArgs(check cobra.PositionalArgs) cobra.PositionalArgs {
	return func(cmd *cobra.Command, args []string) error {
		if err := check(cmd, args); err != nil {
			return usageError{err: err}
		}
		return nil
	}
}

// report prints err the way the CLI always has and returns the exit code.
func report(env Env, err error) int {
	var usage usageError
	switch {
	case err == nil:
		return 0
	case errors.As(err, &usage):
		if usage.err != nil {
			fmt.Fprintf(env.Stderr, "✗ %v\n\n", usage.err)
		}
		fmt.Fprintln(env.Stderr, usageText())
		return 2
	default:
		fmt.Fprintf(env.Stderr, "✗ %s\n", explain(err))
		return 1
	}
}

// explain turns the API errors people run into into advice. Other errors may
// quote the API or a subprocess, so each of their lines is cleaned.
func explain(err error) string {
	switch {
	case errors.Is(err, codepen.ErrConflict):
		return "the Pen changed remotely (editor?). run `pen pull` first, or push with --force"
	case errors.Is(err, codepen.ErrNotFound):
		return "Pen not found. The API only reaches your own Pens (fork someone else's on codepen.io first)"
	default:
		lines := strings.Split(err.Error(), "\n")
		for i, line := range lines {
			lines[i] = human.Clean(line)
		}
		return strings.Join(lines, "\n")
	}
}
