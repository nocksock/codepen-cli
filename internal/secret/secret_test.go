package secret

import (
	"context"
	"errors"
	"strings"
	"testing"
)

type fakeRunner struct {
	out   string
	err   error
	calls []string
}

func (f *fakeRunner) Output(_ context.Context, name string, args ...string) ([]byte, error) {
	f.calls = append(f.calls, name+" "+strings.Join(args, " "))
	return []byte(f.out), f.err
}

func TestResolve(t *testing.T) {
	t.Parallel()
	run := &fakeRunner{out: "cp_user_real\n"}
	got, err := Resolve(t.Context(), "op://Private/CodePen/API Key", run)
	if err != nil || got != "cp_user_real" {
		t.Errorf("Resolve() = %q, %v", got, err)
	}
	if len(run.calls) != 1 || run.calls[0] != "op read op://Private/CodePen/API Key" {
		t.Errorf("calls = %q", run.calls)
	}
}

func TestResolve_PlainKey(t *testing.T) {
	t.Parallel()
	run := &fakeRunner{}
	got, err := Resolve(t.Context(), "cp_user_plain", run)
	if err != nil || got != "cp_user_plain" || len(run.calls) != 0 {
		t.Errorf("Resolve() = %q, %v, calls %q", got, err, run.calls)
	}
}

func TestResolve_Failure(t *testing.T) {
	t.Parallel()
	_, err := Resolve(t.Context(), "op://x/y/z", &fakeRunner{err: errors.New("not signed in")})
	if err == nil || !strings.Contains(err.Error(), "not signed in") {
		t.Errorf("Resolve() error = %v", err)
	}
}

func TestResolve_FailureHidesOutput(t *testing.T) {
	t.Parallel()
	run := &fakeRunner{out: "cp_user_leaky", err: errors.New("exit status 1")}
	_, err := Resolve(t.Context(), "op://x/y/z", run)
	if err == nil || strings.Contains(err.Error(), "cp_user_leaky") {
		t.Errorf("Resolve() error = %v, must not contain the output", err)
	}
	if !strings.Contains(err.Error(), "op://x/y/z") {
		t.Errorf("Resolve() error = %v, want the reference", err)
	}
}
