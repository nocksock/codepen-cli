package human

import (
	"testing"
	"time"
)

func TestBytes(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		input    int64
		expected string
	}{
		{name: "bytes", input: 18, expected: "18 B"},
		{name: "kilobytes", input: 1536, expected: "1.5 KB"},
		{name: "megabytes", input: 3 * 1024 * 1024, expected: "3.0 MB"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := Bytes(tt.input); got != tt.expected {
				t.Errorf("Bytes(%d) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestPlural(t *testing.T) {
	t.Parallel()
	if got := Plural(1, "file"); got != "1 file" {
		t.Errorf("Plural(1) = %q", got)
	}
	if got := Plural(2, "file"); got != "2 files" {
		t.Errorf("Plural(2) = %q", got)
	}
}

func TestAgo(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 10, 24, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name     string
		offset   time.Duration
		expected string
	}{
		{name: "now", offset: 0, expected: "now"},
		{name: "seconds ago", offset: -5 * time.Second, expected: "5 seconds ago"},
		{name: "minutes ago", offset: -3 * time.Minute, expected: "3 minutes ago"},
		{name: "one hour ago", offset: -time.Hour, expected: "1 hour ago"},
		{name: "yesterday", offset: -24 * time.Hour, expected: "yesterday"},
		{name: "days ago", offset: -72 * time.Hour, expected: "3 days ago"},
		{name: "half rounds toward future", offset: -90 * time.Second, expected: "1 minute ago"},
		{name: "in the future", offset: 48 * time.Hour, expected: "in 2 days"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := Ago(now.Add(tt.offset), now); got != tt.expected {
				t.Errorf("Ago(%v) = %q, want %q", tt.offset, got, tt.expected)
			}
		})
	}
}

func TestTable(t *testing.T) {
	t.Parallel()
	got := Table([]Row{
		{Label: "title", Value: "Demo"},
		{Label: "tags", Value: ""},
		{Label: "popularity", Value: "0"},
	})
	expected := "title       Demo\npopularity  0"
	if got != expected {
		t.Errorf("Table() =\n%s\nwant\n%s", got, expected)
	}
}
