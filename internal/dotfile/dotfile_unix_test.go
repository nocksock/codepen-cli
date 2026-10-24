//go:build unix

package dotfile

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

func TestEnsure_fifo(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), ".env")
	if err := syscall.Mkfifo(path, 0o600); err != nil {
		t.Skipf("mkfifo unsupported: %v", err)
	}
	done := make(chan Status, 1)
	go func() {
		status, _ := Ensure(path, "A=1\n", 0o600)
		done <- status
	}()
	select {
	case status := <-done:
		if status != Kept {
			t.Errorf("Ensure = %v, want kept", status)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Ensure blocked on a fifo")
	}
}

func TestEnsure_danglingSymlink(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	target := filepath.Join(dir, "target")
	path := filepath.Join(dir, ".env")
	if err := os.Symlink(target, path); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}
	status, err := Ensure(path, "A=1\n", 0o600)
	if err != nil || status != Kept {
		t.Fatalf("Ensure = %v, %v; want kept", status, err)
	}
	if _, err := os.Lstat(target); !os.IsNotExist(err) {
		t.Errorf("target was created: %v", err)
	}
}
