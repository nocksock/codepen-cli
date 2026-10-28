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
	// A partly written file is removed: left behind, it would be kept forever.
	if _, err := file.WriteString(content); err != nil {
		return Unknown, errors.Join(fmt.Errorf("writing %q: %w", path, err), file.Close(), os.Remove(path))
	}
	if err := file.Close(); err != nil {
		return Unknown, errors.Join(fmt.Errorf("closing %q: %w", path, err), os.Remove(path))
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
	addition := strings.Join(missing, "\n") + "\n"
	if text != "" && !strings.HasSuffix(text, "\n") {
		addition = "\n" + addition
	}
	if err := appendText(path, addition); err != nil {
		return Unknown, fmt.Errorf("writing %q: %w", path, err)
	}
	return Updated, nil
}

// appendText adds text at the end of path. Unlike rewriting the file, a
// failure can only lose the addition, never what was there.
func appendText(path, text string) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0)
	if err != nil {
		return err
	}
	if _, err := file.WriteString(text); err != nil {
		return errors.Join(err, file.Close())
	}
	return file.Close()
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
