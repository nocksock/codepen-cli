package penconfig

import (
	"os"
	"path/filepath"
	"testing"
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

func TestSave(t *testing.T) {
	t.Parallel()
	root := filepath.Join(t.TempDir(), "new")
	if id, err := Read(root); err != nil || id != "" {
		t.Fatalf("Read(missing) = %q, %v", id, err)
	}
	if err := Save(root, "P1"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, Name), []byte(`{"id":"P1","keep":true}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Save(root, "P2"); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(root, Name))
	if err != nil || string(data) != "{\n  \"id\": \"P2\",\n  \"keep\": true\n}\n" {
		t.Errorf("config = %q, %v", data, err)
	}
	if id, err := Read(root); err != nil || id != "P2" {
		t.Errorf("Read() = %q, %v", id, err)
	}
}

func TestSave_RefusesInvalidJSON(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, Name), []byte("nope"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Save(root, "P1"); err == nil {
		t.Error("Save() over invalid json succeeded")
	}
}
