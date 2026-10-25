// Package envfile reads dotenv files (KEY=value lines).
package envfile

import (
	"errors"
	"fmt"
	"io/fs"
	"iter"
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

// Parse reads KEY=value entries like Node's util.parseEnv. It understands
// "export " prefixes, whole-line # comments, and single, double or backtick
// quotes (\n is expanded in double quotes). A quoted value may span several
// lines. An unquoted value ends at the first #, so "A=abc#def" is "abc".
func Parse(text string) map[string]string {
	values := map[string]string{}
	for key, raw := range entries(text) {
		values[key] = unquote(raw)
	}
	return values
}

// entries yields each key with its raw (still quoted) value. A value whose
// opening quote closes on a later line takes all lines up to that quote.
func entries(text string) iter.Seq2[string, string] {
	return func(yield func(string, string) bool) {
		rest := text
		for rest != "" {
			var line string
			line, rest, _ = strings.Cut(rest, "\n")
			key, raw, ok := splitEntry(line)
			if !ok {
				continue
			}
			if isUnclosed(raw) {
				if end := strings.IndexByte(rest, raw[0]); end >= 0 {
					raw += "\n" + rest[:end+1]
					_, rest, _ = strings.Cut(rest[end:], "\n")
				}
			}
			if !yield(key, raw) {
				return
			}
		}
	}
}

// splitEntry splits a line into its key and raw value.
func splitEntry(line string) (key, raw string, ok bool) {
	line = strings.TrimSpace(line)
	if line == "" || strings.HasPrefix(line, "#") {
		return "", "", false
	}
	line = strings.TrimPrefix(line, "export ")
	key, raw, ok = strings.Cut(line, "=")
	key = strings.TrimSpace(key)
	if !ok || key == "" {
		return "", "", false
	}
	return key, strings.TrimSpace(raw), true
}

func isQuote(c byte) bool { return c == '"' || c == '\'' || c == '`' }

// isUnclosed reports whether raw opens a quote that its own line never closes.
func isUnclosed(raw string) bool {
	return raw != "" && isQuote(raw[0]) && !strings.Contains(raw[1:], raw[:1])
}

// unquote strips matching quotes or, from an unquoted value, a trailing #
// comment.
func unquote(raw string) string {
	if raw == "" {
		return raw
	}
	quote := raw[0]
	if isQuote(quote) {
		if end := strings.IndexByte(raw[1:], quote); end >= 0 {
			inner := raw[1 : end+1]
			if quote == '"' {
				inner = strings.ReplaceAll(inner, `\n`, "\n")
			}
			return inner
		}
	}
	before, _, _ := strings.Cut(raw, "#")
	return strings.TrimSpace(before)
}
