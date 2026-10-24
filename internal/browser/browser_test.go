package browser

import (
	"slices"
	"testing"
)

func TestCommand(t *testing.T) {
	t.Parallel()
	tests := []struct {
		goos string
		name string
		args []string
	}{
		{goos: "darwin", name: "open", args: []string{"https://x"}},
		{goos: "windows", name: "cmd", args: []string{"/c", "start", "", "https://x"}},
		{goos: "linux", name: "xdg-open", args: []string{"https://x"}},
		{goos: "freebsd", name: "xdg-open", args: []string{"https://x"}},
	}
	for _, tt := range tests {
		t.Run(tt.goos, func(t *testing.T) {
			t.Parallel()
			name, args := Command(tt.goos, "https://x")
			if name != tt.name || !slices.Equal(args, tt.args) {
				t.Errorf("Command(%s) = %s %q", tt.goos, name, args)
			}
		})
	}
}
