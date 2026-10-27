// Package dotfile makes small line-based config files (.env, .envrc,
// .gitignore) contain what the pen CLI needs without destroying what is there.
package dotfile

import (
	"errors"
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

// String returns the word pen init prints: "created", "updated" or "kept".
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

// Ensure creates path with content unless something already exists there. An
// existing file is never touched, since it may be managed by another tool.
// Creation is exclusive, so concurrent callers cannot race and a dangling
// symlink is kept.
func Ensure(path, content string, perm fs.FileMode) (Status, error) {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, perm)
	if errors.Is(err, fs.ErrExist) {
		return Kept, nil
	}
	if err != nil {
		return Unknown, fmt.Errorf("creating %q: %w", path, err)
	}
	if _, err := file.WriteString(content); err != nil {
		_ = file.Close()
		return Unknown, fmt.Errorf("writing %q: %w", path, err)
	}
	if err := file.Close(); err != nil {
		return Unknown, fmt.Errorf("closing %q: %w", path, err)
	}
	return Created, nil
}

// EnsureLines appends the lines missing from path, creating the file if needed.
// Lines are compared after trimming whitespace. It reads the whole file, so it
// is not meant for .env, which may be a FIFO.
func EnsureLines(path string, lines []string) (Status, error) {
	status, err := Ensure(path, strings.Join(lines, "\n")+"\n", 0o644)
	if err != nil || status == Created {
		return status, err
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
