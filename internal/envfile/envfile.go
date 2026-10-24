// Package envfile reads and edits dotenv files (KEY=value lines).
package envfile

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strings"
)

// Read parses the dotenv file at path; a missing file has no values. The file
// is read exactly once, so it may be a FIFO such as a 1Password Environments mount.
func Read(path string) (map[string]string, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return map[string]string{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading %q: %w", path, err)
	}
	return Parse(string(data)), nil
}

// Parse reads KEY=value lines. It understands "export " prefixes, # comments,
// and single, double or backtick quotes (\n is expanded in double quotes).
func Parse(text string) map[string]string {
	values := map[string]string{}
	for line := range strings.Lines(text) {
		if key, value, ok := parseLine(line); ok {
			values[key] = value
		}
	}
	return values
}

func parseLine(line string) (key, value string, ok bool) {
	line = strings.TrimSpace(line)
	if line == "" || strings.HasPrefix(line, "#") {
		return "", "", false
	}
	line = strings.TrimPrefix(line, "export ")
	key, value, ok = strings.Cut(line, "=")
	key = strings.TrimSpace(key)
	if !ok || key == "" {
		return "", "", false
	}
	return key, unquote(strings.TrimSpace(value)), true
}

// unquote strips matching quotes or, from an unquoted value, a " # comment".
func unquote(value string) string {
	if value == "" {
		return value
	}
	quote := value[0]
	if quote == '"' || quote == '\'' || quote == '`' {
		if end := strings.IndexByte(value[1:], quote); end >= 0 {
			inner := value[1 : end+1]
			if quote == '"' {
				inner = strings.ReplaceAll(inner, `\n`, "\n")
			}
			return inner
		}
	}
	before, _, _ := strings.Cut(value, " #")
	return strings.TrimSpace(before)
}

// Set writes KEY=value into the file at path, replacing the first KEY= line or
// appending one. A new file gets mode 0600, since env files may hold secrets.
func Set(path, key, value string) error {
	data, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("reading %q: %w", path, err)
	}
	text := replaceLine(string(data), key+"=", key+"="+value)
	if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
		return fmt.Errorf("writing %q: %w", path, err)
	}
	return nil
}

// replaceLine swaps the first line starting with prefix for line, keeping its
// line ending, or appends line when there is none.
func replaceLine(text, prefix, line string) string {
	lines := strings.SplitAfter(text, "\n")
	for i, current := range lines {
		if strings.HasPrefix(current, prefix) {
			ending := current[len(strings.TrimRight(current, "\r\n")):]
			lines[i] = line + ending
			return strings.Join(lines, "")
		}
	}
	if text != "" && !strings.HasSuffix(text, "\n") {
		text += "\n"
	}
	return text + line + "\n"
}
