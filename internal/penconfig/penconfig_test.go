package penconfig

import (
	"testing"

	"github.com/nocksock/codepen-cli/internal/codepen"
	"github.com/nocksock/codepen-cli/internal/tree"
)

func TestID(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{name: "with id", input: `{"blocks":{},"id":"PwWmgeN"}`, expected: "PwWmgeN"},
		{name: "without id", input: `{"blocks":{}}`, expected: ""},
		{name: "empty", input: ``, expected: ""},
		{name: "not json", input: `nope`, expected: ""},
		{name: "id not a string", input: `{"id":42}`, expected: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := ID([]byte(tt.input)); got != tt.expected {
				t.Errorf("ID(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestWithID(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{name: "adds the id", input: `{"blocks":{"a":1}}`, expected: "{\n  \"blocks\": {\n    \"a\": 1\n  },\n  \"id\": \"P1\"\n}\n"},
		{name: "replaces an id", input: `{"id":"old"}`, expected: "{\n  \"id\": \"P1\"\n}\n"},
		{name: "creates the object", input: ``, expected: "{\n  \"id\": \"P1\"\n}\n"},
		{name: "keeps big numbers exact", input: `{"n":12345678901234567890}`, expected: "{\n  \"id\": \"P1\",\n  \"n\": 12345678901234567890\n}\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := WithID([]byte(tt.input), "P1")
			if err != nil || string(got) != tt.expected {
				t.Errorf("WithID(%q) = %q, %v; want %q", tt.input, got, err, tt.expected)
			}
		})
	}
}

func TestWithID_RefusesInvalidJSON(t *testing.T) {
	t.Parallel()
	if _, err := WithID([]byte(`nope`), "P1"); err == nil {
		t.Error("WithID(invalid json) succeeded")
	}
}

func TestShared(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		input    string
		expected string
		isKept   bool
	}{
		{name: "drops the id", input: `{"blocks":[],"id":"P1"}`, expected: "{\n  \"blocks\": []\n}\n", isKept: true},
		{name: "normalises formatting", input: `{"blocks":[]}`, expected: "{\n  \"blocks\": []\n}\n", isKept: true},
		{name: "only an id is not shared", input: `{"id":"P1"}`, isKept: false},
		{name: "invalid json stays as is", input: `nope`, expected: "nope", isKept: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, isKept := Shared([]byte(tt.input))
			if isKept != tt.isKept || (isKept && string(got) != tt.expected) {
				t.Errorf("Shared(%q) = %q, %v; want %q, %v", tt.input, got, isKept, tt.expected, tt.isKept)
			}
		})
	}
}

func TestLocal(t *testing.T) {
	t.Parallel()
	local := tree.Snapshot{"/a.js": []byte("1"), Path: []byte(`{"blocks":[],"id":"P1"}`)}
	got := Local(local)
	if string(got[Path]) != "{\n  \"blocks\": []\n}\n" || string(got["/a.js"]) != "1" {
		t.Errorf("Local() = %q", got)
	}
	if string(local[Path]) != `{"blocks":[],"id":"P1"}` {
		t.Error("Local() changed its input")
	}
	idOnly := Local(tree.Snapshot{Path: []byte(`{"id":"P1"}`)})
	if _, ok := idOnly[Path]; ok {
		t.Errorf("Local() kept a config that only holds the id: %q", idOnly)
	}
}

func TestRemote(t *testing.T) {
	t.Parallel()
	config := `{"blocks":[],"id":"someone-else"}`
	other := "x"
	files := []codepen.File{{Path: Path, Content: &config}, {Path: "/a.js", Content: &other}}
	got := Remote(files)
	if *got[0].Content != "{\n  \"blocks\": []\n}\n" || *got[1].Content != "x" {
		t.Errorf("Remote() = %q, %q", *got[0].Content, *got[1].Content)
	}
	if config != `{"blocks":[],"id":"someone-else"}` {
		t.Error("Remote() changed its input")
	}
}
