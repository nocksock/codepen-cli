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
		{goos: "darwin", name: "open", args: []string{"https://x/?a&b"}},
		{goos: "windows", name: "rundll32", args: []string{"url.dll,FileProtocolHandler", "https://x/?a&b"}},
		{goos: "linux", name: "xdg-open", args: []string{"https://x/?a&b"}},
		{goos: "freebsd", name: "xdg-open", args: []string{"https://x/?a&b"}},
	}
	for _, tt := range tests {
		t.Run(tt.goos, func(t *testing.T) {
			t.Parallel()
			name, args, err := Command(tt.goos, "https://x/?a&b")
			if err != nil || name != tt.name || !slices.Equal(args, tt.args) {
				t.Errorf("Command(%s) = %s %q, %v", tt.goos, name, args, err)
			}
		})
	}
}

func TestCommand_RefusesOtherSchemes(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		url  string
	}{
		{name: "file url", url: "file:///etc/passwd"},
		{name: "custom scheme", url: "vscode://open?x"},
		{name: "javascript", url: "javascript:alert(1)"},
		{name: "no scheme", url: "codepen.io/x"},
		{name: "unparsable", url: "http://[::1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if _, _, err := Command("linux", tt.url); err == nil {
				t.Errorf("Command(%q) succeeded, want refusal", tt.url)
			}
		})
	}
}
