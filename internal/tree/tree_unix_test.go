//go:build unix

package tree

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWrite_KeepsFileMode(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	file := filepath.Join(root, "run.sh")
	if err := os.WriteFile(file, []byte("old"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(file, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := Write(root, "/run.sh", strings.NewReader("new")); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(file)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o755 {
		t.Errorf("mode = %v, want 0755", info.Mode().Perm())
	}
}

// The umask must not take bits away from a file's existing mode.
func TestWrite_KeepsModeDespiteUmask(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	file := filepath.Join(root, "shared.sh")
	if err := os.WriteFile(file, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(file, 0o777); err != nil { // not limited by the umask
		t.Fatal(err)
	}
	if err := Write(root, "/shared.sh", strings.NewReader("new")); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(file)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o777 {
		t.Errorf("mode = %v, want 0777", info.Mode().Perm())
	}
}
