// Package secret resolves API keys that are 1Password references.
package secret

import (
	"context"
	"fmt"
	"strings"
)

// Runner runs a program and returns its standard output.
type Runner interface {
	Output(ctx context.Context, name string, args ...string) ([]byte, error)
}

// Resolve returns the secret value behind value: op:// references are read
// with `op read`, anything else (a plain key, or one already resolved by
// `op run`) is returned as is.
func Resolve(ctx context.Context, value string, run Runner) (string, error) {
	if !strings.HasPrefix(value, "op://") {
		return value, nil
	}
	out, err := run.Output(ctx, "op", "read", value)
	if err != nil {
		return "", fmt.Errorf("reading %q from 1Password: %w", value, err)
	}
	return strings.TrimSpace(string(out)), nil
}
