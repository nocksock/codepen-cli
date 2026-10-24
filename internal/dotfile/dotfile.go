// Package dotfile makes small line-based config files (.env, .envrc,
// .gitignore) contain what cdpn needs without destroying what is there.
package dotfile

import (
	"fmt"
	"io/fs"
	"os"
	"strings"
)

// Status says what Ensure or EnsureLines did to a file.
type Status int

// Statuses; the zero value is Unknown so an unset Status is never mistaken for a result.
const (
	Unknown Status = iota
	Created
	Updated
	Kept
)

// String returns the word cdpn init prints: "created", "updated" or "kept".
func (s Status) String() string {
	switch s {
	case Created:
		return "created"
	case Updated:
		return "updated"
	case Kept:
		return "kept"
	default:
		return "unknown"
	}
}

// Ensure creates path with content unless it exists. An existing file is never
// touched: .env may be a 1Password mount.
func Ensure(path, content string, perm fs.FileMode) (Status, error) {
	if exists(path) {
		return Kept, nil
	}
	if err := os.WriteFile(path, []byte(content), perm); err != nil {
		return Unknown, fmt.Errorf("creating %q: %w", path, err)
	}
	return Created, nil
}

// EnsureLines appends the lines missing from path, creating the file if needed.
// Lines are compared after trimming whitespace.
func EnsureLines(path string, lines []string) (Status, error) {
	if !exists(path) {
		return Ensure(path, strings.Join(lines, "\n")+"\n", 0o644)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return Unknown, fmt.Errorf("reading %q: %w", path, err)
	}
	text := string(data)
	missing := missingLines(text, lines)
	if len(missing) == 0 {
		return Kept, nil
	}
	if text != "" && !strings.HasSuffix(text, "\n") {
		text += "\n"
	}
	text += strings.Join(missing, "\n") + "\n"
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		return Unknown, fmt.Errorf("writing %q: %w", path, err)
	}
	return Updated, nil
}

func missingLines(text string, lines []string) []string {
	present := map[string]bool{}
	for line := range strings.Lines(text) {
		present[strings.TrimSpace(line)] = true
	}
	missing := []string{}
	for _, line := range lines {
		if !present[line] {
			missing = append(missing, line)
		}
	}
	return missing
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
