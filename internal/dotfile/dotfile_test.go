package dotfile

import (
	"os"
	"path/filepath"
	"testing"
)

func TestEnsure(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), ".env")
	status, err := Ensure(path, "A=1\n", 0o600)
	if err != nil || status != Created {
		t.Fatalf("first Ensure = %v, %v; want created", status, err)
	}
	status, err = Ensure(path, "B=2\n", 0o600)
	if err != nil || status != Kept {
		t.Fatalf("second Ensure = %v, %v; want kept", status, err)
	}
	assertFile(t, path, "A=1\n")
}

func TestEnsureLines(t *testing.T) {
	t.Parallel()
	lines := []string{".env", ".env.local"}
	tests := []struct {
		name     string
		before   *string
		status   Status
		expected string
	}{
		{name: "creates", before: nil, status: Created, expected: ".env\n.env.local\n"},
		{name: "appends missing lines", before: ptr("node_modules\n.env\n"), status: Updated, expected: "node_modules\n.env\n.env.local\n"},
		{name: "adds a separator", before: ptr("export FOO=1"), status: Updated, expected: "export FOO=1\n.env\n.env.local\n"},
		{name: "keeps a complete file", before: ptr(" .env \n.env.local\n"), status: Kept, expected: " .env \n.env.local\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			path := filepath.Join(t.TempDir(), ".gitignore")
			if tt.before != nil {
				if err := os.WriteFile(path, []byte(*tt.before), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			status, err := EnsureLines(path, lines)
			if err != nil || status != tt.status {
				t.Fatalf("EnsureLines = %v, %v; want %v", status, err, tt.status)
			}
			assertFile(t, path, tt.expected)
		})
	}
}

func TestStatus_String(t *testing.T) {
	t.Parallel()
	for status, expected := range map[Status]string{Unknown: "unknown", Created: "created", Updated: "updated", Kept: "kept"} {
		if got := status.String(); got != expected {
			t.Errorf("%d.String() = %q, want %q", status, got, expected)
		}
	}
}

func assertFile(t *testing.T, path, expected string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != expected {
		t.Errorf("%s = %q, want %q", filepath.Base(path), data, expected)
	}
}

func ptr(s string) *string { return &s }
