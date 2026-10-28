package ref

import (
	"strings"
	"testing"
)

const uuid = "01a120bd-45fb-7095-a3ea-622e774cf8f0"

func TestParse(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{name: "bare uuid", input: uuid, expected: uuid},
		{name: "bare classic id with whitespace", input: " PwWmgeN\n", expected: "PwWmgeN"},
		{name: "2.0 editor url", input: "https://codepen.io/editor/nocksock/pen/" + uuid + "/3f4d5706ac5597fbe5baf7e5126fa018", expected: uuid},
		{name: "classic pen url", input: "https://codepen.io/nocksock/pen/PwWmgeN", expected: "PwWmgeN"},
		{name: "classic pen url with hash", input: "https://codepen.io/nocksock/pen/PwWmgeN/23e786f57891fa2384306566d37b3f34", expected: "PwWmgeN"},
		{name: "full view", input: "https://codepen.io/nocksock/full/PwWmgeN", expected: "PwWmgeN"},
		{name: "details with query and fragment", input: "https://codepen.io/nocksock/details/PwWmgeN?foo=bar#x", expected: "PwWmgeN"},
		{name: "embed preview", input: "https://codepen.io/nocksock/embed/preview/PwWmgeN", expected: "PwWmgeN"},
		{name: "cdpn.io grid", input: "https://cdpn.io/username/fullcpgrid/PwWmgeN/23e786f57891fa2384306566d37b3f34/", expected: "PwWmgeN"},
		{name: "without scheme", input: "codepen.io/nocksock/pen/PwWmgeN", expected: "PwWmgeN"},
		{name: "template uuid", input: "https://codepen.io/pen/?template=" + uuid, expected: uuid},
		{name: "uppercase uuid", input: strings.ToUpper(uuid), expected: strings.ToUpper(uuid)},
		{name: "template wins over path id", input: "https://codepen.io/u/pen/PwWmgeN?template=" + uuid, expected: uuid},
		{name: "editor url with classic id", input: "https://codepen.io/editor/nocksock/pen/PwWmgeN", expected: "PwWmgeN"},
		{name: "debug on cdpn.io", input: "https://cdpn.io/pen/debug/PwWmgeN", expected: "PwWmgeN"},
		{name: "template classic", input: "https://codepen.io/pen?template=PwWmgeN", expected: "PwWmgeN"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := Parse(tt.input)
			if err != nil {
				t.Fatalf("Parse(%q) error: %v", tt.input, err)
			}
			if got != tt.expected {
				t.Errorf("Parse(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestParse_Rejects(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		input string
	}{
		{name: "empty", input: ""},
		{name: "profile url", input: "https://codepen.io/nocksock"},
		{name: "codepen.dev host", input: "https://3f4d.codepen.dev/"},
		{name: "prose", input: "not an id"},
		{name: "bare view word", input: "editor"},
		{name: "view without id", input: "https://codepen.io/pen/"},
		{name: "editor without id", input: "https://codepen.io/editor/nocksock"},
		{name: "id not right after the view", input: "https://codepen.io/u/pen/x-y/PwWmgeN"},
		{name: "percent-encoded id", input: "https://codepen.io/u/pen/Pw%57mgeN"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := Parse(tt.input)
			if err == nil || !strings.Contains(err.Error(), "can't find a Pen id") {
				t.Errorf("Parse(%q) error = %v, want \"can't find a Pen id\"", tt.input, err)
			}
		})
	}
}

func FuzzParse(f *testing.F) {
	f.Add(uuid)
	f.Add("https://codepen.io/nocksock/pen/PwWmgeN")
	f.Add("codepen.io/pen?template=PwWmgeN")
	f.Fuzz(func(t *testing.T, input string) {
		id, err := Parse(input)
		if err == nil && !isID(id) {
			t.Errorf("Parse(%q) = %q, which is not an id", input, id)
		}
	})
}
