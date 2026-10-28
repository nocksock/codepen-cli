package cli

import (
	"bytes"
	"errors"
	"fmt"
	"testing"

	"github.com/nocksock/codepen-cli/internal/codepen"
)

const conflictHint = "the Pen changed on CodePen since your last sync: run `pen pull` (it keeps your edits), then push again; or push with --force to overwrite it"

func TestExplain(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		err  error
		want string
	}{
		{name: "conflict", err: &codepen.APIError{Status: 409, Code: "CONFLICT"}, want: conflictHint},
		{name: "not found", err: &codepen.APIError{Status: 404, Code: "NOT_FOUND"}, want: "Pen not found. The API only reaches your own Pens (fork someone else's on codepen.io first)"},
		{name: "wrapped", err: fmt.Errorf("writing: %w", &codepen.APIError{Status: 409, Code: "CONFLICT"}), want: conflictHint},
		{name: "other api error", err: &codepen.APIError{Status: 500, Code: "BOOM", Message: "x"}, want: "500 BOOM: x"},
		{name: "plain error", err: errors.New("disk full"), want: "disk full"},
		{name: "control characters", err: &codepen.APIError{Status: 500, Code: "BOOM", Message: "x\x1b[2J"}, want: `500 BOOM: x\x1b[2J`},
		{name: "lines kept", err: errors.Join(errors.New("a\tb"), errors.New("c")), want: "a\\tb\nc"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := explain(tt.err); got != tt.want {
				t.Errorf("explain = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestReport(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		err    error
		code   int
		stderr string
	}{
		{name: "success", err: nil, code: 0, stderr: ""},
		{name: "usage without message", err: usageError{}, code: 2, stderr: usageText() + "\n"},
		{name: "usage with message", err: usageError{err: errors.New("unknown flag")}, code: 2, stderr: "✗ unknown flag\n\n" + usageText() + "\n"},
		{name: "wrapped usage", err: fmt.Errorf("x: %w", usageError{}), code: 2, stderr: usageText() + "\n"},
		{name: "runtime error", err: errors.New("boom"), code: 1, stderr: "✗ boom\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var stderr bytes.Buffer
			code := report(Env{Stderr: &stderr}, tt.err)
			if code != tt.code || stderr.String() != tt.stderr {
				t.Errorf("code %d, stderr %q; want %d, %q", code, stderr.String(), tt.code, tt.stderr)
			}
		})
	}
}
