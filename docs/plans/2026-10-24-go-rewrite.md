# cdpn Go Rewrite Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use mine:subagent-driven-development (recommended) or mine:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the Node prototype of `cdpn` with a Go implementation that behaves the same, plus atomic writes, retries, a watcher that can't silently go deaf, and exit code 2 for usage errors.

**Architecture:** `cmd/cdpn` only wires signals → `internal/cli`. `cli` (Cobra) composes three domain packages (`pen`, `info`, `setup`), which are written in terms of small building blocks (`codepen`, `tree`, `diff`, `ref`, `config`, `envfile`, `secret`, `dotfile`, `watch`, `human`, `browser`). All side effects enter through `cli.Env`, so the CLI scenario tests run in-process against an `httptest` fake API.

**Tech Stack:** Go 1.25, `github.com/spf13/cobra@v1.10.2`, `github.com/fsnotify/fsnotify@v1.10.1`, `golang.org/x/sync@v0.20.0` (v0.24 needs Go 1.26), `go.uber.org/goleak@v1.3.0` (tests). goreleaser and golangci-lint via `nix run`.

**Spec:** `/home/nr/sandboxes/codepen-api-prototyping/docs/specs/2026-10-24-go-rewrite-design.md`

---

## Ground rules for every task

- Work in the worktree `/home/nr/sandboxes/codepen-api-prototyping/pen-rj8c` (branch `task/pen-rj8c`). Never `cd` elsewhere; use absolute paths or run from there.
- The Node prototype (`bin/cdpn`, `lib/`, `test/`) is the behavioural reference. Read the matching `lib/*.js` file before each task. It stays in place until Task 19. **Do not run `just build` before Task 19**: it would overwrite the tracked Node `bin/cdpn`.
- Go style: early returns; named composite literals; ≤4 params; lowercase error strings wrapped with `%w`, `%q` for paths; initialised maps/slices; doc comment on every exported identifier; unexported by default.
- Tests: standard library only (+ goleak); table-driven with lowercase subtest names; one `_test.go` per source file; `t.Parallel()` where there is no shared state.
- Run `gofmt -l .` before each commit; it must print nothing.
- Commit messages: conventional commits, ending with
  `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>`.
  Beans live in the hub; bean edits are committed in the hub (`git -C /home/nr/sandboxes/codepen-api-prototyping …`), not in the worktree.

## File map

```
go.mod, go.sum, justfile, .golangci.yml, .goreleaser.yaml
cmd/cdpn/main.go, main_test.go               entry point + binary smoke test
internal/cli/cli.go                          Env, Runner, System, Run
internal/cli/app.go                          app: config/client/mirror wiring
internal/cli/runner.go                       execRunner (os/exec, no shell)
internal/cli/root.go                         root command, usage text, version
internal/cli/errors.go                       usageError, explain, exitCode
internal/cli/render.go                       printer: pen.Event → output lines
internal/cli/{push,pull,sync,info,open,init}.go   one Cobra command each
internal/cli/*_test.go                       scenario tests (workspace_test.go = helpers)
internal/pen/mirror.go, event.go             Mirror (push/pull/create), Events
internal/info/info.go                        Gather + Render
internal/setup/setup.go                      Scaffold + AllowDirenv
internal/codepen/{client,error,retry,payload,endpoints,types}.go
internal/codepen/codepentest/server.go       fake API
internal/tree/tree.go
internal/diff/diff.go
internal/ref/ref.go
internal/config/config.go
internal/envfile/envfile.go
internal/secret/secret.go
internal/dotfile/dotfile.go
internal/watch/watch.go
internal/human/human.go
internal/browser/browser.go
```

---

### Task 1: Go module and `ref`

**Files:**
- Create: `go.mod`, `justfile`, `.golangci.yml`, `internal/ref/ref.go`
- Test: `internal/ref/ref_test.go`

- [ ] **Step 1: Create the module**

Run:
```bash
cd /home/nr/sandboxes/codepen-api-prototyping/pen-rj8c && go mod init github.com/nocksock/codepen-cli
```
Expected: `go: creating new go.mod: module github.com/nocksock/codepen-cli`. Then edit `go.mod` so the go line reads `go 1.25`.

- [ ] **Step 2: Add `justfile`**

```just
set shell := ["bash", "-cu"]

version := `git describe --tags --always --dirty 2>/dev/null || echo dev`
ldflags := "-s -w -X github.com/nocksock/codepen-cli/internal/cli.version=" + version

# List recipes
default:
    @just --list

# Build bin/cdpn (on PATH via .envrc)
build:
    go build -trimpath -ldflags '{{ldflags}}' -o bin/cdpn ./cmd/cdpn

# Run all tests with the race detector, in random order
test:
    go test -race -shuffle=on ./...

# Lint with golangci-lint (falls back to nix)
lint:
    if command -v golangci-lint >/dev/null; then golangci-lint run ./...; else nix run nixpkgs#golangci-lint -- run ./...; fi

# Cross-compile all release archives into dist/ without publishing
snapshot:
    if command -v goreleaser >/dev/null; then goreleaser release --snapshot --clean; else nix run nixpkgs#goreleaser -- release --snapshot --clean; fi

# Install cdpn into $GOBIN
install:
    go install -trimpath -ldflags '{{ldflags}}' ./cmd/cdpn
```

- [ ] **Step 3: Add `.golangci.yml`**

```yaml
version: "2"
linters:
  default: standard
  enable:
    - bodyclose
    - errorlint
    - gocritic
    - misspell
    - nilerr
    - revive
    - thelper
    - unconvert
formatters:
  enable:
    - gofumpt
    - goimports
```

- [ ] **Step 4: Write the failing test** — `internal/ref/ref_test.go` (port of `test/ref.test.js` plus a fuzz target)

```go
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
	for _, input := range []string{"", "https://codepen.io/nocksock", "https://3f4d.codepen.dev/", "not an id"} {
		t.Run(input, func(t *testing.T) {
			t.Parallel()
			_, err := Parse(input)
			if err == nil || !strings.Contains(err.Error(), "can't find a Pen id") {
				t.Errorf("Parse(%q) error = %v, want \"can't find a Pen id\"", input, err)
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
```

- [ ] **Step 5: Run it to verify it fails**

Run: `go test ./internal/ref/`
Expected: FAIL, `undefined: Parse`.

- [ ] **Step 6: Implement** — `internal/ref/ref.go`

```go
// Package ref turns anything that points at a Pen into its API id:
//
//	01a120bd-45fb-7095-a3ea-622e774cf8f0               2.0 Pen id
//	PwWmgeN                                            Classic Pen id
//	https://codepen.io/editor/user/pen/<uuid>/<hash>   2.0 editor
//	https://codepen.io/user/pen/PwWmgeN  (/full, …)    Classic / public
//	https://codepen.io/pen/?template=<id>              template
//	codepen.io/user/embed/preview/PwWmgeN              without scheme
package ref

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

var (
	uuidPattern    = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
	classicPattern = regexp.MustCompile(`^[A-Za-z0-9]{5,10}$`)
	schemePattern  = regexp.MustCompile(`(?i)^[a-z]+://`)
)

// views are path segments naming a view of a Pen; the Classic id follows one of them.
var views = map[string]bool{
	"pen": true, "full": true, "details": true, "debug": true, "embed": true, "preview": true,
	"pres": true, "live": true, "fullpage": true, "fullcpgrid": true, "fullembedgrid": true, "editor": true,
}

// Parse returns the Pen id that ref points at: a bare id or any URL containing one.
func Parse(ref string) (string, error) {
	value := strings.TrimSpace(ref)
	if isID(value) {
		return value, nil
	}
	if id := fromURL(value); id != "" {
		return id, nil
	}
	return "", fmt.Errorf("can't find a Pen id in %q", ref)
}

func isID(value string) bool {
	return uuidPattern.MatchString(value) || (classicPattern.MatchString(value) && !views[value])
}

func fromURL(value string) string {
	if !schemePattern.MatchString(value) {
		value = "https://" + value
	}
	u, err := url.Parse(value)
	if err != nil {
		return ""
	}
	if template := u.Query().Get("template"); isID(template) {
		return template
	}
	segments := strings.FieldsFunc(u.Path, func(r rune) bool { return r == '/' })
	for _, segment := range segments {
		if uuidPattern.MatchString(segment) {
			return segment
		}
	}
	return afterView(segments)
}

// afterView returns the first id following the first view segment.
func afterView(segments []string) string {
	for i, segment := range segments {
		if !views[segment] {
			continue
		}
		for _, next := range segments[i+1:] {
			if isID(next) {
				return next
			}
		}
		return ""
	}
	return ""
}
```

- [ ] **Step 7: Run tests and a short fuzz**

Run: `go test ./internal/ref/ && go test ./internal/ref/ -run '^$' -fuzz FuzzParse -fuzztime 10s`
Expected: `ok`, and the fuzz run ends with `PASS`. If the fuzzer finds a failure, it writes `internal/ref/testdata/fuzz/…`; fix the code and keep the corpus file.

- [ ] **Step 8: Commit**

```bash
git add go.mod justfile .golangci.yml internal/ref
git commit -m "feat(ref): port pen ref parsing to go

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 2: `human`

**Files:**
- Create: `internal/human/human.go`
- Test: `internal/human/human_test.go`

Reference: `lib/format.js`.

- [ ] **Step 1: Write the failing test**

```go
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
```

- [ ] **Step 2: Run it to verify it fails**

Run: `go test ./internal/human/`
Expected: FAIL, `undefined: Bytes`.

- [ ] **Step 3: Implement** — `internal/human/human.go`

```go
// Package human formats values for people reading a terminal.
package human

import (
	"fmt"
	"math"
	"strings"
	"time"
)

var units = []string{"B", "KB", "MB", "GB"}

// Bytes formats n as "18 B", "1.5 KB", "3.0 MB".
func Bytes(n int64) string {
	value, unit := float64(n), 0
	for value >= 1024 && unit < len(units)-1 {
		value /= 1024
		unit++
	}
	if unit == 0 {
		return fmt.Sprintf("%d %s", n, units[0])
	}
	return fmt.Sprintf("%.1f %s", value, units[unit])
}

// Plural formats "1 file" or "2 files".
func Plural(n int, word string) string {
	if n == 1 {
		return "1 " + word
	}
	return fmt.Sprintf("%d %ss", n, word)
}

type span struct {
	unit    string
	seconds float64
}

var spans = []span{
	{unit: "year", seconds: 31536000},
	{unit: "month", seconds: 2592000},
	{unit: "day", seconds: 86400},
	{unit: "hour", seconds: 3600},
	{unit: "minute", seconds: 60},
}

// named holds the words that replace -1, 0 and +1 of a unit, as
// Intl.RelativeTimeFormat("en", {numeric: "auto"}) does.
var named = map[string][3]string{
	"second": {"", "now", ""},
	"minute": {"", "this minute", ""},
	"hour":   {"", "this hour", ""},
	"day":    {"yesterday", "today", "tomorrow"},
	"month":  {"last month", "this month", "next month"},
	"year":   {"last year", "this year", "next year"},
}

// Ago formats t relative to now: "now", "5 minutes ago", "yesterday", "in 2 days".
func Ago(t, now time.Time) string {
	seconds := t.Sub(now).Seconds()
	unit, size := "second", 1.0
	for _, s := range spans {
		if math.Abs(seconds) >= s.seconds {
			unit, size = s.unit, s.seconds
			break
		}
	}
	return relative(int(math.Round(seconds/size)), unit)
}

func relative(n int, unit string) string {
	if n >= -1 && n <= 1 {
		if word := named[unit][n+1]; word != "" {
			return word
		}
	}
	if n < 0 {
		return Plural(-n, unit) + " ago"
	}
	return "in " + Plural(n, unit)
}

// Row is one "label  value" line of a Table.
type Row struct {
	Label string
	Value string
}

// Table aligns rows into "label  value" lines and drops rows without a value.
func Table(rows []Row) string {
	shown := []Row{}
	width := 0
	for _, row := range rows {
		if row.Value == "" {
			continue
		}
		shown = append(shown, row)
		width = max(width, len(row.Label))
	}
	lines := make([]string, 0, len(shown))
	for _, row := range shown {
		lines = append(lines, fmt.Sprintf("%-*s  %s", width, row.Label, row.Value))
	}
	return strings.Join(lines, "\n")
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/human/`
Expected: `ok`.

- [ ] **Step 5: Commit**

```bash
git add internal/human
git commit -m "feat(human): add terminal formatting primitives

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 3: `envfile`

**Files:**
- Create: `internal/envfile/envfile.go`
- Test: `internal/envfile/envfile_test.go`

Reference: `lib/env.js` (`load`, `set`).

- [ ] **Step 1: Write the failing test**

```go
package envfile

import (
	"maps"
	"os"
	"path/filepath"
	"testing"
)

func TestParse(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		input    string
		expected map[string]string
	}{
		{name: "plain", input: "A=1\nB=two\n", expected: map[string]string{"A": "1", "B": "two"}},
		{name: "double quotes with newline escape", input: `A="x y\nz"`, expected: map[string]string{"A": "x y\nz"}},
		{name: "single quotes stay literal", input: `A='x\ny'`, expected: map[string]string{"A": `x\ny`}},
		{name: "op ref with spaces", input: `CODEPEN_API_KEY="op://Private/CodePen/API Key"`, expected: map[string]string{"CODEPEN_API_KEY": "op://Private/CodePen/API Key"}},
		{name: "export prefix", input: "export A=1", expected: map[string]string{"A": "1"}},
		{name: "comments and blanks", input: "# note\n\nA=1 # trailing\n", expected: map[string]string{"A": "1"}},
		{name: "crlf", input: "A=1\r\nB=2\r\n", expected: map[string]string{"A": "1", "B": "2"}},
		{name: "lines without key", input: "=1\nnope\n", expected: map[string]string{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := Parse(tt.input); !maps.Equal(got, tt.expected) {
				t.Errorf("Parse(%q) = %v, want %v", tt.input, got, tt.expected)
			}
		})
	}
}

func TestRead(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	got, err := Read(filepath.Join(dir, "missing"))
	if err != nil || len(got) != 0 {
		t.Fatalf("Read(missing) = %v, %v; want empty, nil", got, err)
	}
	path := filepath.Join(dir, ".env")
	if err := os.WriteFile(path, []byte("A=1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err = Read(path)
	if err != nil || got["A"] != "1" {
		t.Errorf("Read() = %v, %v; want A=1", got, err)
	}
}

func TestSet(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		before   *string
		expected string
	}{
		{name: "creates the file", before: nil, expected: "CODEPEN_ID=new\n"},
		{name: "replaces the line", before: ptr("A=1\nCODEPEN_ID=old\nB=2\n"), expected: "A=1\nCODEPEN_ID=new\nB=2\n"},
		{name: "appends with a separator", before: ptr("A=1"), expected: "A=1\nCODEPEN_ID=new\n"},
		{name: "appends to an empty file", before: ptr(""), expected: "CODEPEN_ID=new\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			path := filepath.Join(t.TempDir(), ".env.local")
			if tt.before != nil {
				if err := os.WriteFile(path, []byte(*tt.before), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			if err := Set(path, "CODEPEN_ID", "new"); err != nil {
				t.Fatal(err)
			}
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if string(data) != tt.expected {
				t.Errorf("file = %q, want %q", data, tt.expected)
			}
		})
	}
}

func TestSet_CreatesPrivateFile(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), ".env.local")
	if err := Set(path, "A", "1"); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if mode := info.Mode().Perm(); mode != 0o600 {
		t.Errorf("mode = %o, want 600", mode)
	}
}

func ptr(s string) *string { return &s }
```

- [ ] **Step 2: Run it to verify it fails**

Run: `go test ./internal/envfile/`
Expected: FAIL, `undefined: Parse`.

- [ ] **Step 3: Implement** — `internal/envfile/envfile.go`

```go
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
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/envfile/`
Expected: `ok`.

- [ ] **Step 5: Commit**

```bash
git add internal/envfile
git commit -m "feat(envfile): read and edit dotenv files

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 4: `dotfile`

**Files:**
- Create: `internal/dotfile/dotfile.go`
- Test: `internal/dotfile/dotfile_test.go`

Reference: `lib/dotfile.js`.

- [ ] **Step 1: Write the failing test**

```go
package dotfile

import (
	"os"
	"path/filepath"
	"testing"
)

func TestEnsure(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), ".env")
	status, err := Ensure(path, "A=1\n", 0o600)
	if err != nil || status != Created {
		t.Fatalf("first Ensure = %v, %v; want created", status, err)
	}
	status, err = Ensure(path, "B=2\n", 0o600)
	if err != nil || status != Kept {
		t.Fatalf("second Ensure = %v, %v; want kept", status, err)
	}
	assertFile(t, path, "A=1\n")
}

func TestEnsureLines(t *testing.T) {
	t.Parallel()
	lines := []string{".env", ".env.local"}
	tests := []struct {
		name     string
		before   *string
		status   Status
		expected string
	}{
		{name: "creates", before: nil, status: Created, expected: ".env\n.env.local\n"},
		{name: "appends missing lines", before: ptr("node_modules\n.env\n"), status: Updated, expected: "node_modules\n.env\n.env.local\n"},
		{name: "adds a separator", before: ptr("export FOO=1"), status: Updated, expected: "export FOO=1\n.env\n.env.local\n"},
		{name: "keeps a complete file", before: ptr(" .env \n.env.local\n"), status: Kept, expected: " .env \n.env.local\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			path := filepath.Join(t.TempDir(), ".gitignore")
			if tt.before != nil {
				if err := os.WriteFile(path, []byte(*tt.before), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			status, err := EnsureLines(path, lines)
			if err != nil || status != tt.status {
				t.Fatalf("EnsureLines = %v, %v; want %v", status, err, tt.status)
			}
			assertFile(t, path, tt.expected)
		})
	}
}

func TestStatus_String(t *testing.T) {
	t.Parallel()
	for status, expected := range map[Status]string{Unknown: "unknown", Created: "created", Updated: "updated", Kept: "kept"} {
		if got := status.String(); got != expected {
			t.Errorf("%d.String() = %q, want %q", status, got, expected)
		}
	}
}

func assertFile(t *testing.T, path, expected string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != expected {
		t.Errorf("%s = %q, want %q", filepath.Base(path), data, expected)
	}
}

func ptr(s string) *string { return &s }
```

- [ ] **Step 2: Run it to verify it fails**

Run: `go test ./internal/dotfile/`
Expected: FAIL, `undefined: Ensure`.

- [ ] **Step 3: Implement** — `internal/dotfile/dotfile.go`

```go
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
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/dotfile/`
Expected: `ok`.

- [ ] **Step 5: Commit**

```bash
git add internal/dotfile
git commit -m "feat(dotfile): add non-destructive dotfile writes

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 5: `tree`

**Files:**
- Create: `internal/tree/tree.go`
- Test: `internal/tree/tree_test.go`

Reference: `lib/tree.js`. Differences: ignored directories are not descended into, unfinished writes (`*.cdpn-tmp`) are ignored, and `Write` is atomic and confined to the root via `os.Root`. Remote paths are untrusted.

- [ ] **Step 1: Write the failing test**

```go
package tree

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestIsIgnored(t *testing.T) {
	t.Parallel()
	tests := []struct {
		path     string
		expected bool
	}{
		{path: "/index.html", expected: false},
		{path: "/src/app.js", expected: false},
		{path: "/.git/config", expected: true},
		{path: "/node_modules/dep/index.js", expected: true},
		{path: "/.env", expected: true},
		{path: "/.env.production", expected: true},
		{path: "/.envrc", expected: true},
		{path: "/img/.DS_Store", expected: true},
		{path: "/a.js.cdpn-tmp", expected: true},
		{path: "/.environment.js", expected: false},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			t.Parallel()
			if got := IsIgnored(tt.path); got != tt.expected {
				t.Errorf("IsIgnored(%q) = %v, want %v", tt.path, got, tt.expected)
			}
		})
	}
}

func TestRead(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	write(t, root, "index.html", "<h1>x</h1>")
	write(t, root, "src/app.js", "1")
	write(t, root, "empty.js", "")
	write(t, root, "node_modules/dep/index.js", "x")
	write(t, root, ".env.local", "CODEPEN_ID=x")
	if err := os.Symlink(filepath.Join(root, "index.html"), filepath.Join(root, "link.html")); err != nil {
		t.Fatal(err)
	}

	got, err := Read(root)
	if err != nil {
		t.Fatal(err)
	}
	expected := map[string]string{"/index.html": "<h1>x</h1>", "/src/app.js": "1", "/empty.js": ""}
	if len(got) != len(expected) {
		t.Errorf("Read() has %d files, want %d: %v", len(got), len(expected), keys(got))
	}
	for path, content := range expected {
		if string(got[path]) != content {
			t.Errorf("Read()[%q] = %q, want %q", path, got[path], content)
		}
	}
}

func TestRead_MissingRoot(t *testing.T) {
	t.Parallel()
	_, err := Read(filepath.Join(t.TempDir(), "missing"))
	if !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("Read(missing) error = %v, want fs.ErrNotExist", err)
	}
}

func TestWrite(t *testing.T) {
	t.Parallel()
	root := filepath.Join(t.TempDir(), "work")
	if err := Write(root, "/src/deep/app.js", strings.NewReader("1")); err != nil {
		t.Fatal(err)
	}
	if err := Write(root, "/src/deep/app.js", strings.NewReader("2")); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(root, "src", "deep", "app.js"))
	if err != nil || string(data) != "2" {
		t.Fatalf("file = %q, %v; want \"2\"", data, err)
	}
	if _, err := os.Stat(filepath.Join(root, "src", "deep", "app.js"+tmpSuffix)); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("temporary file left behind: %v", err)
	}
}

func TestWrite_RefusesPathsOutsideRoot(t *testing.T) {
	t.Parallel()
	parent := t.TempDir()
	root := filepath.Join(parent, "work")
	for _, path := range []string{"/../escape.txt", "/a/../../escape.txt"} {
		err := Write(root, path, strings.NewReader("x"))
		if err == nil || !strings.Contains(err.Error(), "refusing to write") {
			t.Errorf("Write(%q) error = %v, want refusal", path, err)
		}
	}
	if _, err := os.Stat(filepath.Join(parent, "escape.txt")); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("escape.txt was written outside the root")
	}
}

func TestIsText(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		input    []byte
		expected bool
	}{
		{name: "ascii", input: []byte("hello"), expected: true},
		{name: "utf-8", input: []byte("grüße ✓"), expected: true},
		{name: "nul byte", input: []byte("a\x00b"), expected: false},
		{name: "invalid utf-8", input: []byte{0xff, 0xfe}, expected: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := IsText(tt.input); got != tt.expected {
				t.Errorf("IsText(%q) = %v, want %v", tt.input, got, tt.expected)
			}
		})
	}
}

func write(t *testing.T, root, path, content string) {
	t.Helper()
	file := filepath.Join(root, filepath.FromSlash(path))
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func keys(s Snapshot) []string {
	out := []string{}
	for k := range s {
		out = append(out, k)
	}
	return out
}
```

- [ ] **Step 2: Run it to verify it fails**

Run: `go test ./internal/tree/`
Expected: FAIL, `undefined: IsIgnored`.

- [ ] **Step 3: Implement** — `internal/tree/tree.go`

```go
// Package tree maps a local directory to and from a Snapshot of its files,
// keyed by "/"-rooted paths as the CodePen API uses them.
package tree

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

// Snapshot holds file contents by path, e.g. "/src/app.js".
type Snapshot map[string][]byte

// tmpSuffix marks a file that Write has not finished yet.
const tmpSuffix = ".cdpn-tmp"

var ignoredNames = map[string]bool{
	".git": true, "node_modules": true, ".env": true, ".envrc": true, ".DS_Store": true,
}

// IsIgnored reports whether path ("/a/b") is never mirrored: VCS and
// dependency folders, env files (secrets and local state) and unfinished writes.
func IsIgnored(path string) bool {
	if strings.HasSuffix(path, tmpSuffix) {
		return true
	}
	for _, part := range strings.Split(path, "/") {
		if ignoredNames[part] || strings.HasPrefix(part, ".env.") {
			return true
		}
	}
	return false
}

// Read returns every regular, non-ignored file below root. Symlinks are skipped.
func Read(root string) (Snapshot, error) {
	files := Snapshot{}
	err := filepath.WalkDir(root, func(file string, entry fs.DirEntry, err error) error {
		if err != nil || file == root {
			return err
		}
		path, err := penPath(root, file)
		if err != nil {
			return err
		}
		if IsIgnored(path) {
			return skip(entry)
		}
		if !entry.Type().IsRegular() {
			return nil
		}
		data, err := os.ReadFile(file)
		if err != nil {
			return err
		}
		files[path] = data
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("reading %q: %w", root, err)
	}
	return files, nil
}

func penPath(root, file string) (string, error) {
	rel, err := filepath.Rel(root, file)
	if err != nil {
		return "", err
	}
	return "/" + filepath.ToSlash(rel), nil
}

func skip(entry fs.DirEntry) error {
	if entry.IsDir() {
		return fs.SkipDir
	}
	return nil
}

// Write stores the contents of r at path below root, atomically: it writes a
// temporary file next to the target, then renames it into place. A path that
// would leave root (such as a remote "/../x") is refused, and all access goes
// through os.Root so symlinks cannot lead outside either.
func Write(root, path string, r io.Reader) error {
	name := filepath.FromSlash(strings.TrimPrefix(path, "/"))
	if !filepath.IsLocal(name) {
		return fmt.Errorf("refusing to write %q: it is outside %q", path, root)
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		return fmt.Errorf("creating %q: %w", root, err)
	}
	dir, err := os.OpenRoot(root)
	if err != nil {
		return fmt.Errorf("opening %q: %w", root, err)
	}
	defer dir.Close()
	if err := dir.MkdirAll(filepath.Dir(name), 0o755); err != nil {
		return fmt.Errorf("writing %q: %w", path, err)
	}
	if err := writeTemp(dir, name+tmpSuffix, r); err != nil {
		return fmt.Errorf("writing %q: %w", path, err)
	}
	if err := dir.Rename(name+tmpSuffix, name); err != nil {
		return fmt.Errorf("writing %q: %w", path, err)
	}
	return nil
}

func writeTemp(dir *os.Root, name string, r io.Reader) error {
	f, err := dir.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	if _, err := io.Copy(f, r); err != nil {
		return errors.Join(err, f.Close(), dir.Remove(name))
	}
	if err := f.Close(); err != nil {
		return errors.Join(err, dir.Remove(name))
	}
	return nil
}

// IsText reports whether data can go to the API as text: valid UTF-8 without NUL bytes.
func IsText(data []byte) bool {
	return bytes.IndexByte(data, 0) < 0 && utf8.Valid(data)
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/tree/`
Expected: `ok`.

- [ ] **Step 5: Commit**

```bash
git add internal/tree
git commit -m "feat(tree): read snapshots and write files atomically inside the root

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 6: `codepen` client core (types, errors, transport, retries)

**Files:**
- Create: `internal/codepen/types.go`, `internal/codepen/error.go`, `internal/codepen/client.go`, `internal/codepen/retry.go`
- Test: `internal/codepen/retry_test.go` (package `codepen`), `internal/codepen/client_test.go` (package `codepen_test`)

Reference: `lib/api.js`. Retry policy (spec): GET retried on network error, 429 and 5xx; POST/PUT only on 429; up to 4 retries; backoff `base·2ⁿ` (base 200ms); `Retry-After` seconds honoured, capped at 30s; ctx checked between attempts. Never put the API key in an error.

- [ ] **Step 1: Write the failing tests**

`internal/codepen/retry_test.go`:

```go
package codepen

import (
	"errors"
	"net/http"
	"testing"
)

func TestRetryable(t *testing.T) {
	t.Parallel()
	network := errors.New("connection reset")
	tests := []struct {
		name     string
		method   string
		status   int
		err      error
		expected bool
	}{
		{name: "get network error", method: http.MethodGet, err: network, expected: true},
		{name: "get 503", method: http.MethodGet, status: 503, expected: true},
		{name: "get 429", method: http.MethodGet, status: 429, expected: true},
		{name: "get 404", method: http.MethodGet, status: 404, expected: false},
		{name: "get 200", method: http.MethodGet, status: 200, expected: false},
		{name: "put 429", method: http.MethodPut, status: 429, expected: true},
		{name: "put 503 may have applied", method: http.MethodPut, status: 503, expected: false},
		{name: "post network error may have applied", method: http.MethodPost, err: network, expected: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := retryable(tt.method, tt.status, tt.err); got != tt.expected {
				t.Errorf("retryable(%s, %d, %v) = %v, want %v", tt.method, tt.status, tt.err, got, tt.expected)
			}
		})
	}
}
```

`internal/codepen/client_test.go`:

```go
package codepen_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nocksock/codepen-cli/internal/codepen"
)

func newClient(t *testing.T, handler http.HandlerFunc) *codepen.Client {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return codepen.New("secret-key",
		codepen.WithBaseURL(srv.URL),
		codepen.WithBackoff(time.Millisecond),
		codepen.WithPollInterval(time.Millisecond),
	)
}

func TestClient_Pen(t *testing.T) {
	t.Parallel()
	api := newClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer secret-key" {
			t.Errorf("Authorization = %q", r.Header.Get("Authorization"))
		}
		if r.URL.Path != "/pens/p1" {
			t.Errorf("path = %q", r.URL.Path)
		}
		w.Write([]byte(`{"id":"p1","title":"Demo","urls":{"editor":"https://e"},"saved_version":{"id":"v1"}}`))
	})
	pen, err := api.Pen(t.Context(), "p1")
	if err != nil {
		t.Fatal(err)
	}
	if pen.Title != "Demo" || pen.URLs.Editor != "https://e" || pen.SavedVersion.ID != "v1" {
		t.Errorf("Pen() = %+v", pen)
	}
}

func TestAPIError(t *testing.T) {
	t.Parallel()
	api := newClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusConflict)
		w.Write([]byte(`{"code":"CONFLICT","message":"the Pen's head moved"}`))
	})
	_, err := api.Pen(t.Context(), "p1")
	if !errors.Is(err, codepen.ErrConflict) {
		t.Errorf("errors.Is(err, ErrConflict) = false for %v", err)
	}
	if errors.Is(err, codepen.ErrNotFound) {
		t.Errorf("a conflict matched ErrNotFound")
	}
	if got := err.Error(); got != "409 CONFLICT: the Pen's head moved" {
		t.Errorf("Error() = %q", got)
	}
}

func TestAPIError_WithoutJSONBody(t *testing.T) {
	t.Parallel()
	api := newClient(t, func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "gateway", http.StatusBadGateway)
	})
	_, err := api.Pen(context.Background(), "p1")
	var apiErr *codepen.APIError
	if !errors.As(err, &apiErr) || apiErr.Status != http.StatusBadGateway {
		t.Errorf("err = %v, want APIError 502", err)
	}
}

func TestClient_Retries(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		statuses []int // replies in order; the last one repeats
		call     func(context.Context, *codepen.Client) error
		attempts int32
		isError  bool
	}{
		{name: "get retries 503", statuses: []int{503, 503, 200}, call: getPen, attempts: 3},
		{name: "get gives up after 4 retries", statuses: []int{503}, call: getPen, attempts: 5, isError: true},
		{name: "get does not retry 404", statuses: []int{404}, call: getPen, attempts: 1, isError: true},
		{name: "put retries 429", statuses: []int{429, 200}, call: putFiles, attempts: 2},
		{name: "put does not retry 503", statuses: []int{503}, call: putFiles, attempts: 1, isError: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var attempts atomic.Int32
			api := newClient(t, func(w http.ResponseWriter, _ *http.Request) {
				n := int(attempts.Add(1)) - 1
				status := tt.statuses[min(n, len(tt.statuses)-1)]
				w.WriteHeader(status)
				w.Write([]byte(`{"build":{"id":"b1"}}`))
			})
			err := tt.call(t.Context(), api)
			if (err != nil) != tt.isError {
				t.Errorf("err = %v, want error: %v", err, tt.isError)
			}
			if got := attempts.Load(); got != tt.attempts {
				t.Errorf("attempts = %d, want %d", got, tt.attempts)
			}
		})
	}
}

func TestClient_StopsRetryingWhenCancelled(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(t.Context())
	api := newClient(t, func(w http.ResponseWriter, _ *http.Request) {
		cancel()
		w.WriteHeader(http.StatusServiceUnavailable)
	})
	if _, err := api.Pen(ctx, "p1"); !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v, want context.Canceled", err)
	}
}

func getPen(ctx context.Context, api *codepen.Client) error {
	_, err := api.Pen(ctx, "p1")
	return err
}

func putFiles(ctx context.Context, api *codepen.Client) error {
	changes := codepen.ChangeSet{Writes: []codepen.Write{{Path: "/a.js", Content: "1"}}}
	_, err := api.WriteFiles(ctx, "p1", changes, "")
	return err
}
```

Note: `putFiles` needs `WriteFiles`, which Task 7 adds. In this task, give `client_test.go` only `newClient`, `TestClient_Pen`, `TestAPIError`, `TestAPIError_WithoutJSONBody`, `TestClient_StopsRetryingWhenCancelled`, `getPen`, and a `TestClient_Retries` without the two `put` rows and without `putFiles`. Task 7 adds those back.

- [ ] **Step 2: Run them to verify they fail**

Run: `go test ./internal/codepen/`
Expected: FAIL, `undefined: retryable` / `undefined: codepen.New`.

- [ ] **Step 3: Implement** — `internal/codepen/types.go`

```go
package codepen

import "time"

// Build statuses the client acts on.
const (
	BuildQueued   = "Queued"
	BuildRunning  = "Running"
	BuildCanceled = "Canceled"
	BuildSuccess  = "Success"
)

// Pen is a Pen's metadata (GET /pens/{id}).
type Pen struct {
	ID           string      `json:"id"`
	Title        string      `json:"title"`
	Description  string      `json:"description"`
	Tags         []string    `json:"tags"`
	Access       string      `json:"access"`
	Editor       string      `json:"editor"`
	Popularity   float64     `json:"popularity"`
	CreatedAt    time.Time   `json:"created_at"`
	UpdatedAt    time.Time   `json:"updated_at"`
	URLs         URLs        `json:"urls"`
	SavedVersion *Version    `json:"saved_version"`
	Deployment   *Deployment `json:"deployment"`
}

// URLs are where a Pen can be opened.
type URLs struct {
	Editor  string `json:"editor"`
	Preview string `json:"preview"`
}

// Deployment is a Pen's live site.
type Deployment struct {
	URL string `json:"url"`
}

// Version is one saved state of a Pen. Summaries leave out Files.
type Version struct {
	ID        string    `json:"id"`
	PenID     string    `json:"pen_id"`
	CreatedAt time.Time `json:"created_at"`
	Files     []File    `json:"files,omitempty"`
	Build     *Build    `json:"build,omitempty"`
}

// Build is the compiler run for a version.
type Build struct {
	ID             string       `json:"id"`
	VersionID      string       `json:"version_id"`
	Status         string       `json:"status"`
	Errors         []BuildError `json:"errors"`
	AppliedPatches []Patch      `json:"applied_patches"`
	NextID         string       `json:"next_id"`
}

// BuildError is a compiler error in one file.
type BuildError struct {
	Path    string `json:"path"`
	Message string `json:"message"`
	Range   *Range `json:"range,omitempty"`
}

// Range locates a BuildError.
type Range struct {
	Start Position `json:"start"`
}

// Position is a line in a file.
type Position struct {
	Line int `json:"line"`
}

// Patch is a file the compiler created or changed, e.g. .codepen/pen.config.json.
type Patch struct {
	Type string `json:"type"`
	Path string `json:"path"`
}

// File is one file of a version. Content is nil for binary files; their
// bytes are at URL and ContentHash is their sha256 in hex.
type File struct {
	ID          string  `json:"id"`
	Path        string  `json:"path"`
	Name        string  `json:"name"`
	IsBinary    bool    `json:"is_binary"`
	Content     *string `json:"content"`
	ContentHash string  `json:"content_hash"`
	URL         string  `json:"url"`
}

// Created is the answer to CreatePen: the new Pen and its first build.
type Created struct {
	Pen
	Build Build `json:"build"`
}

// ChangeSet is what a write sends: text files, deleted paths and binary uploads.
type ChangeSet struct {
	Writes  []Write
	Deletes []string
	Uploads []Upload
}

// Write sets a text file.
type Write struct {
	Path    string
	Content string
}

// Upload sets a binary file.
type Upload struct {
	Path string
	Data []byte
}

// IsEmpty reports whether cs changes nothing.
func (cs ChangeSet) IsEmpty() bool {
	return len(cs.Writes)+len(cs.Deletes)+len(cs.Uploads) == 0
}
```

`internal/codepen/error.go`:

```go
package codepen

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// APIError is an error answer from the API.
type APIError struct {
	Status  int    `json:"-"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

// Sentinels for errors.Is; they match any APIError with the same Code.
var (
	ErrConflict        = &APIError{Code: "CONFLICT"}
	ErrNotFound        = &APIError{Code: "NOT_FOUND"}
	ErrUnauthenticated = &APIError{Code: "UNAUTHENTICATED"}
)

func (e *APIError) Error() string {
	message := e.Message
	if message == "" {
		message = "request failed"
	}
	return fmt.Sprintf("%d %s: %s", e.Status, e.Code, message)
}

// Is matches sentinels by Code.
func (e *APIError) Is(target error) bool {
	t, ok := target.(*APIError)
	return ok && t.Code != "" && t.Code == e.Code
}

func decodeError(resp *http.Response) error {
	apiErr := &APIError{Status: resp.StatusCode}
	// A body that isn't JSON still leaves a useful error with the status alone.
	_ = json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(apiErr)
	return apiErr
}
```

`internal/codepen/client.go`:

```go
// Package codepen is a client for the CodePen REST API v2.
// https://blog.codepen.io/docs/api/api-v2/rest-api-v2/
package codepen

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// DefaultBaseURL is the production API.
const DefaultBaseURL = "https://api.codepen.io/v2"

const (
	defaultPollInterval = 2 * time.Second
	defaultBackoff      = 200 * time.Millisecond
	requestTimeout      = 30 * time.Second
)

// Client talks to the API with one key. It is safe for concurrent use.
type Client struct {
	key     string
	baseURL string
	poll    time.Duration
	backoff time.Duration
	http    *http.Client
}

// Option configures a Client.
type Option func(*Client)

// WithBaseURL points the client at another API, such as a fake in tests.
func WithBaseURL(url string) Option {
	return func(c *Client) { c.baseURL = strings.TrimSuffix(url, "/") }
}

// WithPollInterval sets how often Settle asks about a running build.
func WithPollInterval(d time.Duration) Option {
	return func(c *Client) { c.poll = d }
}

// WithBackoff sets the delay before the first retry; it doubles with each retry.
func WithBackoff(d time.Duration) Option {
	return func(c *Client) { c.backoff = d }
}

// WithHTTPClient replaces the HTTP client (default: 30s timeout).
func WithHTTPClient(h *http.Client) Option {
	return func(c *Client) { c.http = h }
}

// New returns a client that authenticates with key.
func New(key string, opts ...Option) *Client {
	c := &Client{
		key:     key,
		baseURL: DefaultBaseURL,
		poll:    defaultPollInterval,
		backoff: defaultBackoff,
		http:    &http.Client{Timeout: requestTimeout},
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// payload is an encoded request body, kept as bytes so retries can resend it.
type payload struct {
	contentType string
	data        []byte
}

// call sends one API request (with retries) and decodes the JSON answer into
// out unless out is nil. It returns the response headers.
func (c *Client) call(ctx context.Context, method, path string, body *payload, out any) (http.Header, error) {
	resp, err := c.send(ctx, method, path, body)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= http.StatusBadRequest {
		return nil, decodeError(resp)
	}
	if out == nil || resp.StatusCode == http.StatusNoContent {
		return resp.Header, nil
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return nil, fmt.Errorf("%s %s: decoding the answer: %w", method, path, err)
	}
	return resp.Header, nil
}

func (c *Client) send(ctx context.Context, method, path string, body *payload) (*http.Response, error) {
	for attempt := 0; ; attempt++ {
		resp, err := c.attempt(ctx, method, path, body)
		status := 0
		if resp != nil {
			status = resp.StatusCode
		}
		isLast := attempt == maxRetries || ctx.Err() != nil || !retryable(method, status, err)
		if isLast && err != nil {
			return nil, fmt.Errorf("%s %s: %w", method, path, err)
		}
		if isLast {
			return resp, nil
		}
		wait := c.delay(resp, attempt)
		if resp != nil {
			drain(resp)
		}
		if err := sleep(ctx, wait); err != nil {
			return nil, err
		}
	}
}

func (c *Client) attempt(ctx context.Context, method, path string, body *payload) (*http.Response, error) {
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body.data)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reader)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.key)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", body.contentType)
	}
	return c.http.Do(req)
}
```

`internal/codepen/retry.go`:

```go
package codepen

import (
	"context"
	"io"
	"net/http"
	"strconv"
	"time"
)

const (
	maxRetries    = 4
	maxRetryAfter = 30 * time.Second
)

// retryable reports whether a failed attempt may be repeated. Reads are retried
// on network errors, 429 and 5xx. Writes only on 429: after a 5xx or a dropped
// connection the write may already have been applied.
func retryable(method string, status int, err error) bool {
	if status == http.StatusTooManyRequests {
		return true
	}
	if method != http.MethodGet {
		return false
	}
	return err != nil || status >= http.StatusInternalServerError
}

// delay is how long to wait before retry number attempt+1.
func (c *Client) delay(resp *http.Response, attempt int) time.Duration {
	if resp != nil {
		if seconds, err := strconv.Atoi(resp.Header.Get("Retry-After")); err == nil && seconds >= 0 {
			return min(time.Duration(seconds)*time.Second, maxRetryAfter)
		}
	}
	return c.backoff << attempt
}

// sleep waits for d or until ctx is done.
func sleep(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// drain discards a little of an unwanted body so the connection can be reused.
func drain(resp *http.Response) {
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<16)) // best effort
	resp.Body.Close()
}
```

Also add `Pen` to a new `internal/codepen/endpoints.go` so `TestClient_Pen` compiles (Task 7 fills in the rest of the file):

```go
package codepen

import (
	"context"
	"net/http"
	"net/url"
)

// Pen returns a Pen's metadata.
func (c *Client) Pen(ctx context.Context, id string) (Pen, error) {
	var pen Pen
	_, err := c.call(ctx, http.MethodGet, "/pens/"+url.PathEscape(id), nil, &pen)
	return pen, err
}
```

- [ ] **Step 4: Run tests**

Run: `go test -race ./internal/codepen/`
Expected: `ok`.

- [ ] **Step 5: Commit**

```bash
git add internal/codepen
git commit -m "feat(codepen): add api client core with retries and typed errors

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 7: `codepen` endpoints, encoding and build settling

**Files:**
- Create: `internal/codepen/payload.go`
- Modify: `internal/codepen/endpoints.go`
- Test: `internal/codepen/client_test.go` (add tests; restore the two `put` rows and `putFiles` from Task 6)

Reference: `lib/api.js` (`body`, `settle`, `total`) and `lib/pen.js` (request shapes).

- [ ] **Step 1: Write the failing tests**

Restore the `put retries 429` / `put does not retry 503` rows and `putFiles` in `TestClient_Retries`, then append to `client_test.go` (add `encoding/json`, `io`, `strings` to its imports):

```go
func TestClient_WriteFiles_JSON(t *testing.T) {
	t.Parallel()
	api := newClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut || r.URL.Path != "/pens/p1/files" {
			t.Errorf("%s %s", r.Method, r.URL.Path)
		}
		if ct := r.Header.Get("Content-Type"); ct != "application/json" {
			t.Errorf("Content-Type = %q", ct)
		}
		body, _ := io.ReadAll(r.Body)
		expected := `{"files":[{"path":"/a.js","content":"1"},{"path":"/b.js","delete":true}],"expected_version_id":"v1"}`
		if string(body) != expected {
			t.Errorf("body = %s\nwant %s", body, expected)
		}
		w.Write([]byte(`{"build":{"id":"b1","status":"Queued"}}`))
	})
	changes := codepen.ChangeSet{Writes: []codepen.Write{{Path: "/a.js", Content: "1"}}, Deletes: []string{"/b.js"}}
	build, err := api.WriteFiles(t.Context(), "p1", changes, "v1")
	if err != nil || build.ID != "b1" {
		t.Errorf("WriteFiles() = %+v, %v", build, err)
	}
}

func TestClient_WriteFiles_Multipart(t *testing.T) {
	t.Parallel()
	api := newClient(t, func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Fatalf("not multipart: %v", err)
		}
		var changes struct {
			Files             []map[string]any `json:"files"`
			ExpectedVersionID string           `json:"expected_version_id"`
		}
		if err := json.Unmarshal([]byte(r.FormValue("changes")), &changes); err != nil {
			t.Errorf("changes part: %v", err)
		}
		if len(changes.Files) != 1 || changes.ExpectedVersionID != "v1" {
			t.Errorf("changes = %+v", changes)
		}
		if got := r.FormValue("expected_version_id"); got != "v1" {
			t.Errorf("expected_version_id part = %q", got)
		}
		file, header, err := r.FormFile("/img/dot.png")
		if err != nil {
			t.Fatalf("file part: %v", err)
		}
		data, _ := io.ReadAll(file)
		if string(data) != "\x89PNG" || header.Filename != "dot.png" {
			t.Errorf("file part = %q (%s)", data, header.Filename)
		}
		w.Write([]byte(`{"build":{"id":"b1"}}`))
	})
	changes := codepen.ChangeSet{
		Writes:  []codepen.Write{{Path: "/a.js", Content: "1"}},
		Uploads: []codepen.Upload{{Path: "/img/dot.png", Data: []byte("\x89PNG")}},
	}
	if _, err := api.WriteFiles(t.Context(), "p1", changes, "v1"); err != nil {
		t.Fatal(err)
	}
}

func TestClient_CreatePen(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		changes  codepen.ChangeSet
		expected string
	}{
		{name: "with files", changes: codepen.ChangeSet{Writes: []codepen.Write{{Path: "/a.js", Content: "1"}}}, expected: `{"title":"work","files":[{"path":"/a.js","content":"1"}]}`},
		{name: "empty means starter files", changes: codepen.ChangeSet{}, expected: `{"title":"work"}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			api := newClient(t, func(w http.ResponseWriter, r *http.Request) {
				body, _ := io.ReadAll(r.Body)
				if r.Method != http.MethodPost || r.URL.Path != "/pens" || string(body) != tt.expected {
					t.Errorf("%s %s %s\nwant %s", r.Method, r.URL.Path, body, tt.expected)
				}
				w.WriteHeader(http.StatusCreated)
				w.Write([]byte(`{"id":"p1","urls":{"editor":"https://e"},"build":{"id":"b1"}}`))
			})
			created, err := api.CreatePen(t.Context(), codepen.NewPen{Title: "work", Changes: tt.changes})
			if err != nil || created.ID != "p1" || created.Build.ID != "b1" || created.URLs.Editor != "https://e" {
				t.Errorf("CreatePen() = %+v, %v", created, err)
			}
		})
	}
}

func TestClient_Settle(t *testing.T) {
	t.Parallel()
	api := newClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/builds/b1":
			w.Write([]byte(`{"id":"b1","status":"Canceled","next_id":"b2"}`))
		case "/builds/b2":
			w.Write([]byte(`{"id":"b2","status":"Success","version_id":"v2"}`))
		default:
			t.Errorf("unexpected %s", r.URL.Path)
		}
	})
	build, err := api.Settle(t.Context(), codepen.Build{ID: "b1", Status: codepen.BuildQueued})
	if err != nil || build.ID != "b2" || build.Status != codepen.BuildSuccess {
		t.Errorf("Settle() = %+v, %v", build, err)
	}
}

func TestClient_Settle_DoneBuildMakesNoRequest(t *testing.T) {
	t.Parallel()
	api := newClient(t, func(_ http.ResponseWriter, r *http.Request) {
		t.Errorf("unexpected request %s", r.URL.Path)
	})
	if _, err := api.Settle(t.Context(), codepen.Build{ID: "b1", Status: codepen.BuildSuccess}); err != nil {
		t.Fatal(err)
	}
}

func TestClient_CountVersions(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		link     string
		expected int
	}{
		{name: "from the last page link", link: `<https://api/pens/p1/versions?page=7&per_page=1>; rel="last"`, expected: 7},
		{name: "without a link", link: "", expected: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			api := newClient(t, func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/pens/p1/versions" || r.URL.Query().Get("per_page") != "1" {
					t.Errorf("%s?%s", r.URL.Path, r.URL.RawQuery)
				}
				if tt.link != "" {
					w.Header().Set("Link", tt.link)
				}
				w.Write([]byte(`[{"id":"v1"}]`))
			})
			got, err := api.CountVersions(t.Context(), "p1")
			if err != nil || got != tt.expected {
				t.Errorf("CountVersions() = %d, %v; want %d", got, err, tt.expected)
			}
		})
	}
}

func TestClient_Download(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if auth := r.Header.Get("Authorization"); auth != "" {
			t.Errorf("download sent Authorization %q to the file host", auth)
		}
		w.Write([]byte("bytes"))
	}))
	t.Cleanup(srv.Close)
	api := codepen.New("secret-key")
	body, err := api.Download(t.Context(), srv.URL+"/raw/p1/f1")
	if err != nil {
		t.Fatal(err)
	}
	defer body.Close()
	data, _ := io.ReadAll(body)
	if string(data) != "bytes" {
		t.Errorf("Download() = %q", data)
	}
}

func TestClient_Download_Status(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.NotFoundHandler())
	t.Cleanup(srv.Close)
	_, err := codepen.New("k").Download(t.Context(), srv.URL+"/raw/x")
	if err == nil || !strings.Contains(err.Error(), "404") {
		t.Errorf("err = %v, want a 404 error", err)
	}
}
```

- [ ] **Step 2: Run them to verify they fail**

Run: `go test ./internal/codepen/`
Expected: FAIL, `api.WriteFiles undefined`.

- [ ] **Step 3: Implement** — `internal/codepen/payload.go`

```go
package codepen

import (
	"bytes"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"path"
)

// change is one entry of the API's "files" list.
type change struct {
	Path    string `json:"path"`
	Content string `json:"content,omitempty"`
	Delete  bool   `json:"delete,omitempty"`
}

// wire lists the text writes, then the deletes, as the API expects them.
func (cs ChangeSet) wire() []change {
	changes := make([]change, 0, len(cs.Writes)+len(cs.Deletes))
	for _, w := range cs.Writes {
		changes = append(changes, change{Path: w.Path, Content: w.Content})
	}
	for _, p := range cs.Deletes {
		changes = append(changes, change{Path: p, Delete: true})
	}
	return changes
}

// encode picks the request encoding: plain JSON when nothing is binary,
// otherwise multipart with the JSON in the part named field, expected (if set)
// in its own part, and one file part per upload, named by its path.
func encode(field string, v any, expected string, uploads []Upload) (*payload, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("encoding %s: %w", field, err)
	}
	if len(uploads) == 0 {
		return &payload{contentType: "application/json", data: data}, nil
	}
	var buf bytes.Buffer
	form := multipart.NewWriter(&buf)
	if err := form.WriteField(field, string(data)); err != nil {
		return nil, err
	}
	if expected != "" {
		if err := form.WriteField("expected_version_id", expected); err != nil {
			return nil, err
		}
	}
	for _, upload := range uploads {
		part, err := form.CreateFormFile(upload.Path, path.Base(upload.Path))
		if err != nil {
			return nil, err
		}
		if _, err := part.Write(upload.Data); err != nil {
			return nil, err
		}
	}
	if err := form.Close(); err != nil {
		return nil, err
	}
	return &payload{contentType: form.FormDataContentType(), data: buf.Bytes()}, nil
}
```

`internal/codepen/endpoints.go` (replace the file):

```go
package codepen

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
)

var lastLink = regexp.MustCompile(`<([^>]+)>;\s*rel="last"`)

// Pen returns a Pen's metadata.
func (c *Client) Pen(ctx context.Context, id string) (Pen, error) {
	var pen Pen
	_, err := c.call(ctx, http.MethodGet, "/pens/"+url.PathEscape(id), nil, &pen)
	return pen, err
}

// Files returns the files of a Pen's head.
func (c *Client) Files(ctx context.Context, id string) ([]File, error) {
	files := []File{}
	_, err := c.call(ctx, http.MethodGet, "/pens/"+url.PathEscape(id)+"/files", nil, &files)
	return files, err
}

// Version returns a version with its files.
func (c *Client) Version(ctx context.Context, id string) (Version, error) {
	var version Version
	_, err := c.call(ctx, http.MethodGet, "/versions/"+url.PathEscape(id), nil, &version)
	return version, err
}

// Build returns a build.
func (c *Client) Build(ctx context.Context, id string) (Build, error) {
	var build Build
	_, err := c.call(ctx, http.MethodGet, "/builds/"+url.PathEscape(id), nil, &build)
	return build, err
}

// CountVersions returns how many versions a Pen has, read from the rel="last"
// page link of a one-per-page listing.
func (c *Client) CountVersions(ctx context.Context, penID string) (int, error) {
	items := []json.RawMessage{}
	header, err := c.call(ctx, http.MethodGet, "/pens/"+url.PathEscape(penID)+"/versions?per_page=1", nil, &items)
	if err != nil {
		return 0, err
	}
	return total(header.Get("Link"), len(items)), nil
}

func total(link string, items int) int {
	match := lastLink.FindStringSubmatch(link)
	if match == nil {
		return items
	}
	last, err := url.Parse(match[1])
	if err != nil {
		return items
	}
	page, err := strconv.Atoi(last.Query().Get("page"))
	if err != nil {
		return items
	}
	return page
}

// NewPen is what CreatePen sends. Empty Changes give CodePen's starter files.
type NewPen struct {
	Title   string
	Changes ChangeSet
}

// CreatePen creates a Pen.
func (c *Client) CreatePen(ctx context.Context, pen NewPen) (Created, error) {
	body := struct {
		Title string   `json:"title"`
		Files []change `json:"files,omitempty"`
	}{Title: pen.Title, Files: pen.Changes.wire()}
	data, err := encode("pen", body, "", pen.Changes.Uploads)
	if err != nil {
		return Created{}, err
	}
	var created Created
	_, err = c.call(ctx, http.MethodPost, "/pens", data, &created)
	return created, err
}

// WriteFiles applies changes to a Pen as one new version and returns its build.
// With expected set, the API refuses (ErrConflict) if the head is another version.
func (c *Client) WriteFiles(ctx context.Context, id string, changes ChangeSet, expected string) (Build, error) {
	body := struct {
		Files    []change `json:"files"`
		Expected string   `json:"expected_version_id,omitempty"`
	}{Files: changes.wire(), Expected: expected}
	data, err := encode("changes", body, expected, changes.Uploads)
	if err != nil {
		return Build{}, err
	}
	var written struct {
		Build Build `json:"build"`
	}
	_, err = c.call(ctx, http.MethodPut, "/pens/"+url.PathEscape(id)+"/files", data, &written)
	return written.Build, err
}

// Download opens the content of a binary file. The caller closes it. The key
// is not sent: file URLs may point at another host.
func (c *Client) Download(ctx context.Context, fileURL string) (io.ReadCloser, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fileURL, nil)
	if err != nil {
		return nil, fmt.Errorf("downloading %q: %w", fileURL, err)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("downloading %q: %w", fileURL, err)
	}
	if resp.StatusCode >= http.StatusBadRequest {
		resp.Body.Close()
		return nil, fmt.Errorf("downloading %q: %s", fileURL, resp.Status)
	}
	return resp.Body, nil
}

// Settle follows a build until it is done, including through next_id when the
// API cancelled it to apply the compiler's patches.
func (c *Client) Settle(ctx context.Context, build Build) (Build, error) {
	for {
		var next string
		switch {
		case build.Status == BuildQueued || build.Status == BuildRunning:
			next = build.ID
		case build.Status == BuildCanceled && build.NextID != "":
			next = build.NextID
		default:
			return build, nil
		}
		if err := sleep(ctx, c.poll); err != nil {
			return build, err
		}
		var err error
		if build, err = c.Build(ctx, next); err != nil {
			return build, err
		}
	}
}
```

- [ ] **Step 4: Run tests**

Run: `go test -race ./internal/codepen/`
Expected: `ok`.

- [ ] **Step 5: Commit**

```bash
git add internal/codepen
git commit -m "feat(codepen): add endpoints, multipart encoding and build settling

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 8: `codepentest` fake API

**Files:**
- Create: `internal/codepen/codepentest/server.go`
- Test: `internal/codepen/codepentest/server_test.go`

Port of `test/support/fake-api.js`, checked through the real client so that the two agree.

- [ ] **Step 1: Write the failing test**

```go
package codepentest

import (
	"errors"
	"testing"
	"time"

	"github.com/nocksock/codepen-cli/internal/codepen"
)

func client(s *Server, key string) *codepen.Client {
	return codepen.New(key, codepen.WithBaseURL(s.URL()), codepen.WithPollInterval(time.Millisecond))
}

func TestServer_SeedAndRead(t *testing.T) {
	t.Parallel()
	s := Start(t)
	id := s.Seed(Seed{Title: "Demo", Files: map[string]string{"a.js": "1"}, Binaries: map[string][]byte{"dot.png": {0x89, 0}}})
	api := client(s, Key)

	pen, err := api.Pen(t.Context(), id)
	if err != nil || pen.Title != "Demo" || pen.Editor != "V2" || pen.SavedVersion == nil {
		t.Fatalf("Pen() = %+v, %v", pen, err)
	}
	files, err := api.Files(t.Context(), id)
	if err != nil || len(files) != 3 { // a.js, dot.png and the compiler's config patch
		t.Fatalf("Files() = %+v, %v", files, err)
	}
	if got := s.Files(id)["/a.js"]; got != "1" {
		t.Errorf("Files(id)[/a.js] = %q", got)
	}
	data, isBinary, ok := s.File(id, "/dot.png")
	if !ok || !isBinary || len(data) != 2 {
		t.Errorf("File(/dot.png) = %v, %v, %v", data, isBinary, ok)
	}
}

func TestServer_RejectsWrongKey(t *testing.T) {
	t.Parallel()
	s := Start(t)
	_, err := client(s, "cp_user_wrong").Pen(t.Context(), "x")
	if !errors.Is(err, codepen.ErrUnauthenticated) {
		t.Errorf("err = %v, want UNAUTHENTICATED", err)
	}
}

func TestServer_WriteConflict(t *testing.T) {
	t.Parallel()
	s := Start(t)
	id := s.Seed(Seed{Files: map[string]string{"a.js": "1"}})
	api := client(s, Key)
	pen, _ := api.Pen(t.Context(), id)
	s.Edit(id, map[string]string{"a.js": "remote"})

	changes := codepen.ChangeSet{Writes: []codepen.Write{{Path: "/a.js", Content: "local"}}}
	_, err := api.WriteFiles(t.Context(), id, changes, pen.SavedVersion.ID)
	if !errors.Is(err, codepen.ErrConflict) {
		t.Errorf("err = %v, want CONFLICT", err)
	}
	if got := s.Calls(`^PUT /v2/pens/` + id + `/files$`); got != 1 {
		t.Errorf("Calls(PUT) = %d, want 1", got)
	}
}

func TestServer_QueuedBuildSettles(t *testing.T) {
	t.Parallel()
	s := Start(t)
	id := s.Seed(Seed{Files: map[string]string{"a.js": "1"}})
	s.QueueBuilds(true)
	api := client(s, Key)
	changes := codepen.ChangeSet{Writes: []codepen.Write{{Path: "/a.js", Content: "2"}}}
	build, err := api.WriteFiles(t.Context(), id, changes, "")
	if err != nil || build.Status != codepen.BuildQueued {
		t.Fatalf("WriteFiles() = %+v, %v", build, err)
	}
	build, err = api.Settle(t.Context(), build)
	if err != nil || build.Status != codepen.BuildSuccess {
		t.Errorf("Settle() = %+v, %v", build, err)
	}
}

func TestServer_CreateWithoutFilesUsesStarter(t *testing.T) {
	t.Parallel()
	s := Start(t)
	created, err := client(s, Key).CreatePen(t.Context(), codepen.NewPen{Title: "work"})
	if err != nil {
		t.Fatal(err)
	}
	if got := s.Files(created.ID)["/index.html"]; got != "<h1>starter</h1>" {
		t.Errorf("starter index.html = %q", got)
	}
}
```

- [ ] **Step 2: Run it to verify it fails**

Run: `go test ./internal/codepen/codepentest/`
Expected: FAIL, `undefined: Start`.

- [ ] **Step 3: Implement** — `internal/codepen/codepentest/server.go`

```go
// Package codepentest is an in-memory stand-in for the CodePen REST API v2,
// just enough for cdpn's tests. It ports the prototype's test/support/fake-api.js.
package codepentest

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"math"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"path"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nocksock/codepen-cli/internal/codepen"
)

// Key is the only API key the server accepts.
const Key = "cp_user_test"

// configPath is the file the "compiler" adds on the first build.
const configPath = "/.codepen/pen.config.json"

// Server is a running fake API. Its methods are safe for concurrent use.
type Server struct {
	http        *httptest.Server
	mu          sync.Mutex
	pens        map[string]*pen
	versions    map[string]*version
	requests    []string // "METHOD /path"
	queueBuilds bool
}

type pen struct {
	id, title, access    string
	tags                 []string
	createdAt, updatedAt time.Time
	deployment           *codepen.Deployment
	versions             []*version
}

func (p *pen) head() *version {
	if len(p.versions) == 0 {
		return nil
	}
	return p.versions[len(p.versions)-1]
}

type version struct {
	id, penID string
	createdAt time.Time
	files     []*file // sorted by path
	build     *codepen.Build
}

type file struct {
	id, path string
	data     []byte
	isBinary bool
}

// Start runs a server until the test ends.
func Start(t testing.TB) *Server {
	s := &Server{pens: map[string]*pen{}, versions: map[string]*version{}}
	s.http = httptest.NewServer(s.routes())
	t.Cleanup(s.http.Close)
	return s
}

// URL is the API base URL, ending in /v2.
func (s *Server) URL() string { return s.http.URL + "/v2" }

// QueueBuilds makes writes answer with a Queued build; GET /builds then reports Success.
func (s *Server) QueueBuilds(on bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.queueBuilds = on
}

// Seed describes a Pen created directly, without going through the API.
type Seed struct {
	Title      string
	Tags       []string
	Deployment *codepen.Deployment
	Files      map[string]string // text files by path ("a.js" or "/a.js")
	Binaries   map[string][]byte
}

// Seed creates a Pen and returns its id.
func (s *Server) Seed(seed Seed) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	changes := []change{}
	for _, p := range slices.Sorted(maps.Keys(seed.Files)) {
		changes = append(changes, change{Path: p, Content: seed.Files[p]})
	}
	uploads := []upload{}
	for _, p := range slices.Sorted(maps.Keys(seed.Binaries)) {
		uploads = append(uploads, upload{path: p, data: seed.Binaries[p]})
	}
	req := newPenRequest{Title: seed.Title, Tags: seed.Tags, Deployment: seed.Deployment, Files: &changes}
	created, err := s.createPen(req, uploads)
	if err != nil {
		panic(fmt.Sprintf("codepentest: bad seed: %v", err)) // a bug in the test
	}
	return created.id
}

// Edit saves text files as if someone used the CodePen editor.
func (s *Server) Edit(id string, files map[string]string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	changes := []change{}
	for _, p := range slices.Sorted(maps.Keys(files)) {
		changes = append(changes, change{Path: p, Content: files[p]})
	}
	if _, err := s.write(s.pens[id], changes, nil); err != nil {
		panic(fmt.Sprintf("codepentest: bad edit: %v", err))
	}
}

// Files returns the head's files by path, binary ones as raw strings.
func (s *Server) Files(id string) map[string]string {
	s.mu.Lock()
	defer s.mu.Unlock()
	files := map[string]string{}
	for _, f := range s.pens[id].head().files {
		files[f.path] = string(f.data)
	}
	return files
}

// File returns one file of the head.
func (s *Server) File(id, filePath string) (data []byte, isBinary, ok bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, f := range s.pens[id].head().files {
		if f.path == filePath {
			return f.data, f.isBinary, true
		}
	}
	return nil, false, false
}

// URLs returns where the Pen can be opened.
func (s *Server) URLs(id string) codepen.URLs {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.penJSON(s.pens[id]).URLs
}

// Calls counts the requests whose "METHOD /path" matches pattern.
func (s *Server) Calls(pattern string) int {
	re := regexp.MustCompile(pattern)
	s.mu.Lock()
	defer s.mu.Unlock()
	n := 0
	for _, r := range s.requests {
		if re.MatchString(r) {
			n++
		}
	}
	return n
}

// apiFailure is an error answer.
type apiFailure struct {
	status        int
	code, message string
	extra         map[string]any
}

func (f *apiFailure) Error() string { return f.message }

func (f *apiFailure) body() map[string]any {
	body := map[string]any{"code": f.code, "message": f.message}
	maps.Copy(body, f.extra)
	return body
}

func notFound(what string) error {
	return &apiFailure{status: http.StatusNotFound, code: "NOT_FOUND", message: "no such " + what}
}

type handlerFunc func(w http.ResponseWriter, r *http.Request) (int, any, error)

func (s *Server) routes() http.Handler {
	api := http.NewServeMux()
	api.Handle("POST /v2/pens", s.handle(s.postPen))
	api.Handle("GET /v2/pens/{id}", s.handle(s.getPen))
	api.Handle("GET /v2/pens/{id}/files", s.handle(s.getFiles))
	api.Handle("PUT /v2/pens/{id}/files", s.handle(s.putFiles))
	api.Handle("GET /v2/pens/{id}/versions", s.handle(s.getVersions))
	api.Handle("GET /v2/versions/{id}", s.handle(s.getVersion))
	api.Handle("GET /v2/builds/{id}", s.handle(s.getBuild))

	mux := http.NewServeMux()
	mux.HandleFunc("GET /raw/{pen}/{file}", s.raw)
	mux.Handle("/v2/", s.authorized(api))
	return s.logged(mux)
}

func (s *Server) logged(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		s.requests = append(s.requests, r.Method+" "+r.URL.Path)
		s.mu.Unlock()
		next.ServeHTTP(w, r)
	})
}

func (s *Server) authorized(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+Key {
			failure := &apiFailure{status: http.StatusUnauthorized, code: "UNAUTHENTICATED", message: "invalid or missing API key"}
			writeJSON(w, failure.status, failure.body())
			return
		}
		next.ServeHTTP(w, r)
	})
}

// handle runs fn under the lock and writes its answer or failure as JSON.
func (s *Server) handle(fn handlerFunc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		status, body, err := fn(w, r)
		s.mu.Unlock()
		var failure *apiFailure
		switch {
		case errors.As(err, &failure):
			status, body = failure.status, failure.body()
		case err != nil:
			status, body = http.StatusInternalServerError, map[string]any{"message": err.Error()}
		}
		writeJSON(w, status, body)
	})
}

func (s *Server) raw(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p := s.pens[r.PathValue("pen")]
	if p == nil {
		http.NotFound(w, r)
		return
	}
	for _, f := range p.head().files {
		if f.id == r.PathValue("file") {
			w.Header().Set("Content-Type", "application/octet-stream")
			w.Write(f.data)
			return
		}
	}
	http.NotFound(w, r)
}

func (s *Server) pen(id string) (*pen, error) {
	p := s.pens[id]
	if p == nil {
		return nil, notFound("Pen")
	}
	return p, nil
}

func (s *Server) postPen(_ http.ResponseWriter, r *http.Request) (int, any, error) {
	var req newPenRequest
	uploads, _, err := parse(r, "pen", &req)
	if err != nil {
		return 0, nil, err
	}
	created, err := s.createPen(req, uploads)
	if err != nil {
		return 0, nil, err
	}
	return http.StatusCreated, codepen.Created{Pen: s.penJSON(created), Build: *created.head().build}, nil
}

func (s *Server) getPen(_ http.ResponseWriter, r *http.Request) (int, any, error) {
	p, err := s.pen(r.PathValue("id"))
	if err != nil {
		return 0, nil, err
	}
	return http.StatusOK, s.penJSON(p), nil
}

func (s *Server) getFiles(_ http.ResponseWriter, r *http.Request) (int, any, error) {
	p, err := s.pen(r.PathValue("id"))
	if err != nil {
		return 0, nil, err
	}
	return http.StatusOK, s.filesJSON(p.head()), nil
}

func (s *Server) putFiles(_ http.ResponseWriter, r *http.Request) (int, any, error) {
	p, err := s.pen(r.PathValue("id"))
	if err != nil {
		return 0, nil, err
	}
	var req writeRequest
	uploads, form, err := parse(r, "changes", &req)
	if err != nil {
		return 0, nil, err
	}
	if expected := form["expected_version_id"]; len(expected) > 0 {
		req.ExpectedVersionID = expected[0]
	}
	if req.ExpectedVersionID != "" && req.ExpectedVersionID != p.head().id {
		return 0, nil, &apiFailure{status: http.StatusConflict, code: "CONFLICT", message: "the Pen's head moved",
			extra: map[string]any{"expected_version_id": req.ExpectedVersionID, "current_version_id": p.head().id}}
	}
	v, err := s.write(p, req.Files, uploads)
	if err != nil {
		return 0, nil, err
	}
	return http.StatusOK, map[string]any{"pen": s.penJSON(p), "version": s.versionJSON(v), "build": *v.build}, nil
}

func (s *Server) getVersions(w http.ResponseWriter, r *http.Request) (int, any, error) {
	p, err := s.pen(r.PathValue("id"))
	if err != nil {
		return 0, nil, err
	}
	perPage := 30
	if n, err := strconv.Atoi(r.URL.Query().Get("per_page")); err == nil && n > 0 {
		perPage = n
	}
	newestFirst := slices.Clone(p.versions)
	slices.Reverse(newestFirst)
	last := max(1, int(math.Ceil(float64(len(newestFirst))/float64(perPage))))
	w.Header().Set("Link", fmt.Sprintf(`<%s%s?page=%d&per_page=%d>; rel="last"`, s.http.URL, r.URL.Path, last, perPage))
	summaries := []*codepen.Version{}
	for _, v := range newestFirst[:min(perPage, len(newestFirst))] {
		summaries = append(summaries, summary(v))
	}
	return http.StatusOK, summaries, nil
}

func (s *Server) getVersion(_ http.ResponseWriter, r *http.Request) (int, any, error) {
	v := s.versions[r.PathValue("id")]
	if v == nil {
		return 0, nil, notFound("version")
	}
	return http.StatusOK, s.versionJSON(v), nil
}

func (s *Server) getBuild(_ http.ResponseWriter, r *http.Request) (int, any, error) {
	v := s.versions[r.PathValue("id")]
	if v == nil {
		return 0, nil, notFound("build")
	}
	v.build.Status = codepen.BuildSuccess
	return http.StatusOK, *v.build, nil
}

type change struct {
	Path    string `json:"path"`
	Content string `json:"content"`
	Delete  bool   `json:"delete"`
}

type upload struct {
	path string
	data []byte
}

type newPenRequest struct {
	Title      string              `json:"title"`
	Tags       []string            `json:"tags"`
	Access     string              `json:"access"`
	Deployment *codepen.Deployment `json:"deployment"`
	Files      *[]change           `json:"files"` // nil: starter files
}

type writeRequest struct {
	Files             []change `json:"files"`
	ExpectedVersionID string   `json:"expected_version_id"`
}

func (s *Server) createPen(req newPenRequest, uploads []upload) (*pen, error) {
	now := time.Now().UTC()
	p := &pen{id: newID(), title: or(req.Title, "Untitled"), tags: req.Tags, access: or(req.Access, "Private"),
		createdAt: now, deployment: req.Deployment}
	if p.tags == nil {
		p.tags = []string{}
	}
	changes := []change{{Path: "/index.html", Content: "<h1>starter</h1>"}}
	switch {
	case req.Files != nil:
		changes = *req.Files
	case len(uploads) > 0:
		changes = []change{}
	}
	if _, err := s.write(p, changes, uploads); err != nil {
		return nil, err
	}
	s.pens[p.id] = p
	return p, nil
}

// write applies changes on top of the head as one new version, as PUT /files does.
func (s *Server) write(p *pen, changes []change, uploads []upload) (*version, error) {
	files := map[string]*file{}
	if head := p.head(); head != nil {
		for _, f := range head.files {
			files[f.path] = f
		}
	}
	for _, c := range changes {
		filePath := normalize(c.Path)
		switch {
		case c.Delete:
			for key := range files {
				if key == filePath || strings.HasPrefix(key, filePath+"/") {
					delete(files, key)
				}
			}
		case c.Content == "":
			return nil, &apiFailure{status: http.StatusBadRequest, code: "BAD_USER_INPUT", message: "empty content for " + filePath}
		default:
			files[filePath] = newFile(files[filePath], filePath, []byte(c.Content), false)
		}
	}
	for _, u := range uploads {
		filePath := normalize(u.path)
		files[filePath] = newFile(files[filePath], filePath, u.data, true)
	}
	patches := []codepen.Patch{}
	if _, ok := files[configPath]; !ok {
		files[configPath] = newFile(nil, configPath, []byte(`{"blocks":[]}`), false)
		patches = append(patches, codepen.Patch{Type: "create", Path: configPath})
	}
	id := newID()
	status := codepen.BuildSuccess
	if s.queueBuilds {
		status = codepen.BuildQueued
	}
	v := &version{id: id, penID: p.id, createdAt: time.Now().UTC(), files: sortedFiles(files),
		build: &codepen.Build{ID: id, VersionID: id, Status: status, Errors: []codepen.BuildError{}, AppliedPatches: patches}}
	p.versions = append(p.versions, v)
	p.updatedAt = v.createdAt
	s.versions[id] = v
	return v, nil
}

func newFile(previous *file, filePath string, data []byte, isBinary bool) *file {
	id := newID()
	if previous != nil {
		id = previous.id
	}
	return &file{id: id, path: filePath, data: data, isBinary: isBinary}
}

func sortedFiles(files map[string]*file) []*file {
	out := make([]*file, 0, len(files))
	for _, p := range slices.Sorted(maps.Keys(files)) {
		out = append(out, files[p])
	}
	return out
}

func (s *Server) penJSON(p *pen) codepen.Pen {
	head := p.head()
	hash := strings.ReplaceAll(head.id, "-", "")
	return codepen.Pen{
		ID: p.id, Title: p.title, Tags: p.tags, Access: p.access, Editor: "V2",
		CreatedAt: p.createdAt, UpdatedAt: p.updatedAt,
		URLs: codepen.URLs{
			Editor:  "https://codepen.io/editor/test/pen/" + p.id + "/" + hash,
			Preview: "https://" + hash + ".codepen.dev/",
		},
		SavedVersion: summary(head),
		Deployment:   p.deployment,
	}
}

func summary(v *version) *codepen.Version {
	build := *v.build
	return &codepen.Version{ID: v.id, PenID: v.penID, CreatedAt: v.createdAt, Build: &build}
}

func (s *Server) versionJSON(v *version) codepen.Version {
	full := *summary(v)
	full.Files = s.filesJSON(v)
	return full
}

func (s *Server) filesJSON(v *version) []codepen.File {
	files := make([]codepen.File, 0, len(v.files))
	for _, f := range v.files {
		sum := sha256.Sum256(f.data)
		out := codepen.File{ID: f.id, Path: f.path, Name: path.Base(f.path), IsBinary: f.isBinary,
			ContentHash: hex.EncodeToString(sum[:]), URL: s.http.URL + "/raw/" + v.penID + "/" + f.id}
		if !f.isBinary {
			content := string(f.data)
			out.Content = &content
		}
		files = append(files, out)
	}
	return files
}

// parse decodes a JSON or multipart body. For multipart, the JSON is in the
// part named field and every file part is an upload named by its path.
func parse(r *http.Request, field string, into any) ([]upload, map[string][]string, error) {
	if !strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/form-data") {
		data, err := io.ReadAll(r.Body)
		if err != nil || len(data) == 0 {
			return nil, nil, err
		}
		return nil, nil, json.Unmarshal(data, into)
	}
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		return nil, nil, err
	}
	if value := r.FormValue(field); value != "" {
		if err := json.Unmarshal([]byte(value), into); err != nil {
			return nil, nil, err
		}
	}
	uploads := []upload{}
	for _, name := range slices.Sorted(maps.Keys(r.MultipartForm.File)) {
		for _, header := range r.MultipartForm.File[name] {
			data, err := readPart(header)
			if err != nil {
				return nil, nil, err
			}
			uploads = append(uploads, upload{path: name, data: data})
		}
	}
	return uploads, r.MultipartForm.Value, nil
}

func readPart(header *multipart.FileHeader) ([]byte, error) {
	f, err := header.Open()
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return io.ReadAll(f)
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body) // a failed write shows up in the client as a bad body
}

func normalize(p string) string {
	if strings.HasPrefix(p, "/") {
		return p
	}
	return "/" + p
}

func or(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

// newID returns a random UUID v4.
func newID() string {
	var b [16]byte
	_, _ = rand.Read(b[:]) // crypto/rand.Read never fails (Go 1.24+)
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
```

- [ ] **Step 4: Run tests**

Run: `go test -race ./internal/codepen/...`
Expected: `ok` for both packages.

- [ ] **Step 5: Commit**

```bash
git add internal/codepen/codepentest
git commit -m "test(codepentest): port the fake codepen api to go

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 9: `diff`

**Files:**
- Create: `internal/diff/diff.go`
- Test: `internal/diff/diff_test.go`

Reference: `lib/diff.js`. Lists are sorted by path so output and requests are the same every time.

- [ ] **Step 1: Write the failing test**

```go
package diff

import (
	"crypto/sha256"
	"encoding/hex"
	"reflect"
	"testing"

	"github.com/nocksock/codepen-cli/internal/codepen"
	"github.com/nocksock/codepen-cli/internal/tree"
)

func text(p, content string) codepen.File {
	return codepen.File{Path: p, Content: &content}
}

func binary(p string, data []byte) codepen.File {
	sum := sha256.Sum256(data)
	return codepen.File{Path: p, IsBinary: true, ContentHash: hex.EncodeToString(sum[:])}
}

func TestIsSame(t *testing.T) {
	t.Parallel()
	png := []byte{0x89, 'P', 'N', 'G', 0}
	tests := []struct {
		name     string
		local    []byte
		remote   codepen.File
		expected bool
	}{
		{name: "equal text", local: []byte("1"), remote: text("/a.js", "1"), expected: true},
		{name: "changed text", local: []byte("2"), remote: text("/a.js", "1"), expected: false},
		{name: "equal binary by hash", local: png, remote: binary("/a.png", png), expected: true},
		{name: "changed binary", local: []byte{1}, remote: binary("/a.png", png), expected: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := IsSame(tt.local, tt.remote); got != tt.expected {
				t.Errorf("IsSame() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestBetween(t *testing.T) {
	t.Parallel()
	png := []byte{0x89, 'P', 'N', 'G', 0}
	local := tree.Snapshot{
		"/z.js":     []byte("new"),
		"/a.js":     []byte("changed"),
		"/same.js":  []byte("1"),
		"/empty.js": {},
		"/dot.png":  png,
	}
	remote := ByPath([]codepen.File{
		text("/a.js", "old"),
		text("/same.js", "1"),
		text("/gone.js", "x"),
		text("/also-gone.js", "x"),
	})
	expected := codepen.ChangeSet{
		Writes:  []codepen.Write{{Path: "/a.js", Content: "changed"}, {Path: "/z.js", Content: "new"}},
		Deletes: []string{"/also-gone.js", "/gone.js"},
		Uploads: []codepen.Upload{{Path: "/dot.png", Data: png}},
	}
	if got := Between(local, remote); !reflect.DeepEqual(got, expected) {
		t.Errorf("Between() =\n%+v\nwant\n%+v", got, expected)
	}
}

func TestBetween_Nothing(t *testing.T) {
	t.Parallel()
	got := Between(tree.Snapshot{"/a.js": []byte("1")}, ByPath([]codepen.File{text("/a.js", "1")}))
	if !got.IsEmpty() {
		t.Errorf("Between() = %+v, want empty", got)
	}
}
```

- [ ] **Step 2: Run it to verify it fails**

Run: `go test ./internal/diff/`
Expected: FAIL, `undefined: IsSame`.

- [ ] **Step 3: Implement** — `internal/diff/diff.go`

```go
// Package diff compares a local Snapshot with a Pen's files and turns the
// difference into the change set a push sends.
package diff

import (
	"crypto/sha256"
	"encoding/hex"
	"maps"
	"slices"

	"github.com/nocksock/codepen-cli/internal/codepen"
	"github.com/nocksock/codepen-cli/internal/tree"
)

// ByPath indexes files by their path.
func ByPath(files []codepen.File) map[string]codepen.File {
	byPath := make(map[string]codepen.File, len(files))
	for _, f := range files {
		byPath[f.Path] = f
	}
	return byPath
}

// IsSame reports whether local holds remote's content: text is compared as is,
// binary files by sha256.
func IsSame(local []byte, remote codepen.File) bool {
	if remote.Content != nil {
		return string(local) == *remote.Content
	}
	sum := sha256.Sum256(local)
	return hex.EncodeToString(sum[:]) == remote.ContentHash
}

// Between lists what a push must send so remote matches local: changed and new
// text files as Writes, binary ones as Uploads, and remote-only paths as
// Deletes. Empty local files are skipped, since the API refuses empty content.
func Between(local tree.Snapshot, remote map[string]codepen.File) codepen.ChangeSet {
	changes := codepen.ChangeSet{Writes: []codepen.Write{}, Deletes: []string{}, Uploads: []codepen.Upload{}}
	for _, p := range slices.Sorted(maps.Keys(local)) {
		data := local[p]
		file, exists := remote[p]
		if len(data) == 0 || (exists && IsSame(data, file)) {
			continue
		}
		if tree.IsText(data) {
			changes.Writes = append(changes.Writes, codepen.Write{Path: p, Content: string(data)})
		} else {
			changes.Uploads = append(changes.Uploads, codepen.Upload{Path: p, Data: data})
		}
	}
	for _, p := range slices.Sorted(maps.Keys(remote)) {
		if _, exists := local[p]; !exists {
			changes.Deletes = append(changes.Deletes, p)
		}
	}
	return changes
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/diff/`
Expected: `ok`.

- [ ] **Step 5: Commit**

```bash
git add internal/diff
git commit -m "feat(diff): compute push change sets

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 10: `config` and `secret`

**Files:**
- Create: `internal/config/config.go`, `internal/secret/secret.go`
- Test: `internal/config/config_test.go`, `internal/secret/secret_test.go`

Reference: `lib/env.js` (`load`, `get`, `secret`) and `pen.open()` in `lib/pen.js`. Precedence: process env > `.env.local` > `.env`; empty values count as unset.

- [ ] **Step 1: Write the failing tests**

`internal/config/config_test.go`:

```go
package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func env(vars map[string]string) func(string) (string, bool) {
	return func(name string) (string, bool) {
		v, ok := vars[name]
		return v, ok
	}
}

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestLoad_Precedence(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeFile(t, dir, SecretsFile, "CODEPEN_API_KEY=from-env-file\nCODEPEN_DIR=secrets-dir\nCODEPEN_API_URL=http://secrets\n")
	writeFile(t, dir, StateFile, "CODEPEN_DIR=pen\nCODEPEN_API_URL=http://state\n")
	cfg, err := Load(dir, env(map[string]string{"CODEPEN_API_URL": "http://process", "CODEPEN_API_KEY": ""}))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.KeyRef != "from-env-file" {
		t.Errorf("KeyRef = %q (an empty process value must not win)", cfg.KeyRef)
	}
	if cfg.Dir != filepath.Join(dir, "pen") {
		t.Errorf("Dir = %q, want state file's pen", cfg.Dir)
	}
	if cfg.APIURL != "http://process" {
		t.Errorf("APIURL = %q, want the process env", cfg.APIURL)
	}
}

func TestLoad_Defaults(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	cfg, err := Load(dir, env(nil))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Dir != dir || cfg.PenID != "" || cfg.PollInterval != 0 || cfg.APIURL != "" {
		t.Errorf("Load() = %+v", cfg)
	}
	if _, err := cfg.Key(); err == nil || !strings.Contains(err.Error(), "CODEPEN_API_KEY is not set") {
		t.Errorf("Key() error = %v", err)
	}
}

func TestLoad_Values(t *testing.T) {
	t.Parallel()
	cfg, err := Load(t.TempDir(), env(map[string]string{
		"CODEPEN_ID":      "https://codepen.io/me/pen/PwWmgeN",
		"CODEPEN_POLL_MS": "5",
		"CODEPEN_DIR":     "/abs/dir",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.PenID != "PwWmgeN" || cfg.PollInterval != 5*time.Millisecond || cfg.Dir != "/abs/dir" {
		t.Errorf("Load() = %+v", cfg)
	}
}

func TestLoad_Invalid(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		vars    map[string]string
		message string
	}{
		{name: "bad pen id", vars: map[string]string{"CODEPEN_ID": "not an id"}, message: "CODEPEN_ID"},
		{name: "bad poll interval", vars: map[string]string{"CODEPEN_POLL_MS": "soon"}, message: "CODEPEN_POLL_MS"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := Load(t.TempDir(), env(tt.vars))
			if err == nil || !strings.Contains(err.Error(), tt.message) {
				t.Errorf("Load() error = %v, want %s", err, tt.message)
			}
		})
	}
}
```

`internal/secret/secret_test.go`:

```go
package secret

import (
	"context"
	"errors"
	"strings"
	"testing"
)

type fakeRunner struct {
	out   string
	err   error
	calls []string
}

func (f *fakeRunner) Output(_ context.Context, name string, args ...string) ([]byte, error) {
	f.calls = append(f.calls, name+" "+strings.Join(args, " "))
	return []byte(f.out), f.err
}

func TestResolve(t *testing.T) {
	t.Parallel()
	run := &fakeRunner{out: "cp_user_real\n"}
	got, err := Resolve(t.Context(), "op://Private/CodePen/API Key", run)
	if err != nil || got != "cp_user_real" {
		t.Errorf("Resolve() = %q, %v", got, err)
	}
	if len(run.calls) != 1 || run.calls[0] != "op read op://Private/CodePen/API Key" {
		t.Errorf("calls = %q", run.calls)
	}
}

func TestResolve_PlainKey(t *testing.T) {
	t.Parallel()
	run := &fakeRunner{}
	got, err := Resolve(t.Context(), "cp_user_plain", run)
	if err != nil || got != "cp_user_plain" || len(run.calls) != 0 {
		t.Errorf("Resolve() = %q, %v, calls %q", got, err, run.calls)
	}
}

func TestResolve_Failure(t *testing.T) {
	t.Parallel()
	_, err := Resolve(t.Context(), "op://x/y/z", &fakeRunner{err: errors.New("not signed in")})
	if err == nil || !strings.Contains(err.Error(), "not signed in") {
		t.Errorf("Resolve() error = %v", err)
	}
}
```

- [ ] **Step 2: Run them to verify they fail**

Run: `go test ./internal/config/ ./internal/secret/`
Expected: FAIL, `undefined: Load` / `undefined: Resolve`.

- [ ] **Step 3: Implement** — `internal/config/config.go`

```go
// Package config gathers cdpn's settings from the process environment and the
// env files in the working directory.
package config

import (
	"fmt"
	"path/filepath"
	"strconv"
	"time"

	"github.com/nocksock/codepen-cli/internal/envfile"
	"github.com/nocksock/codepen-cli/internal/ref"
)

const (
	// SecretsFile holds the API key. cdpn only ever reads it: it may be a
	// 1Password Environments mount.
	SecretsFile = ".env"
	// StateFile holds local state such as CODEPEN_ID and wins over SecretsFile.
	StateFile = ".env.local"
)

// Config is cdpn's settings for one working directory.
type Config struct {
	KeyRef       string        // CODEPEN_API_KEY: a key or an op:// reference
	Dir          string        // CODEPEN_DIR as an absolute path (default: the working directory)
	PenID        string        // CODEPEN_ID, parsed; "" when no Pen is connected
	APIURL       string        // CODEPEN_API_URL; "" for the production API
	PollInterval time.Duration // CODEPEN_POLL_MS; 0 for the default
}

// Load reads the settings for the working directory dir (absolute). Values come
// from getenv first, then StateFile, then SecretsFile; empty values count as unset.
func Load(dir string, getenv func(string) (string, bool)) (Config, error) {
	lookup, err := sources(dir, getenv)
	if err != nil {
		return Config{}, err
	}
	cfg := Config{
		KeyRef: lookup("CODEPEN_API_KEY"),
		Dir:    resolve(dir, lookup("CODEPEN_DIR")),
		APIURL: lookup("CODEPEN_API_URL"),
	}
	if cfg.PenID, err = penID(lookup("CODEPEN_ID")); err != nil {
		return Config{}, err
	}
	if cfg.PollInterval, err = millis(lookup("CODEPEN_POLL_MS")); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

// Key returns the API key reference, or an error saying where to set it.
func (c Config) Key() (string, error) {
	if c.KeyRef == "" {
		return "", fmt.Errorf("CODEPEN_API_KEY is not set (check %s / %s)", StateFile, SecretsFile)
	}
	return c.KeyRef, nil
}

func sources(dir string, getenv func(string) (string, bool)) (func(string) string, error) {
	state, err := envfile.Read(filepath.Join(dir, StateFile))
	if err != nil {
		return nil, err
	}
	secrets, err := envfile.Read(filepath.Join(dir, SecretsFile))
	if err != nil {
		return nil, err
	}
	return func(name string) string {
		if value, ok := getenv(name); ok && value != "" {
			return value
		}
		if value := state[name]; value != "" {
			return value
		}
		return secrets[name]
	}, nil
}

func resolve(dir, path string) string {
	if path == "" {
		return dir
	}
	if filepath.IsAbs(path) {
		return filepath.Clean(path)
	}
	return filepath.Join(dir, path)
}

func penID(value string) (string, error) {
	if value == "" {
		return "", nil
	}
	id, err := ref.Parse(value)
	if err != nil {
		return "", fmt.Errorf("CODEPEN_ID: %w", err)
	}
	return id, nil
}

func millis(value string) (time.Duration, error) {
	if value == "" {
		return 0, nil
	}
	n, err := strconv.Atoi(value)
	if err != nil || n <= 0 {
		return 0, fmt.Errorf("CODEPEN_POLL_MS: %q is not a positive number", value)
	}
	return time.Duration(n) * time.Millisecond, nil
}
```

`internal/secret/secret.go`:

```go
// Package secret resolves API keys that are 1Password references.
package secret

import (
	"context"
	"fmt"
	"strings"
)

// Runner runs a program and returns its standard output.
type Runner interface {
	Output(ctx context.Context, name string, args ...string) ([]byte, error)
}

// Resolve returns the secret value behind value: op:// references are read
// with `op read`, anything else (a plain key, or one already resolved by
// `op run`) is returned as is.
func Resolve(ctx context.Context, value string, run Runner) (string, error) {
	if !strings.HasPrefix(value, "op://") {
		return value, nil
	}
	out, err := run.Output(ctx, "op", "read", value)
	if err != nil {
		return "", fmt.Errorf("reading %q from 1Password: %w", value, err)
	}
	return strings.TrimSpace(string(out)), nil
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/config/ ./internal/secret/`
Expected: `ok` for both.

- [ ] **Step 5: Commit**

```bash
git add internal/config internal/secret
git commit -m "feat(config): load settings from env files and resolve op refs

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 11: `pen` Mirror

**Files:**
- Create: `internal/pen/event.go`, `internal/pen/mirror.go`
- Test: `internal/pen/mirror_test.go`

Reference: `lib/pen.js`. The Mirror never prints and never touches env files: progress goes to `Reporter`, a new Pen id goes to `Config.SaveID`. The CLI scenario tests (Tasks 14–16) cover most behaviour; these tests cover the core flows and path traversal.

- [ ] **Step 1: Add the dependency**

Run: `go get golang.org/x/sync@v0.20.0`
Expected: `go.mod` gains `golang.org/x/sync v0.20.0`.

- [ ] **Step 2: Write the failing test**

```go
package pen

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nocksock/codepen-cli/internal/codepen"
	"github.com/nocksock/codepen-cli/internal/codepen/codepentest"
)

type recorder struct {
	mu     sync.Mutex
	events []Event
	saved  []string
}

func (r *recorder) Report(e Event) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, e)
}

func (r *recorder) saveID(id string) error {
	r.saved = append(r.saved, id)
	return nil
}

func open(t *testing.T, api *codepentest.Server, root, id string) (*Mirror, *recorder) {
	t.Helper()
	rec := &recorder{}
	client := codepen.New(codepentest.Key, codepen.WithBaseURL(api.URL()), codepen.WithPollInterval(time.Millisecond))
	return Open(Config{API: client, Root: root, ID: id, SaveID: rec.saveID, Reporter: rec}), rec
}

func writeFile(t *testing.T, root, path, content string) {
	t.Helper()
	file := filepath.Join(root, filepath.FromSlash(path))
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestMirror_Push_CreatesPen(t *testing.T) {
	t.Parallel()
	api := codepentest.Start(t)
	root := t.TempDir()
	writeFile(t, root, "index.html", "<h1>x</h1>")
	m, rec := open(t, api, root, "")

	if err := m.Push(t.Context(), PushOptions{}); err != nil {
		t.Fatal(err)
	}
	if len(rec.saved) != 1 || m.ID() != rec.saved[0] {
		t.Fatalf("saved ids = %q, mirror id %q", rec.saved, m.ID())
	}
	if got := api.Files(m.ID())["/index.html"]; got != "<h1>x</h1>" {
		t.Errorf("remote index.html = %q", got)
	}
	if _, ok := rec.events[0].(Created); !ok {
		t.Errorf("first event = %#v, want Created", rec.events[0])
	}
	if _, err := os.Stat(filepath.Join(root, ".codepen", "pen.config.json")); err != nil {
		t.Errorf("compiler patch not written back: %v", err)
	}
}

func TestMirror_Push_NothingChanged(t *testing.T) {
	t.Parallel()
	api := codepentest.Start(t)
	id := api.Seed(codepentest.Seed{Files: map[string]string{"a.js": "1"}})
	root := t.TempDir()
	m, _ := open(t, api, root, "")
	if err := m.Pull(t.Context(), PullOptions{Ref: id}); err != nil {
		t.Fatal(err)
	}
	if err := m.Push(t.Context(), PushOptions{}); err != nil {
		t.Fatal(err)
	}
	if got := api.Calls(`^PUT `); got != 0 {
		t.Errorf("PUT calls = %d, want 0", got)
	}
}

func TestMirror_Push_RefusesEmptyDirectory(t *testing.T) {
	t.Parallel()
	api := codepentest.Start(t)
	id := api.Seed(codepentest.Seed{Files: map[string]string{"a.js": "1"}})
	m, _ := open(t, api, t.TempDir(), id)
	err := m.Push(t.Context(), PushOptions{})
	if err == nil || !strings.Contains(err.Error(), "refusing to push") {
		t.Errorf("Push() error = %v", err)
	}
}

func TestMirror_Pull_SwitchingNeedsForce(t *testing.T) {
	t.Parallel()
	api := codepentest.Start(t)
	first := api.Seed(codepentest.Seed{Files: map[string]string{"a.js": "1"}})
	second := api.Seed(codepentest.Seed{Files: map[string]string{"b.js": "2"}})
	m, rec := open(t, api, t.TempDir(), first)

	err := m.Pull(t.Context(), PullOptions{Ref: second})
	if err == nil || !strings.Contains(err.Error(), "pass --force") {
		t.Fatalf("Pull() error = %v", err)
	}
	if err := m.Pull(t.Context(), PullOptions{Ref: second, Force: true}); err != nil {
		t.Fatal(err)
	}
	if m.ID() != second || len(rec.saved) != 1 || rec.saved[0] != second {
		t.Errorf("id = %q, saved = %q", m.ID(), rec.saved)
	}
}

func TestMirror_Pull_RestoresIDWhenFetchFails(t *testing.T) {
	t.Parallel()
	api := codepentest.Start(t)
	id := api.Seed(codepentest.Seed{Files: map[string]string{"a.js": "1"}})
	m, _ := open(t, api, t.TempDir(), id)
	err := m.Pull(t.Context(), PullOptions{Ref: "01a120bd-0000-7000-8000-000000000000", Force: true})
	if !errors.Is(err, codepen.ErrNotFound) {
		t.Fatalf("Pull() error = %v, want not found", err)
	}
	if m.ID() != id {
		t.Errorf("id = %q after a failed pull, want %q", m.ID(), id)
	}
}

func TestMirror_Pull_RefusesPathsOutsideRoot(t *testing.T) {
	t.Parallel()
	api := codepentest.Start(t)
	id := api.Seed(codepentest.Seed{Files: map[string]string{"../escape.txt": "x"}})
	parent := t.TempDir()
	m, _ := open(t, api, filepath.Join(parent, "work"), "")

	err := m.Pull(t.Context(), PullOptions{Ref: id})
	if err == nil || !strings.Contains(err.Error(), "refusing to write") {
		t.Errorf("Pull() error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(parent, "escape.txt")); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("a remote path escaped the directory")
	}
}
```

- [ ] **Step 3: Run it to verify it fails**

Run: `go test ./internal/pen/`
Expected: FAIL, `undefined: Open`.

- [ ] **Step 4: Implement** — `internal/pen/event.go`

```go
package pen

import "github.com/nocksock/codepen-cli/internal/codepen"

// Reporter receives a Mirror's progress.
type Reporter interface {
	Report(Event)
}

// Event is one step of a push or pull: Planned, Built, Downloaded, Created or Connected.
type Event interface {
	event()
}

// Planned lists what a push is about to send.
type Planned struct{ Changes codepen.ChangeSet }

// Built is a settled build, with its patches and errors.
type Built struct{ Build codepen.Build }

// Downloaded is a remote file written locally.
type Downloaded struct{ Path string }

// Created is a Pen made by the first push.
type Created struct{ URLs codepen.URLs }

// Connected is a directory that now mirrors another Pen.
type Connected struct{ URLs codepen.URLs }

func (Planned) event()    {}
func (Built) event()      {}
func (Downloaded) event() {}
func (Created) event()    {}
func (Connected) event()  {}
```

`internal/pen/mirror.go`:

```go
// Package pen mirrors a local directory to a CodePen Pen: push sends local
// changes up, pull brings remote files down.
package pen

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"path/filepath"
	"slices"
	"strings"

	"golang.org/x/sync/errgroup"

	"github.com/nocksock/codepen-cli/internal/codepen"
	"github.com/nocksock/codepen-cli/internal/diff"
	"github.com/nocksock/codepen-cli/internal/ref"
	"github.com/nocksock/codepen-cli/internal/tree"
)

// Config is what a Mirror needs.
type Config struct {
	API      *codepen.Client
	Root     string                // absolute directory to mirror
	ID       string                // connected Pen; "" when none
	SaveID   func(id string) error // records a newly connected Pen
	Reporter Reporter
}

// Mirror is a local directory mirrored to a Pen. Between calls it remembers the
// remote head, so repeated pushes only talk to the API when something changed.
// It is not safe for concurrent use.
type Mirror struct {
	cfg        Config
	id         string
	head       string // version id of the tracked head
	urls       codepen.URLs
	remote     map[string]codepen.File
	isTracking bool
}

// Open returns a Mirror for cfg. It makes no requests.
func Open(cfg Config) *Mirror {
	return &Mirror{cfg: cfg, id: cfg.ID, remote: map[string]codepen.File{}}
}

// ID is the connected Pen, "" when there is none.
func (m *Mirror) ID() string { return m.id }

// URLs are the connected Pen's URLs, once known.
func (m *Mirror) URLs() codepen.URLs { return m.urls }

// PushOptions change how Push behaves.
type PushOptions struct {
	Force bool // skip the version check and overwrite remote edits
}

// Push makes the Pen match the directory: changed and new files go up, files
// missing locally are deleted remotely. Without a connected Pen it creates one.
func (m *Mirror) Push(ctx context.Context, opts PushOptions) error {
	if m.id == "" {
		return m.create(ctx)
	}
	if !m.isTracking {
		if err := m.fetch(ctx); err != nil {
			return err
		}
	}
	local, err := tree.Read(m.cfg.Root)
	if err != nil {
		return err
	}
	if len(local) == 0 {
		return fmt.Errorf("refusing to push: %q is empty (would delete every file)", m.cfg.Root)
	}
	changes := diff.Between(local, m.remote)
	if changes.IsEmpty() {
		return nil
	}
	m.cfg.Reporter.Report(Planned{Changes: changes})
	expected := m.head
	if opts.Force {
		expected = ""
	}
	build, err := m.cfg.API.WriteFiles(ctx, m.id, changes, expected)
	if err != nil {
		return err
	}
	if err := m.settle(ctx, build); err != nil {
		return err
	}
	return m.writeBack(ctx, local)
}

// create makes a Pen from the directory; an empty one gets CodePen's starter files.
func (m *Mirror) create(ctx context.Context) error {
	local, err := tree.Read(m.cfg.Root)
	if err != nil {
		return err
	}
	created, err := m.cfg.API.CreatePen(ctx, codepen.NewPen{
		Title:   filepath.Base(m.cfg.Root),
		Changes: diff.Between(local, nil),
	})
	if err != nil {
		return err
	}
	m.id, m.urls = created.ID, created.URLs
	if err := m.cfg.SaveID(created.ID); err != nil {
		return fmt.Errorf("saving the Pen id: %w", err)
	}
	m.cfg.Reporter.Report(Created{URLs: created.URLs})
	if err := m.settle(ctx, created.Build); err != nil {
		return err
	}
	return m.writeBack(ctx, local)
}

// PullOptions change how Pull behaves.
type PullOptions struct {
	Ref   string // Pen id or URL to connect to; "" for the connected Pen
	Force bool   // allow switching away from another connected Pen
}

// Pull writes the Pen's files that differ locally. It never deletes local files.
func (m *Mirror) Pull(ctx context.Context, opts PullOptions) error {
	id, err := m.target(opts.Ref)
	if err != nil {
		return err
	}
	if m.id != "" && m.id != id && !opts.Force {
		return fmt.Errorf("this directory is connected to %s; pass --force to switch to %s", m.id, id)
	}
	previous := m.id
	m.id = id
	if err := m.fetch(ctx); err != nil {
		m.id = previous
		return err
	}
	if previous != id {
		if err := m.cfg.SaveID(id); err != nil {
			return fmt.Errorf("saving the Pen id: %w", err)
		}
		m.cfg.Reporter.Report(Connected{URLs: m.urls})
	}
	local, err := readOrEmpty(m.cfg.Root)
	if err != nil {
		return err
	}
	return m.writeBack(ctx, local)
}

func (m *Mirror) target(penRef string) (string, error) {
	if penRef != "" {
		return ref.Parse(penRef)
	}
	if m.id != "" {
		return m.id, nil
	}
	return "", errors.New("no Pen to pull: pass a Pen id or URL, or set CODEPEN_ID")
}

// fetch loads the Pen's metadata and head files.
func (m *Mirror) fetch(ctx context.Context) error {
	var (
		meta  codepen.Pen
		files []codepen.File
	)
	g, gctx := errgroup.WithContext(ctx)
	g.Go(func() (err error) {
		meta, err = m.cfg.API.Pen(gctx, m.id)
		return err
	})
	g.Go(func() (err error) {
		files, err = m.cfg.API.Files(gctx, m.id)
		return err
	})
	if err := g.Wait(); err != nil {
		return err
	}
	m.urls = meta.URLs
	head := ""
	if meta.SavedVersion != nil {
		head = meta.SavedVersion.ID
	}
	m.track(head, files)
	return nil
}

// settle waits for a build and tracks the version it produced. That version may
// differ from what was sent (compiler patches), so it is the source of truth.
func (m *Mirror) settle(ctx context.Context, build codepen.Build) error {
	build, err := m.cfg.API.Settle(ctx, build)
	if err != nil {
		return err
	}
	m.cfg.Reporter.Report(Built{Build: build})
	version, err := m.cfg.API.Version(ctx, build.VersionID)
	if err != nil {
		return err
	}
	m.track(version.ID, version.Files)
	return nil
}

func (m *Mirror) track(head string, files []codepen.File) {
	m.head = head
	m.remote = diff.ByPath(files)
	m.isTracking = true
}

// writeBack writes every tracked remote file that local lacks or differs from.
func (m *Mirror) writeBack(ctx context.Context, local tree.Snapshot) error {
	for _, p := range slices.Sorted(maps.Keys(m.remote)) {
		file := m.remote[p]
		if data, ok := local[p]; ok && diff.IsSame(data, file) {
			continue
		}
		if err := m.download(ctx, file); err != nil {
			return err
		}
		m.cfg.Reporter.Report(Downloaded{Path: p})
	}
	return nil
}

func (m *Mirror) download(ctx context.Context, file codepen.File) error {
	if file.Content != nil {
		return tree.Write(m.cfg.Root, file.Path, strings.NewReader(*file.Content))
	}
	body, err := m.cfg.API.Download(ctx, file.URL)
	if err != nil {
		return err
	}
	defer body.Close()
	return tree.Write(m.cfg.Root, file.Path, body)
}

// readOrEmpty reads root; a directory that doesn't exist yet is empty.
func readOrEmpty(root string) (tree.Snapshot, error) {
	local, err := tree.Read(root)
	if errors.Is(err, fs.ErrNotExist) {
		return tree.Snapshot{}, nil
	}
	return local, err
}
```

- [ ] **Step 5: Run tests**

Run: `go test -race ./internal/pen/`
Expected: `ok`.

- [ ] **Step 6: Commit**

```bash
git add go.mod go.sum internal/pen
git commit -m "feat(pen): mirror a directory to a pen with push and pull

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 12: `setup`, `browser` and `info`

**Files:**
- Create: `internal/setup/setup.go`, `internal/browser/browser.go`, `internal/info/info.go`
- Test: `internal/setup/setup_test.go`, `internal/browser/browser_test.go`, `internal/info/info_test.go`

Reference: `lib/init.js`, `lib/browser.js`, `lib/info.js`.

- [ ] **Step 1: Write the failing tests**

`internal/setup/setup_test.go`:

```go
package setup

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/nocksock/codepen-cli/internal/dotfile"
)

func TestScaffold(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	results, err := Scaffold(dir, DefaultKey)
	if err != nil {
		t.Fatal(err)
	}
	expected := []Result{{File: ".env", Status: dotfile.Created}, {File: ".envrc", Status: dotfile.Created}, {File: ".gitignore", Status: dotfile.Created}}
	if len(results) != len(expected) {
		t.Fatalf("results = %+v", results)
	}
	for i := range expected {
		if results[i] != expected[i] {
			t.Errorf("results[%d] = %+v, want %+v", i, results[i], expected[i])
		}
	}
	files := map[string]string{
		".env":       "CODEPEN_API_KEY=\"op://Private/CodePen/API Key\"\n",
		".envrc":     "dotenv_if_exists .env\ndotenv_if_exists .env.local\n",
		".gitignore": ".env\n.env.local\n",
	}
	for name, content := range files {
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil || string(data) != content {
			t.Errorf("%s = %q, %v; want %q", name, data, err, content)
		}
	}
}

type runner struct{ err error }

func (r runner) Output(context.Context, string, ...string) ([]byte, error) { return nil, r.err }

func TestAllowDirenv(t *testing.T) {
	t.Parallel()
	if !AllowDirenv(t.Context(), runner{}) {
		t.Error("AllowDirenv() = false when direnv succeeds")
	}
	if AllowDirenv(t.Context(), runner{err: errors.New("not found")}) {
		t.Error("AllowDirenv() = true when direnv is missing")
	}
}
```

`internal/browser/browser_test.go`:

```go
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
		{goos: "darwin", name: "open", args: []string{"https://x"}},
		{goos: "windows", name: "cmd", args: []string{"/c", "start", "", "https://x"}},
		{goos: "linux", name: "xdg-open", args: []string{"https://x"}},
		{goos: "freebsd", name: "xdg-open", args: []string{"https://x"}},
	}
	for _, tt := range tests {
		t.Run(tt.goos, func(t *testing.T) {
			t.Parallel()
			name, args := Command(tt.goos, "https://x")
			if name != tt.name || !slices.Equal(args, tt.args) {
				t.Errorf("Command(%s) = %s %q", tt.goos, name, args)
			}
		})
	}
}
```

`internal/info/info_test.go`:

```go
package info

import (
	"strings"
	"testing"
	"time"

	"github.com/nocksock/codepen-cli/internal/codepen"
)

func TestRender(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 10, 24, 12, 0, 0, 0, time.UTC)
	content := "12345"
	status := Status{
		Pen: codepen.Pen{
			ID: "p1", Title: "Demo", Editor: "V2", Access: "Private", Tags: []string{"api", "demo"},
			CreatedAt: now.Add(-48 * time.Hour), UpdatedAt: now.Add(-time.Hour),
			URLs:         codepen.URLs{Editor: "https://e", Preview: "https://p"},
			SavedVersion: &codepen.Version{CreatedAt: now.Add(-time.Hour), Build: &codepen.Build{Status: "Success"}},
		},
		Files:    []codepen.File{{Path: "/a.js", Content: &content}, {Path: "/dot.png"}},
		Versions: 3,
		Changes:  codepen.ChangeSet{Writes: []codepen.Write{{Path: "/b.js"}}, Deletes: []string{"/c.js"}},
	}
	expected := strings.Join([]string{
		"title       Demo",
		"id          p1 (V2)",
		"access      Private",
		"tags        api, demo",
		"created     2 days ago",
		"updated     1 hour ago",
		"versions    3, head 1 hour ago",
		"build       Success",
		"files       2 files, 5 B text + 1 binary",
		"local       1 to upload, 1 to delete (cdpn push)",
		"deployed    no",
		"popularity  0",
		"editor      https://e",
		"preview     https://p",
	}, "\n")
	if got := Render(status, now); got != expected {
		t.Errorf("Render() =\n%s\nwant\n%s", got, expected)
	}
}

func TestRender_InSyncWithBuildErrors(t *testing.T) {
	t.Parallel()
	status := Status{Pen: codepen.Pen{
		SavedVersion: &codepen.Version{Build: &codepen.Build{Status: "Failure", Errors: []codepen.BuildError{{}, {}}}},
		Deployment:   &codepen.Deployment{URL: "https://live"},
	}}
	got := Render(status, time.Now())
	for _, line := range []string{"build       Failure, 2 errors", "local       in sync", "deployed    https://live"} {
		if !strings.Contains(got, line) {
			t.Errorf("Render() lacks %q:\n%s", line, got)
		}
	}
}
```

- [ ] **Step 2: Run them to verify they fail**

Run: `go test ./internal/setup/ ./internal/browser/ ./internal/info/`
Expected: FAIL, undefined `Scaffold`, `Command`, `Render`.

- [ ] **Step 3: Implement** — `internal/setup/setup.go`

```go
// Package setup prepares a directory for cdpn (`cdpn init`).
package setup

import (
	"context"
	"path/filepath"

	"github.com/nocksock/codepen-cli/internal/config"
	"github.com/nocksock/codepen-cli/internal/dotfile"
)

// DefaultKey is the API key a new .env points at: a 1Password reference.
const DefaultKey = "op://Private/CodePen/API Key"

// Result is what Scaffold did to one file.
type Result struct {
	File   string
	Status dotfile.Status
}

// Runner runs a program in the directory being set up.
type Runner interface {
	Output(ctx context.Context, name string, args ...string) ([]byte, error)
}

type step struct {
	file   string
	ensure func(path string) (dotfile.Status, error)
}

// Scaffold makes sure dir has a .env with an API key, an .envrc that loads
// both env files (state last, so it wins in direnv) and a .gitignore that keeps
// them out of git. Nothing that exists is overwritten.
func Scaffold(dir, key string) ([]Result, error) {
	steps := []step{
		{file: config.SecretsFile, ensure: func(p string) (dotfile.Status, error) {
			return dotfile.Ensure(p, "CODEPEN_API_KEY=\""+key+"\"\n", 0o600)
		}},
		{file: ".envrc", ensure: func(p string) (dotfile.Status, error) {
			return dotfile.EnsureLines(p, []string{"dotenv_if_exists " + config.SecretsFile, "dotenv_if_exists " + config.StateFile})
		}},
		{file: ".gitignore", ensure: func(p string) (dotfile.Status, error) {
			return dotfile.EnsureLines(p, []string{config.SecretsFile, config.StateFile})
		}},
	}
	results := make([]Result, 0, len(steps))
	for _, s := range steps {
		status, err := s.ensure(filepath.Join(dir, s.file))
		if err != nil {
			return results, err
		}
		results = append(results, Result{File: s.file, Status: status})
	}
	return results, nil
}

// AllowDirenv runs `direnv allow .` and reports whether direnv accepted the
// .envrc. A missing direnv just reports false.
func AllowDirenv(ctx context.Context, run Runner) bool {
	_, err := run.Output(ctx, "direnv", "allow", ".")
	return err == nil
}
```

`internal/browser/browser.go`:

```go
// Package browser knows how each OS opens a URL in the default browser.
package browser

// Command returns the program and arguments that open url on goos.
func Command(goos, url string) (string, []string) {
	switch goos {
	case "darwin":
		return "open", []string{url}
	case "windows":
		return "cmd", []string{"/c", "start", "", url}
	default:
		return "xdg-open", []string{url}
	}
}
```

`internal/info/info.go`:

```go
// Package info gathers and renders what `cdpn info` shows: the connected Pen's
// details and how the local directory compares.
package info

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/nocksock/codepen-cli/internal/codepen"
	"github.com/nocksock/codepen-cli/internal/diff"
	"github.com/nocksock/codepen-cli/internal/human"
	"github.com/nocksock/codepen-cli/internal/tree"
)

// Status is everything info shows.
type Status struct {
	Pen      codepen.Pen
	Files    []codepen.File
	Versions int
	Changes  codepen.ChangeSet // what a push would send
}

// Gather fetches the Pen, its files and version count, and compares the files
// with the directory root.
func Gather(ctx context.Context, api *codepen.Client, id, root string) (Status, error) {
	var s Status
	g, gctx := errgroup.WithContext(ctx)
	g.Go(func() (err error) {
		s.Pen, err = api.Pen(gctx, id)
		return err
	})
	g.Go(func() (err error) {
		s.Files, err = api.Files(gctx, id)
		return err
	})
	g.Go(func() (err error) {
		s.Versions, err = api.CountVersions(gctx, id)
		return err
	})
	if err := g.Wait(); err != nil {
		return Status{}, err
	}
	s.Changes = diff.Between(localFiles(root), diff.ByPath(s.Files))
	return s, nil
}

// localFiles snapshots root; an unreadable directory counts as empty, so info
// still shows the Pen.
func localFiles(root string) tree.Snapshot {
	local, err := tree.Read(root)
	if err != nil {
		return tree.Snapshot{}
	}
	return local
}

// Render formats s as aligned "label  value" lines, with times relative to now.
func Render(s Status, now time.Time) string {
	p := s.Pen
	return human.Table([]human.Row{
		{Label: "title", Value: p.Title},
		{Label: "id", Value: fmt.Sprintf("%s (%s)", p.ID, p.Editor)},
		{Label: "access", Value: p.Access},
		{Label: "tags", Value: strings.Join(p.Tags, ", ")},
		{Label: "created", Value: human.Ago(p.CreatedAt, now)},
		{Label: "updated", Value: human.Ago(p.UpdatedAt, now)},
		{Label: "versions", Value: versions(s.Versions, p.SavedVersion, now)},
		{Label: "build", Value: build(p.SavedVersion)},
		{Label: "files", Value: size(s.Files)},
		{Label: "local", Value: local(s.Changes)},
		{Label: "deployed", Value: deployed(p.Deployment)},
		{Label: "popularity", Value: strconv.FormatFloat(p.Popularity, 'f', -1, 64)},
		{Label: "editor", Value: p.URLs.Editor},
		{Label: "preview", Value: p.URLs.Preview},
	})
}

func versions(n int, head *codepen.Version, now time.Time) string {
	out := strconv.Itoa(n)
	if head != nil {
		out += ", head " + human.Ago(head.CreatedAt, now)
	}
	return out
}

func build(head *codepen.Version) string {
	if head == nil {
		return ""
	}
	if head.Build == nil {
		return "none"
	}
	out := head.Build.Status
	if n := len(head.Build.Errors); n > 0 {
		out += ", " + human.Plural(n, "error")
	}
	return out
}

func size(files []codepen.File) string {
	var text int64
	binaries := 0
	for _, f := range files {
		if f.Content == nil {
			binaries++
			continue
		}
		text += int64(len(*f.Content))
	}
	out := fmt.Sprintf("%s, %s text", human.Plural(len(files), "file"), human.Bytes(text))
	if binaries > 0 {
		out += fmt.Sprintf(" + %d binary", binaries)
	}
	return out
}

func local(changes codepen.ChangeSet) string {
	writes := len(changes.Writes) + len(changes.Uploads)
	deletes := len(changes.Deletes)
	if writes == 0 && deletes == 0 {
		return "in sync"
	}
	parts := []string{}
	if writes > 0 {
		parts = append(parts, fmt.Sprintf("%d to upload", writes))
	}
	if deletes > 0 {
		parts = append(parts, fmt.Sprintf("%d to delete", deletes))
	}
	return strings.Join(parts, ", ") + " (cdpn push)"
}

func deployed(d *codepen.Deployment) string {
	if d == nil {
		return "no"
	}
	return d.URL
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/setup/ ./internal/browser/ ./internal/info/`
Expected: `ok` for all three.

- [ ] **Step 5: Commit**

```bash
git add internal/setup internal/browser internal/info
git commit -m "feat: add init scaffolding, browser opener and info report

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 13: `watch`

**Files:**
- Create: `internal/watch/watch.go`
- Test: `internal/watch/watch_test.go`

Reference: `lib/watch.js` and the spike's `~/studies/cli-tool-writing/impl/go-fsnotify/watch.go`. fsnotify isn't recursive. Events only arm a debounce timer. A fingerprint rescan (paths, sizes, mtimes of files) catches what events miss. Watcher errors don't stop it, and its goroutine owns and closes the channel.

- [ ] **Step 1: Add the dependencies**

Run: `go get github.com/fsnotify/fsnotify@v1.10.1 go.uber.org/goleak@v1.3.0`

- [ ] **Step 2: Write the failing test**

```go
package watch

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

var fast = Options{Debounce: 30 * time.Millisecond, Rescan: time.Hour}

func start(t *testing.T, dir string, opts Options) <-chan struct{} {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	changes, err := Changes(ctx, dir, opts)
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cancel()
		for range changes { // wait until the watcher has shut down
		}
	})
	return changes
}

func expectSignal(t *testing.T, changes <-chan struct{}) {
	t.Helper()
	select {
	case _, ok := <-changes:
		if !ok {
			t.Fatal("channel closed")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("no change signalled")
	}
}

func expectQuiet(t *testing.T, changes <-chan struct{}, d time.Duration) {
	t.Helper()
	select {
	case <-changes:
		t.Fatal("unexpected change signalled")
	case <-time.After(d):
	}
}

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestChanges(t *testing.T) {
	t.Parallel()

	t.Run("signals a new file", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		changes := start(t, dir, fast)
		write(t, filepath.Join(dir, "a.js"), "1")
		expectSignal(t, changes)
	})

	t.Run("watches new subdirectories", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		changes := start(t, dir, fast)
		if err := os.Mkdir(filepath.Join(dir, "src"), 0o755); err != nil {
			t.Fatal(err)
		}
		expectSignal(t, changes)
		write(t, filepath.Join(dir, "src", "a.js"), "1")
		expectSignal(t, changes)
	})

	t.Run("ignores skipped paths", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		if err := os.Mkdir(filepath.Join(dir, "node_modules"), 0o755); err != nil {
			t.Fatal(err)
		}
		opts := fast
		opts.Skip = func(p string) bool { return strings.Contains(p, "node_modules") || p == "/.env.local" }
		changes := start(t, dir, opts)
		write(t, filepath.Join(dir, "node_modules", "dep.js"), "x")
		write(t, filepath.Join(dir, ".env.local"), "CODEPEN_ID=x")
		expectQuiet(t, changes, 300*time.Millisecond)
	})

	t.Run("collapses a burst into one signal", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		changes := start(t, dir, Options{Debounce: 100 * time.Millisecond, Rescan: time.Hour})
		for _, name := range []string{"a", "b", "c", "d", "e"} {
			write(t, filepath.Join(dir, name+".js"), name)
		}
		expectSignal(t, changes)
		expectQuiet(t, changes, 300*time.Millisecond)
	})

	t.Run("closes the channel when ctx is done", func(t *testing.T) {
		t.Parallel()
		ctx, cancel := context.WithCancel(context.Background())
		changes, err := Changes(ctx, t.TempDir(), fast)
		if err != nil {
			t.Fatal(err)
		}
		cancel()
		select {
		case _, ok := <-changes:
			if ok {
				t.Fatal("got a signal instead of a close")
			}
		case <-time.After(3 * time.Second):
			t.Fatal("channel not closed")
		}
	})

	t.Run("fails for a missing directory", func(t *testing.T) {
		t.Parallel()
		if _, err := Changes(t.Context(), filepath.Join(t.TempDir(), "missing"), fast); err == nil {
			t.Fatal("Changes(missing) succeeded")
		}
	})
}

func TestTracker_Fingerprint(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	write(t, filepath.Join(dir, "a.js"), "1")
	tr := &tracker{dir: dir, opts: Options{Skip: func(p string) bool { return p == "/ignored.js" }}.withDefaults()}
	before := tr.fingerprint()

	write(t, filepath.Join(dir, "ignored.js"), "x")
	if tr.fingerprint() != before {
		t.Error("a skipped file changed the fingerprint")
	}
	write(t, filepath.Join(dir, "a.js"), "22")
	if tr.fingerprint() == before {
		t.Error("a changed file left the fingerprint as it was")
	}
}
```

- [ ] **Step 3: Run it to verify it fails**

Run: `go test ./internal/watch/`
Expected: FAIL, `undefined: Changes`.

- [ ] **Step 4: Implement** — `internal/watch/watch.go`

```go
// Package watch tells when files below a directory change. Events are only a
// hint: they are debounced into one signal, and a periodic rescan of file
// sizes and mtimes catches anything the OS failed to report.
package watch

import (
	"context"
	"fmt"
	"hash/fnv"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/fsnotify/fsnotify"
)

// Options tune Changes.
type Options struct {
	Skip     func(path string) bool // paths ("/a/b", relative to the directory) to ignore
	Debounce time.Duration          // quiet time before a signal (default 400ms)
	Rescan   time.Duration          // fingerprint interval (default 5s)
}

func (o Options) withDefaults() Options {
	if o.Skip == nil {
		o.Skip = func(string) bool { return false }
	}
	if o.Debounce <= 0 {
		o.Debounce = 400 * time.Millisecond
	}
	if o.Rescan <= 0 {
		o.Rescan = 5 * time.Second
	}
	return o
}

// Changes watches dir and its subdirectories until ctx is done, then closes the
// returned channel. Each value means "something changed"; bursts collapse into
// one value, and one is kept while the receiver is busy.
func Changes(ctx context.Context, dir string, opts Options) (<-chan struct{}, error) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("starting the file watcher: %w", err)
	}
	t := &tracker{dir: dir, opts: opts.withDefaults(), watcher: watcher}
	if err := t.addTree(dir); err != nil {
		watcher.Close()
		return nil, err
	}
	t.last = t.fingerprint()
	out := make(chan struct{}, 1)
	go t.run(ctx, out)
	return out, nil
}

type tracker struct {
	dir     string
	opts    Options
	watcher *fsnotify.Watcher
	last    uint64 // fingerprint at the last signal
}

func (t *tracker) run(ctx context.Context, out chan<- struct{}) {
	defer close(out)
	defer t.watcher.Close()
	debounce := time.NewTimer(t.opts.Debounce)
	debounce.Stop()
	defer debounce.Stop()
	rescan := time.NewTicker(t.opts.Rescan)
	defer rescan.Stop()
	events, errs := t.watcher.Events, t.watcher.Errors
	for {
		select {
		case <-ctx.Done():
			return
		case event, ok := <-events:
			if !ok {
				events = nil // watcher gone: rescans carry on
				continue
			}
			if t.isRelevant(event) {
				debounce.Reset(t.opts.Debounce)
			}
		case _, ok := <-errs:
			// Overflow or a failed watch: events may be missing, rescans catch up.
			if !ok {
				errs = nil
			}
		case <-debounce.C:
			t.last = t.fingerprint()
			notify(out)
		case <-rescan.C:
			if fp := t.fingerprint(); fp != t.last {
				t.last = fp
				_ = t.addTree(t.dir) // best effort: picks up directories events missed
				notify(out)
			}
		}
	}
}

// isRelevant reports whether event should trigger a signal, and starts
// watching directories as they appear.
func (t *tracker) isRelevant(event fsnotify.Event) bool {
	if event.Op == fsnotify.Chmod || t.isSkipped(event.Name) {
		return false
	}
	if event.Has(fsnotify.Create) {
		if info, err := os.Stat(event.Name); err == nil && info.IsDir() {
			_ = t.addTree(event.Name) // a failure here is covered by the rescan
		}
	}
	return true
}

// notify sends a signal unless one is already waiting.
func notify(out chan<- struct{}) {
	select {
	case out <- struct{}{}:
	default:
	}
}

// addTree watches root and every directory below it that isn't skipped.
func (t *tracker) addTree(root string) error {
	return filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			if path == root {
				return fmt.Errorf("watching %q: %w", path, err)
			}
			return nil // vanished while walking; the rescan catches up
		}
		if !entry.IsDir() {
			return nil
		}
		if path != t.dir && t.isSkipped(path) {
			return fs.SkipDir
		}
		if err := t.watcher.Add(path); err != nil {
			return fmt.Errorf("watching %q: %w", path, err)
		}
		return nil
	})
}

func (t *tracker) isSkipped(file string) bool {
	rel, err := filepath.Rel(t.dir, file)
	if err != nil {
		return false
	}
	return t.opts.Skip("/" + filepath.ToSlash(rel))
}

// fingerprint hashes the paths, sizes and mtimes of all files that aren't skipped.
func (t *tracker) fingerprint() uint64 {
	h := fnv.New64a()
	// The callback never returns an error, so neither does WalkDir.
	_ = filepath.WalkDir(t.dir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil || path == t.dir {
			return nil
		}
		if t.isSkipped(path) {
			if entry.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return nil
		}
		fmt.Fprintf(h, "%s\x00%d\x00%d\n", path, info.Size(), info.ModTime().UnixNano())
		return nil
	})
	return h.Sum64()
}
```

- [ ] **Step 5: Run tests**

Run: `go test -race -count=3 ./internal/watch/`
Expected: `ok` all three runs (checks flakiness).

- [ ] **Step 6: Commit**

```bash
git add go.mod go.sum internal/watch
git commit -m "feat(watch): add recursive debounced watcher with rescan fallback

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 14: `cli` core with `push` and `pull`

**Files:**
- Create: `internal/cli/cli.go`, `app.go`, `runner.go`, `root.go`, `errors.go`, `render.go`, `push.go`, `pull.go`
- Test: `internal/cli/workspace_test.go` (helpers + `TestMain`), `root_test.go`, `push_test.go`, `pull_test.go`

Reference: `bin/cdpn` and the Node tests `test/cli-basics.test.js` and `test/cli-push-pull.test.js`. These Go tests port them 1:1. The intentional difference: usage errors exit **2** and print usage to **stderr**.

- [ ] **Step 1: Add the dependency**

Run: `go get github.com/spf13/cobra@v1.10.2`

- [ ] **Step 2: Write the test helpers** — `internal/cli/workspace_test.go`

```go
package cli

import (
	"bytes"
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"go.uber.org/goleak"

	"github.com/nocksock/codepen-cli/internal/codepen/codepentest"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m,
		// Idle keep-alive connections of the API client wind down on their own.
		goleak.IgnoreAnyFunction("net/http.(*persistConn).readLoop"),
		goleak.IgnoreAnyFunction("net/http.(*persistConn).writeLoop"),
	)
}

// workspace is a temporary working directory plus a fresh fake API.
type workspace struct {
	t      *testing.T
	dir    string
	api    *codepentest.Server
	vars   map[string]string
	runner *recorder
}

func newWorkspace(t *testing.T, files map[string]string) *workspace {
	t.Helper()
	api := codepentest.Start(t)
	w := &workspace{
		t:      t,
		dir:    t.TempDir(),
		api:    api,
		runner: &recorder{missing: map[string]bool{}},
		vars: map[string]string{
			"CODEPEN_API_KEY": codepentest.Key,
			"CODEPEN_API_URL": api.URL(),
			"CODEPEN_POLL_MS": "5",
		},
	}
	for p, content := range files {
		w.write(p, content)
	}
	return w
}

// connected returns a workspace connected to a fresh Pen via `cdpn pull`.
func connected(t *testing.T, seed codepentest.Seed) (*workspace, string) {
	t.Helper()
	w := newWorkspace(t, nil)
	id := w.api.Seed(seed)
	if r := w.run("pull", id); r.code != 0 {
		t.Fatalf("setup pull failed: %s", r.stderr)
	}
	return w, id
}

type result struct {
	code   int
	stdout string
	stderr string
}

func (w *workspace) env(stdout, stderr io.Writer) Env {
	return Env{
		Dir: w.dir,
		Getenv: func(name string) (string, bool) {
			v, ok := w.vars[name]
			return v, ok
		},
		Stdout: stdout,
		Stderr: stderr,
		Runner: w.runner,
		GOOS:   "linux",
	}
}

func (w *workspace) run(args ...string) result {
	w.t.Helper()
	var stdout, stderr bytes.Buffer
	code := Run(w.t.Context(), args, w.env(&stdout, &stderr))
	return result{code: code, stdout: stdout.String(), stderr: stderr.String()}
}

func (w *workspace) path(p string) string { return filepath.Join(w.dir, filepath.FromSlash(p)) }

func (w *workspace) write(p, content string) { w.writeBytes(p, []byte(content)) }

func (w *workspace) writeBytes(p string, data []byte) {
	w.t.Helper()
	if err := os.MkdirAll(filepath.Dir(w.path(p)), 0o755); err != nil {
		w.t.Fatal(err)
	}
	if err := os.WriteFile(w.path(p), data, 0o644); err != nil {
		w.t.Fatal(err)
	}
}

func (w *workspace) read(p string) string {
	w.t.Helper()
	data, err := os.ReadFile(w.path(p))
	if err != nil {
		w.t.Fatal(err)
	}
	return string(data)
}

func (w *workspace) remove(p string) {
	w.t.Helper()
	if err := os.RemoveAll(w.path(p)); err != nil {
		w.t.Fatal(err)
	}
}

func (w *workspace) exists(p string) bool {
	_, err := os.Stat(w.path(p))
	return err == nil
}

// recorder stands in for op, direnv and the browser, recording each call.
type recorder struct {
	mu      sync.Mutex
	calls   []string
	missing map[string]bool // programs that are "not installed"
}

func (r *recorder) Output(_ context.Context, name string, args ...string) ([]byte, error) {
	return nil, r.record(name, args)
}

func (r *recorder) Start(name string, args ...string) error { return r.record(name, args) }

func (r *recorder) record(name string, args []string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls = append(r.calls, strings.Join(append([]string{name}, args...), " "))
	if r.missing[name] {
		return exec.ErrNotFound
	}
	return nil
}

func (r *recorder) Calls() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string{}, r.calls...)
}

// session is a long-running command (sync) with its combined output.
type session struct {
	out    *lockedBuffer
	cancel context.CancelFunc
	done   chan int
	once   sync.Once
	code   int
}

func (w *workspace) start(args ...string) *session {
	ctx, cancel := context.WithCancel(context.Background())
	s := &session{out: &lockedBuffer{}, cancel: cancel, done: make(chan int, 1)}
	go func() { s.done <- Run(ctx, args, w.env(s.out, s.out)) }()
	w.t.Cleanup(func() { s.stop() })
	return s
}

// stop cancels the command and returns its exit code.
func (s *session) stop() int {
	s.once.Do(func() {
		s.cancel()
		s.code = <-s.done
	})
	return s.code
}

func (s *session) waitFor(t *testing.T, pattern string) {
	t.Helper()
	re := regexp.MustCompile(pattern)
	eventually(t, func() bool { return re.MatchString(s.out.String()) },
		func() string { return "output matching " + pattern + "; got:\n" + s.out.String() })
}

func eventually(t *testing.T, condition func() bool, what func() string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !condition() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what())
		}
		time.Sleep(10 * time.Millisecond)
	}
}

type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// matches fails unless text has a line matching pattern.
func matches(t *testing.T, text, pattern string) {
	t.Helper()
	if !regexp.MustCompile("(?m)" + pattern).MatchString(text) {
		t.Errorf("no match for %q in:\n%s", pattern, text)
	}
}
```

- [ ] **Step 3: Write the failing scenario tests**

`internal/cli/root_test.go`:

```go
package cli

import (
	"strings"
	"testing"
)

func TestRun_Help(t *testing.T) {
	t.Parallel()
	r := newWorkspace(t, nil).run("--help")
	if r.code != 0 {
		t.Fatalf("code = %d, want 0", r.code)
	}
	for _, command := range []string{"init", "push", "pull", "sync", "info", "open"} {
		if !strings.Contains(r.stdout, "cdpn "+command) {
			t.Errorf("usage lacks %q:\n%s", "cdpn "+command, r.stdout)
		}
	}
}

func TestRun_UsageErrors(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		args    []string
		message string
	}{
		{name: "no command", args: nil, message: ""},
		{name: "unknown command", args: []string{"bogus"}, message: `unknown command "bogus"`},
		{name: "unknown flag", args: []string{"push", "--bogus"}, message: "unknown flag: --bogus"},
		{name: "too many args", args: []string{"pull", "a", "b"}, message: "accepts at most 1 arg"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			r := newWorkspace(t, nil).run(tt.args...)
			if r.code != 2 {
				t.Errorf("code = %d, want 2", r.code)
			}
			if !strings.Contains(r.stderr, "cdpn push") || !strings.Contains(r.stderr, tt.message) {
				t.Errorf("stderr lacks usage or %q:\n%s", tt.message, r.stderr)
			}
		})
	}
}

func TestRun_MissingAPIKey(t *testing.T) {
	t.Parallel()
	w := newWorkspace(t, nil)
	delete(w.vars, "CODEPEN_API_KEY")
	r := w.run("push")
	if r.code != 1 || !strings.Contains(r.stderr, "CODEPEN_API_KEY is not set") {
		t.Errorf("code %d, stderr %q", r.code, r.stderr)
	}
}

func TestRun_InvalidAPIKey(t *testing.T) {
	t.Parallel()
	w := newWorkspace(t, map[string]string{"a.js": "1"})
	w.vars["CODEPEN_API_KEY"] = "cp_user_wrong"
	r := w.run("push")
	if r.code != 1 || !strings.Contains(r.stderr, "401 UNAUTHENTICATED") {
		t.Errorf("code %d, stderr %q", r.code, r.stderr)
	}
}

func TestRun_UnknownPenExplainsScope(t *testing.T) {
	t.Parallel()
	r := newWorkspace(t, nil).run("pull", "01a120bd-0000-7000-8000-000000000000")
	if r.code != 1 || !strings.Contains(r.stderr, "only reaches your own Pens") {
		t.Errorf("code %d, stderr %q", r.code, r.stderr)
	}
}
```

`internal/cli/push_test.go`:

```go
package cli

import (
	"bytes"
	"encoding/hex"
	"regexp"
	"strings"
	"testing"

	"github.com/nocksock/codepen-cli/internal/codepen/codepentest"
)

var png, _ = hex.DecodeString("89504e470d0a1a0a0000000d49484452000000010000000108060000001f15c489")

func writes(w *workspace, id string) int {
	return w.api.Calls(`^PUT /v2/pens/` + id + `/files$`)
}

func TestPush_CreatesPen(t *testing.T) {
	t.Parallel()
	w := newWorkspace(t, map[string]string{"index.html": "<h1>x</h1>", "src/app.js": "1"})
	r := w.run("push")
	if r.code != 0 {
		t.Fatalf("code %d: %s", r.code, r.stderr)
	}
	if !strings.HasPrefix(r.stdout, "created https://codepen.io/editor/") {
		t.Errorf("stdout = %q", r.stdout)
	}
	match := regexp.MustCompile(`(?m)^CODEPEN_ID=(.+)$`).FindStringSubmatch(w.read(".env.local"))
	if match == nil {
		t.Fatalf(".env.local = %q", w.read(".env.local"))
	}
	files := w.api.Files(match[1])
	if files["/index.html"] != "<h1>x</h1>" || files["/src/app.js"] != "1" {
		t.Errorf("remote files = %v", files)
	}
	if !w.exists(".codepen/pen.config.json") {
		t.Error("the compiler's patch did not come back down")
	}
}

func TestPush_EmptyDirectoryGetsStarterFiles(t *testing.T) {
	t.Parallel()
	w := newWorkspace(t, nil)
	if r := w.run("push"); r.code != 0 {
		t.Fatalf("code %d: %s", r.code, r.stderr)
	}
	if got := w.read("index.html"); got != "<h1>starter</h1>" {
		t.Errorf("index.html = %q", got)
	}
}

func TestPush_NothingChanged(t *testing.T) {
	t.Parallel()
	w, id := connected(t, codepentest.Seed{Files: map[string]string{"a.js": "1"}})
	r := w.run("push")
	if r.code != 0 || r.stdout != "" || writes(w, id) != 0 {
		t.Errorf("code %d, stdout %q, writes %d", r.code, r.stdout, writes(w, id))
	}
}

func TestPush_UploadsAndDeletes(t *testing.T) {
	t.Parallel()
	w, id := connected(t, codepentest.Seed{Files: map[string]string{"a.js": "1", "b.js": "2"}})
	w.write("a.js", "changed")
	w.write("c.js", "new")
	w.remove("b.js")
	r := w.run("push")
	if r.code != 0 {
		t.Fatalf("code %d: %s", r.code, r.stderr)
	}
	for _, line := range []string{`~ /a\.js`, `~ /c\.js`, `- /b\.js`} {
		matches(t, r.stdout, line)
	}
	files := w.api.Files(id)
	if files["/a.js"] != "changed" || files["/c.js"] != "new" {
		t.Errorf("remote files = %v", files)
	}
	if _, ok := files["/b.js"]; ok {
		t.Error("/b.js was not deleted")
	}
}

func TestPush_BinaryFiles(t *testing.T) {
	t.Parallel()
	w, id := connected(t, codepentest.Seed{Files: map[string]string{"a.js": "1"}})
	w.writeBytes("img/dot.png", png)
	first := w.run("push")
	matches(t, first.stdout, `~ /img/dot\.png \(binary\)`)
	data, isBinary, ok := w.api.File(id, "/img/dot.png")
	if !ok || !isBinary || !bytes.Equal(data, png) {
		t.Errorf("remote dot.png = %v, binary %v, found %v", data, isBinary, ok)
	}
	second := w.run("push")
	if second.stdout != "" || writes(w, id) != 1 {
		t.Errorf("second push stdout %q, writes %d; want no write", second.stdout, writes(w, id))
	}
}

func TestPush_SkipsEmptyAndIgnoredFiles(t *testing.T) {
	t.Parallel()
	w, id := connected(t, codepentest.Seed{Files: map[string]string{"a.js": "1"}})
	w.write("empty.js", "")
	w.write("node_modules/dep/index.js", "x")
	w.write(".env.production", "SECRET=1")
	w.write(".envrc", "dotenv")
	r := w.run("push")
	if r.code != 0 || r.stdout != "" || writes(w, id) != 0 {
		t.Errorf("code %d, stdout %q, writes %d", r.code, r.stdout, writes(w, id))
	}
}

func TestPush_RefusesToEmptyPen(t *testing.T) {
	t.Parallel()
	w, id := connected(t, codepentest.Seed{Files: map[string]string{"a.js": "1"}})
	w.remove("a.js")
	w.remove(".codepen")
	r := w.run("push")
	if r.code != 1 || !strings.Contains(r.stderr, "refusing to push") || writes(w, id) != 0 {
		t.Errorf("code %d, stderr %q, writes %d", r.code, r.stderr, writes(w, id))
	}
}

func TestPush_PollsRunningBuild(t *testing.T) {
	t.Parallel()
	w, id := connected(t, codepentest.Seed{Files: map[string]string{"a.js": "1"}})
	w.write("a.js", "2")
	w.api.QueueBuilds(true)
	r := w.run("push")
	if r.code != 0 {
		t.Fatalf("code %d: %s", r.code, r.stderr)
	}
	matches(t, r.stdout, `build Success`)
	if w.api.Calls(`^GET /v2/builds/`) == 0 {
		t.Error("the build was never polled")
	}
	if got := w.api.Files(id)["/a.js"]; got != "2" {
		t.Errorf("remote a.js = %q", got)
	}
}
```

`internal/cli/pull_test.go`:

```go
package cli

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/nocksock/codepen-cli/internal/codepen/codepentest"
)

func TestPull_EditorURLDownloadsTextAndBinary(t *testing.T) {
	t.Parallel()
	w := newWorkspace(t, map[string]string{"keep.txt": "local only"})
	id := w.api.Seed(codepentest.Seed{Files: map[string]string{"index.html": "<p>hi</p>"}, Binaries: map[string][]byte{"dot.png": png}})
	r := w.run("pull", "https://codepen.io/editor/me/pen/"+id+"/abc123")
	if r.code != 0 {
		t.Fatalf("code %d: %s", r.code, r.stderr)
	}
	matches(t, r.stdout, `connected`)
	if got := w.read("index.html"); got != "<p>hi</p>" {
		t.Errorf("index.html = %q", got)
	}
	data, err := os.ReadFile(w.path("dot.png"))
	if err != nil || !bytes.Equal(data, png) {
		t.Errorf("dot.png = %v, %v", data, err)
	}
	if got := w.read("keep.txt"); got != "local only" {
		t.Errorf("pull touched keep.txt: %q", got)
	}
}

func TestPull_OverwritesLocalEdits(t *testing.T) {
	t.Parallel()
	w, id := connected(t, codepentest.Seed{Files: map[string]string{"a.js": "1"}})
	w.api.Edit(id, map[string]string{"a.js": "remote"})
	w.write("a.js", "local")
	r := w.run("pull")
	matches(t, r.stdout, `↓ /a\.js`)
	if got := w.read("a.js"); got != "remote" {
		t.Errorf("a.js = %q", got)
	}
}

func TestPull_WithoutPenFails(t *testing.T) {
	t.Parallel()
	r := newWorkspace(t, nil).run("pull")
	if r.code != 1 || !strings.Contains(r.stderr, "no Pen to pull") {
		t.Errorf("code %d, stderr %q", r.code, r.stderr)
	}
}

func TestPull_SwitchingNeedsForce(t *testing.T) {
	t.Parallel()
	w, _ := connected(t, codepentest.Seed{Files: map[string]string{"a.js": "1"}})
	other := w.api.Seed(codepentest.Seed{Files: map[string]string{"b.js": "2"}})
	refused := w.run("pull", other)
	if refused.code != 1 || !strings.Contains(refused.stderr, "pass --force") {
		t.Errorf("refused: code %d, stderr %q", refused.code, refused.stderr)
	}
	forced := w.run("pull", other, "--force")
	if forced.code != 0 {
		t.Fatalf("forced: code %d: %s", forced.code, forced.stderr)
	}
	if got := w.read(".env.local"); got != "CODEPEN_ID="+other+"\n" {
		t.Errorf(".env.local = %q", got)
	}
	if got := w.read("b.js"); got != "2" {
		t.Errorf("b.js = %q", got)
	}
}
```

- [ ] **Step 4: Run them to verify they fail**

Run: `go test ./internal/cli/`
Expected: FAIL, `undefined: Run` / `undefined: Env`.

- [ ] **Step 5: Implement** — `internal/cli/cli.go`

```go
// Package cli is cdpn's command line: it parses arguments, wires the packages
// together, and turns their results and errors into output and exit codes.
package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"runtime"
)

// Env is everything cdpn takes from the outside world. Tests build their own.
type Env struct {
	Dir    string                      // working directory, absolute
	Getenv func(string) (string, bool) // process environment
	Stdout io.Writer
	Stderr io.Writer
	Runner Runner // runs op, direnv and the browser
	GOOS   string
}

// Runner runs external programs in Env.Dir, never through a shell.
type Runner interface {
	// Output runs a program to completion and returns its standard output.
	Output(ctx context.Context, name string, args ...string) ([]byte, error)
	// Start launches a program without waiting for it.
	Start(name string, args ...string) error
}

// System is the environment of the running process.
func System() (Env, error) {
	dir, err := os.Getwd()
	if err != nil {
		return Env{}, fmt.Errorf("finding the working directory: %w", err)
	}
	return Env{
		Dir:    dir,
		Getenv: os.LookupEnv,
		Stdout: os.Stdout,
		Stderr: os.Stderr,
		Runner: execRunner{dir: dir},
		GOOS:   runtime.GOOS,
	}, nil
}

// Run executes cdpn with args (without the program name) and returns the exit
// code: 0 success, 1 runtime or API error, 2 usage error.
func Run(ctx context.Context, args []string, env Env) int {
	if args == nil {
		args = []string{} // nil would make Cobra read os.Args
	}
	root := newRoot(&app{env: env})
	root.SetArgs(args)
	root.SetOut(env.Stdout)
	root.SetErr(env.Stderr)
	return exitCode(env, root.ExecuteContext(ctx))
}
```

`internal/cli/runner.go`:

```go
package cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
)

// execRunner runs programs with os/exec in dir.
type execRunner struct{ dir string }

func (r execRunner) Output(ctx context.Context, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = r.dir
	out, err := cmd.Output()
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) && len(exitErr.Stderr) > 0 {
		return out, fmt.Errorf("%s: %w: %s", name, err, bytes.TrimSpace(exitErr.Stderr))
	}
	if err != nil {
		return out, fmt.Errorf("%s: %w", name, err)
	}
	return out, nil
}

func (r execRunner) Start(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Dir = r.dir
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	return cmd.Process.Release()
}
```

`internal/cli/app.go`:

```go
package cli

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/nocksock/codepen-cli/internal/codepen"
	"github.com/nocksock/codepen-cli/internal/config"
	"github.com/nocksock/codepen-cli/internal/envfile"
	"github.com/nocksock/codepen-cli/internal/pen"
	"github.com/nocksock/codepen-cli/internal/secret"
)

// app builds what the commands need from Env.
type app struct{ env Env }

func (a *app) config() (config.Config, error) {
	return config.Load(a.env.Dir, a.env.Getenv)
}

// client returns an API client for cfg, resolving an op:// key first.
func (a *app) client(ctx context.Context, cfg config.Config) (*codepen.Client, error) {
	keyRef, err := cfg.Key()
	if err != nil {
		return nil, err
	}
	key, err := secret.Resolve(ctx, keyRef, a.env.Runner)
	if err != nil {
		return nil, err
	}
	opts := []codepen.Option{}
	if cfg.APIURL != "" {
		opts = append(opts, codepen.WithBaseURL(cfg.APIURL))
	}
	if cfg.PollInterval > 0 {
		opts = append(opts, codepen.WithPollInterval(cfg.PollInterval))
	}
	return codepen.New(key, opts...), nil
}

// mirror opens the Mirror for the working directory.
func (a *app) mirror(ctx context.Context) (*pen.Mirror, config.Config, error) {
	cfg, err := a.config()
	if err != nil {
		return nil, cfg, err
	}
	api, err := a.client(ctx, cfg)
	if err != nil {
		return nil, cfg, err
	}
	m := pen.Open(pen.Config{
		API:      api,
		Root:     cfg.Dir,
		ID:       cfg.PenID,
		SaveID:   a.saveID,
		Reporter: printer{w: a.env.Stdout},
	})
	return m, cfg, nil
}

// connected returns a client and config for commands that need a connected Pen.
func (a *app) connected(ctx context.Context) (*codepen.Client, config.Config, error) {
	cfg, err := a.config()
	if err != nil {
		return nil, cfg, err
	}
	api, err := a.client(ctx, cfg)
	if err != nil {
		return nil, cfg, err
	}
	if cfg.PenID == "" {
		return nil, cfg, errNotConnected
	}
	return api, cfg, nil
}

// saveID records the connected Pen in the state file of the working directory.
func (a *app) saveID(id string) error {
	return envfile.Set(filepath.Join(a.env.Dir, config.StateFile), "CODEPEN_ID", id)
}

func (a *app) printf(format string, args ...any) {
	fmt.Fprintf(a.env.Stdout, format, args...)
}
```

`internal/cli/errors.go`:

```go
package cli

import (
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/nocksock/codepen-cli/internal/codepen"
)

// usageError is a mistake in how cdpn was called: exit 2 with the usage text.
type usageError struct{ err error }

func (e usageError) Error() string {
	if e.err == nil {
		return "usage"
	}
	return e.err.Error()
}

func (e usageError) Unwrap() error { return e.err }

var errNotConnected = errors.New("no Pen connected: run `cdpn push` or `cdpn pull <PEN>`")

// usageArgs turns the errors of an argument check into usage errors.
func usageArgs(check cobra.PositionalArgs) cobra.PositionalArgs {
	return func(cmd *cobra.Command, args []string) error {
		if err := check(cmd, args); err != nil {
			return usageError{err: err}
		}
		return nil
	}
}

// exitCode reports err the way cdpn always has and returns the exit code.
func exitCode(env Env, err error) int {
	var usage usageError
	switch {
	case err == nil:
		return 0
	case errors.As(err, &usage):
		if usage.err != nil {
			fmt.Fprintf(env.Stderr, "✗ %v\n\n", usage.err)
		}
		fmt.Fprintln(env.Stderr, usageText())
		return 2
	default:
		fmt.Fprintf(env.Stderr, "✗ %s\n", explain(err))
		return 1
	}
}

// explain turns the API errors people run into into advice.
func explain(err error) string {
	switch {
	case errors.Is(err, codepen.ErrConflict):
		return "the Pen changed remotely (editor?). run `cdpn pull` first, or push with --force"
	case errors.Is(err, codepen.ErrNotFound):
		return "Pen not found. The API only reaches your own Pens (fork someone else's on codepen.io first)"
	default:
		return err.Error()
	}
}
```

`internal/cli/render.go`:

```go
package cli

import (
	"fmt"
	"io"
	"strings"

	"github.com/nocksock/codepen-cli/internal/codepen"
	"github.com/nocksock/codepen-cli/internal/pen"
)

// printer shows a Mirror's progress on w.
type printer struct{ w io.Writer }

func (p printer) Report(e pen.Event) {
	fmt.Fprint(p.w, render(e))
}

func render(e pen.Event) string {
	var b strings.Builder
	switch e := e.(type) {
	case pen.Planned:
		for _, write := range e.Changes.Writes {
			fmt.Fprintf(&b, "  ~ %s\n", write.Path)
		}
		for _, p := range e.Changes.Deletes {
			fmt.Fprintf(&b, "  - %s\n", p)
		}
		for _, upload := range e.Changes.Uploads {
			fmt.Fprintf(&b, "  ~ %s (binary)\n", upload.Path)
		}
	case pen.Built:
		renderBuild(&b, e.Build)
	case pen.Downloaded:
		fmt.Fprintf(&b, "  ↓ %s\n", e.Path)
	case pen.Created:
		fmt.Fprintf(&b, "created %s\n", e.URLs.Editor)
	case pen.Connected:
		fmt.Fprintf(&b, "connected %s\n", e.URLs.Editor)
	}
	return b.String()
}

func renderBuild(b *strings.Builder, build codepen.Build) {
	for _, patch := range build.AppliedPatches {
		fmt.Fprintf(b, "  patched %s %s\n", patch.Type, patch.Path)
	}
	for _, e := range build.Errors {
		line := ""
		if e.Range != nil && e.Range.Start.Line > 0 {
			line = fmt.Sprintf(":%d", e.Range.Start.Line)
		}
		fmt.Fprintf(b, "  ✗ %s%s %s\n", e.Path, line, e.Message)
	}
	fmt.Fprintf(b, "  build %s\n", build.Status)
}
```

`internal/cli/root.go`:

```go
package cli

import (
	"fmt"
	"runtime/debug"

	"github.com/spf13/cobra"

	"github.com/nocksock/codepen-cli/internal/setup"
)

// version is set when building releases:
// -ldflags "-X github.com/nocksock/codepen-cli/internal/cli.version=v1.2.3"
var version string

const usageFormat = `cdpn — mirror a local directory to a CodePen 2.0 Pen

  cdpn init [PEN]         set up .env, .envrc and .gitignore here; PEN pulls it
  cdpn push [--force]     push local files (creates a Pen if CODEPEN_ID is unset)
  cdpn pull [PEN]         download the Pen's files; PEN (id or URL) connects to it
  cdpn sync [--force]     push, then watch and push on every change
  cdpn info               show the Pen's details and local sync status
  cdpn open [TARGET]      open editor (default), preview or live in the browser
  cdpn completion SHELL   print a completion script (bash, zsh, fish, powershell)

env (.env, .env.local):
  CODEPEN_API_KEY   key or 1Password ref, e.g. "op://Private/CodePen/API Key"
  CODEPEN_ID        Pen id, written automatically on first push
  CODEPEN_DIR       directory to mirror (default: .)

--key      init: API key or 1Password ref for a new .env (default: %q)
--force    push/sync: skip the version check and overwrite remote edits
           pull: switch to PEN although another Pen is connected
--version  print the version`

func usageText() string {
	return fmt.Sprintf(usageFormat, setup.DefaultKey)
}

func buildVersion() string {
	if version != "" {
		return version
	}
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" {
		return info.Main.Version
	}
	return "dev"
}

func newRoot(a *app) *cobra.Command {
	root := &cobra.Command{
		Use:           "cdpn",
		Short:         "mirror a local directory to a CodePen 2.0 Pen",
		Version:       buildVersion(),
		SilenceUsage:  true,
		SilenceErrors: true,
		Args: func(_ *cobra.Command, args []string) error {
			if len(args) > 0 {
				return usageError{err: fmt.Errorf("unknown command %q", args[0])}
			}
			return nil
		},
		RunE: func(*cobra.Command, []string) error { return usageError{} },
	}
	root.SetFlagErrorFunc(func(_ *cobra.Command, err error) error { return usageError{err: err} })
	root.AddCommand(newPushCmd(a), newPullCmd(a))
	help := root.HelpFunc()
	root.SetHelpFunc(func(c *cobra.Command, args []string) {
		if c != root {
			help(c, args)
			return
		}
		fmt.Fprintln(c.OutOrStdout(), usageText())
	})
	return root
}
```

`internal/cli/push.go`:

```go
package cli

import (
	"github.com/spf13/cobra"

	"github.com/nocksock/codepen-cli/internal/pen"
)

func newPushCmd(a *app) *cobra.Command {
	var force bool
	cmd := &cobra.Command{
		Use:   "push",
		Short: "push local files (creates a Pen if CODEPEN_ID is unset)",
		Args:  usageArgs(cobra.NoArgs),
		RunE: func(cmd *cobra.Command, _ []string) error {
			m, _, err := a.mirror(cmd.Context())
			if err != nil {
				return err
			}
			return m.Push(cmd.Context(), pen.PushOptions{Force: force})
		},
	}
	cmd.Flags().BoolVar(&force, "force", false, "skip the version check and overwrite remote edits")
	return cmd
}
```

`internal/cli/pull.go`:

```go
package cli

import (
	"github.com/spf13/cobra"

	"github.com/nocksock/codepen-cli/internal/pen"
)

func newPullCmd(a *app) *cobra.Command {
	var force bool
	cmd := &cobra.Command{
		Use:   "pull [PEN]",
		Short: "download the Pen's files; PEN (id or URL) connects to it",
		Args:  usageArgs(cobra.MaximumNArgs(1)),
		RunE: func(cmd *cobra.Command, args []string) error {
			m, _, err := a.mirror(cmd.Context())
			if err != nil {
				return err
			}
			opts := pen.PullOptions{Force: force}
			if len(args) == 1 {
				opts.Ref = args[0]
			}
			return m.Pull(cmd.Context(), opts)
		},
	}
	cmd.Flags().BoolVar(&force, "force", false, "switch to PEN although another Pen is connected")
	return cmd
}
```

- [ ] **Step 6: Run tests**

Run: `go test -race ./internal/cli/`
Expected: `ok`. If goleak reports leaked goroutines, check the reported stack. If it's an HTTP keep-alive goroutine not covered by the ignores, add its top function to `TestMain`. If it's a cdpn goroutine, fix the leak.

- [ ] **Step 7: Commit**

```bash
git add go.mod go.sum internal/cli
git commit -m "feat(cli): add cobra cli with push and pull

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 15: `cli` `init`, `info` and `open`

**Files:**
- Create: `internal/cli/init.go`, `internal/cli/info.go`, `internal/cli/open.go`
- Modify: `internal/cli/root.go` (the `root.AddCommand` line)
- Test: `internal/cli/init_test.go`, `internal/cli/info_test.go`, `internal/cli/open_test.go`

Reference: `test/cli-init.test.js`, `test/cli-info-open.test.js`. One change: `open bogus` is now a usage error (exit 2).

- [ ] **Step 1: Write the failing tests**

`internal/cli/init_test.go`:

```go
package cli

import (
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/nocksock/codepen-cli/internal/codepen/codepentest"
)

func TestInit_CreatesFilesAndAllowsDirenv(t *testing.T) {
	t.Parallel()
	w := newWorkspace(t, nil)
	r := w.run("init")
	if r.code != 0 {
		t.Fatalf("code %d: %s", r.code, r.stderr)
	}
	matches(t, r.stdout, `created \.env$`)
	matches(t, r.stdout, `direnv allowed`)
	expected := map[string]string{
		".env":       "CODEPEN_API_KEY=\"op://Private/CodePen/API Key\"\n",
		".envrc":     "dotenv_if_exists .env\ndotenv_if_exists .env.local\n",
		".gitignore": ".env\n.env.local\n",
	}
	for file, content := range expected {
		if got := w.read(file); got != content {
			t.Errorf("%s = %q, want %q", file, got, content)
		}
	}
	if calls := w.runner.Calls(); !slices.Equal(calls, []string{"direnv allow ."}) {
		t.Errorf("calls = %q", calls)
	}
}

func TestInit_KeySetsNewEnv(t *testing.T) {
	t.Parallel()
	w := newWorkspace(t, nil)
	w.run("init", "--key", "cp_user_abc")
	if got := w.read(".env"); got != "CODEPEN_API_KEY=\"cp_user_abc\"\n" {
		t.Errorf(".env = %q", got)
	}
}

func TestInit_TwiceChangesNothing(t *testing.T) {
	t.Parallel()
	w := newWorkspace(t, nil)
	w.run("init")
	files := []string{".env", ".envrc", ".gitignore"}
	before := []string{}
	for _, f := range files {
		before = append(before, w.read(f))
	}
	r := w.run("init")
	if got := strings.Count(r.stdout, "kept"); got != 3 {
		t.Errorf("kept %d files, want 3:\n%s", got, r.stdout)
	}
	for i, f := range files {
		if got := w.read(f); got != before[i] {
			t.Errorf("%s changed to %q", f, got)
		}
	}
}

func TestInit_AppendsAndKeepsExistingEnv(t *testing.T) {
	t.Parallel()
	w := newWorkspace(t, map[string]string{
		".env":       "CODEPEN_API_KEY=mine\n",
		".envrc":     "export FOO=1",
		".gitignore": "node_modules\n.env\n",
	})
	r := w.run("init", "--key", "ignored")
	if !regexp.MustCompile(`kept {4}\.env\n`).MatchString(r.stdout) {
		t.Errorf("stdout = %q", r.stdout)
	}
	matches(t, r.stdout, `updated \.envrc`)
	if got := w.read(".env"); got != "CODEPEN_API_KEY=mine\n" {
		t.Errorf(".env = %q", got)
	}
	if got := w.read(".envrc"); got != "export FOO=1\ndotenv_if_exists .env\ndotenv_if_exists .env.local\n" {
		t.Errorf(".envrc = %q", got)
	}
	if got := w.read(".gitignore"); got != "node_modules\n.env\n.env.local\n" {
		t.Errorf(".gitignore = %q", got)
	}
}

func TestInit_WithoutDirenv(t *testing.T) {
	t.Parallel()
	w := newWorkspace(t, nil)
	w.runner.missing["direnv"] = true
	r := w.run("init")
	if r.code != 0 || strings.Contains(r.stdout, "direnv allowed") {
		t.Errorf("code %d, stdout %q", r.code, r.stdout)
	}
}

func TestInit_PenConnectsAndPulls(t *testing.T) {
	t.Parallel()
	w := newWorkspace(t, nil)
	id := w.api.Seed(codepentest.Seed{Files: map[string]string{"index.html": "<p>hi</p>"}})
	if r := w.run("init", id); r.code != 0 {
		t.Fatalf("code %d: %s", r.code, r.stderr)
	}
	if got := w.read("index.html"); got != "<p>hi</p>" {
		t.Errorf("index.html = %q", got)
	}
	if got := w.read(".env.local"); got != "CODEPEN_ID="+id+"\n" {
		t.Errorf(".env.local = %q", got)
	}
}
```

`internal/cli/info_test.go`:

```go
package cli

import (
	"regexp"
	"testing"

	"github.com/nocksock/codepen-cli/internal/codepen"
	"github.com/nocksock/codepen-cli/internal/codepen/codepentest"
)

const live = "https://cheerful-term-baboon.codepen.app/"

func TestInfo_InSync(t *testing.T) {
	t.Parallel()
	w, id := connected(t, codepentest.Seed{Title: "Demo", Tags: []string{"api", "demo"}, Files: map[string]string{"a.js": "12345"}})
	r := w.run("info")
	if r.code != 0 {
		t.Fatalf("code %d: %s", r.code, r.stderr)
	}
	for _, line := range []string{
		`^title +Demo$`,
		`^id +` + regexp.QuoteMeta(id) + ` \(V2\)$`,
		`^tags +api, demo$`,
		`^versions +1, head `,
		`^build +Success$`,
		`^files +2 files, 18 B text$`,
		`^local +in sync$`,
		`^deployed +no$`,
		`^editor +https://codepen\.io/editor/`,
	} {
		matches(t, r.stdout, line)
	}
}

func TestInfo_CountsPendingChanges(t *testing.T) {
	t.Parallel()
	w, _ := connected(t, codepentest.Seed{Files: map[string]string{"a.js": "1", "b.js": "2"}})
	w.write("a.js", "changed")
	w.write("c.js", "new")
	w.remove("b.js")
	matches(t, w.run("info").stdout, `^local +2 to upload, 1 to delete \(cdpn push\)$`)
}

func TestInfo_DeploymentURL(t *testing.T) {
	t.Parallel()
	w, _ := connected(t, codepentest.Seed{Files: map[string]string{"a.js": "1"}, Deployment: &codepen.Deployment{URL: live}})
	matches(t, w.run("info").stdout, `^deployed +`+regexp.QuoteMeta(live)+`$`)
}

func TestInfo_NeedsConnectedPen(t *testing.T) {
	t.Parallel()
	r := newWorkspace(t, nil).run("info")
	if r.code != 1 {
		t.Errorf("code = %d, want 1", r.code)
	}
	matches(t, r.stderr, `no Pen connected`)
}
```

`internal/cli/open_test.go`:

```go
package cli

import (
	"slices"
	"strings"
	"testing"

	"github.com/nocksock/codepen-cli/internal/codepen"
	"github.com/nocksock/codepen-cli/internal/codepen/codepentest"
)

func TestOpen_LaunchesTargets(t *testing.T) {
	t.Parallel()
	w, id := connected(t, codepentest.Seed{Files: map[string]string{"a.js": "1"}, Deployment: &codepen.Deployment{URL: live}})
	urls := w.api.URLs(id)
	tests := []struct {
		args     []string
		expected string
	}{
		{args: nil, expected: urls.Editor},
		{args: []string{"preview"}, expected: urls.Preview},
		{args: []string{"live"}, expected: live},
	}
	for _, tt := range tests {
		r := w.run(append([]string{"open"}, tt.args...)...)
		if r.code != 0 || strings.TrimSpace(r.stdout) != tt.expected {
			t.Errorf("open %v: code %d, stdout %q, want %q", tt.args, r.code, r.stdout, tt.expected)
		}
	}
	expected := []string{"xdg-open " + urls.Editor, "xdg-open " + urls.Preview, "xdg-open " + live}
	if calls := w.runner.Calls(); !slices.Equal(calls, expected) {
		t.Errorf("calls = %q, want %q", calls, expected)
	}
}

func TestOpen_Failures(t *testing.T) {
	t.Parallel()
	w, _ := connected(t, codepentest.Seed{Files: map[string]string{"a.js": "1"}})
	undeployed := w.run("open", "live")
	if undeployed.code != 1 || !strings.Contains(undeployed.stderr, "no live URL") {
		t.Errorf("open live: code %d, stderr %q", undeployed.code, undeployed.stderr)
	}
	bogus := w.run("open", "bogus")
	if bogus.code != 2 || !strings.Contains(bogus.stderr, `unknown target "bogus"`) {
		t.Errorf("open bogus: code %d, stderr %q", bogus.code, bogus.stderr)
	}
	if calls := w.runner.Calls(); len(calls) != 0 {
		t.Errorf("calls = %q, want none", calls)
	}
}

func TestOpen_NeedsConnectedPen(t *testing.T) {
	t.Parallel()
	r := newWorkspace(t, nil).run("open")
	if r.code != 1 || !strings.Contains(r.stderr, "no Pen connected") {
		t.Errorf("code %d, stderr %q", r.code, r.stderr)
	}
}
```

- [ ] **Step 2: Run them to verify they fail**

Run: `go test ./internal/cli/ -run 'TestInit|TestInfo|TestOpen'`
Expected: FAIL. The commands don't exist yet, so `cdpn init` exits 2 with `unknown command "init"`.

- [ ] **Step 3: Implement** — `internal/cli/init.go`

```go
package cli

import (
	"context"

	"github.com/spf13/cobra"

	"github.com/nocksock/codepen-cli/internal/pen"
	"github.com/nocksock/codepen-cli/internal/setup"
)

func newInitCmd(a *app) *cobra.Command {
	var key string
	cmd := &cobra.Command{
		Use:   "init [PEN]",
		Short: "set up .env, .envrc and .gitignore here; PEN pulls it",
		Args:  usageArgs(cobra.MaximumNArgs(1)),
		RunE: func(cmd *cobra.Command, args []string) error {
			penRef := ""
			if len(args) == 1 {
				penRef = args[0]
			}
			return a.initDir(cmd.Context(), key, penRef)
		},
	}
	cmd.Flags().StringVar(&key, "key", setup.DefaultKey, "API key or 1Password ref for a new .env")
	return cmd
}

// initDir scaffolds the working directory, allows direnv and, given a Pen,
// connects to it and pulls it.
func (a *app) initDir(ctx context.Context, key, penRef string) error {
	results, err := setup.Scaffold(a.env.Dir, key)
	for _, r := range results {
		a.printf("  %-7s %s\n", r.Status, r.File)
	}
	if err != nil {
		return err
	}
	if setup.AllowDirenv(ctx, a.env.Runner) {
		a.printf("  direnv allowed\n")
	}
	if penRef == "" {
		a.printf("next: `cdpn push` creates a Pen, `cdpn pull <PEN>` connects an existing one\n")
		return nil
	}
	m, _, err := a.mirror(ctx)
	if err != nil {
		return err
	}
	return m.Pull(ctx, pen.PullOptions{Ref: penRef})
}
```

`internal/cli/info.go`:

```go
package cli

import (
	"context"
	"time"

	"github.com/spf13/cobra"

	"github.com/nocksock/codepen-cli/internal/info"
)

func newInfoCmd(a *app) *cobra.Command {
	return &cobra.Command{
		Use:   "info",
		Short: "show the Pen's details and local sync status",
		Args:  usageArgs(cobra.NoArgs),
		RunE:  func(cmd *cobra.Command, _ []string) error { return a.showInfo(cmd.Context()) },
	}
}

func (a *app) showInfo(ctx context.Context) error {
	api, cfg, err := a.connected(ctx)
	if err != nil {
		return err
	}
	status, err := info.Gather(ctx, api, cfg.PenID, cfg.Dir)
	if err != nil {
		return err
	}
	a.printf("%s\n", info.Render(status, time.Now()))
	return nil
}
```

`internal/cli/open.go`:

```go
package cli

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/nocksock/codepen-cli/internal/browser"
	"github.com/nocksock/codepen-cli/internal/codepen"
)

// targets pick a URL of a Pen; "" means the Pen has none.
var targets = map[string]func(codepen.Pen) string{
	"editor":  func(p codepen.Pen) string { return p.URLs.Editor },
	"preview": func(p codepen.Pen) string { return p.URLs.Preview },
	"live": func(p codepen.Pen) string {
		if p.Deployment == nil {
			return ""
		}
		return p.Deployment.URL
	},
}

func newOpenCmd(a *app) *cobra.Command {
	return &cobra.Command{
		Use:       "open [editor|preview|live]",
		Short:     "open editor (default), preview or live in the browser",
		ValidArgs: []string{"editor", "preview", "live"},
		Args:      usageArgs(cobra.MatchAll(cobra.MaximumNArgs(1), knownTarget)),
		RunE: func(cmd *cobra.Command, args []string) error {
			target := "editor"
			if len(args) == 1 {
				target = args[0]
			}
			return a.openPen(cmd.Context(), target)
		},
	}
}

func knownTarget(_ *cobra.Command, args []string) error {
	if len(args) == 1 && targets[args[0]] == nil {
		return fmt.Errorf("unknown target %q: use editor, preview or live", args[0])
	}
	return nil
}

// openPen prints the target URL and opens it in the browser without waiting.
func (a *app) openPen(ctx context.Context, target string) error {
	api, cfg, err := a.connected(ctx)
	if err != nil {
		return err
	}
	p, err := api.Pen(ctx, cfg.PenID)
	if err != nil {
		return err
	}
	href := targets[target](p)
	if href == "" {
		return fmt.Errorf("the Pen has no %s URL (not deployed?)", target)
	}
	a.printf("%s\n", href)
	name, args := browser.Command(a.env.GOOS, href)
	if err := a.env.Runner.Start(name, args...); err != nil {
		a.printf("could not launch a browser; open %s\n", href)
	}
	return nil
}
```

In `internal/cli/root.go`, change the `AddCommand` line to:

```go
	root.AddCommand(newInitCmd(a), newPushCmd(a), newPullCmd(a), newInfoCmd(a), newOpenCmd(a))
```

- [ ] **Step 4: Run tests**

Run: `go test -race ./internal/cli/`
Expected: `ok`.

- [ ] **Step 5: Commit**

```bash
git add internal/cli
git commit -m "feat(cli): add init, info and open commands

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 16: `cli` `sync`

**Files:**
- Create: `internal/cli/sync.go`
- Modify: `internal/cli/root.go` (the `AddCommand` line)
- Test: `internal/cli/sync_test.go`

Reference: `sync()` in `bin/cdpn`, `test/cli-sync.test.js`. Watching starts before the first push, so edits made during that push are caught. One goroutine runs the pushes, so they never overlap.

- [ ] **Step 1: Write the failing test** — `internal/cli/sync_test.go`

```go
package cli

import (
	"testing"

	"github.com/nocksock/codepen-cli/internal/codepen/codepentest"
)

func TestSync_PushesChanges(t *testing.T) {
	t.Parallel()
	w, id := connected(t, codepentest.Seed{Files: map[string]string{"a.js": "1"}})
	s := w.start("sync")
	s.waitFor(t, `watching`)
	matches(t, s.out.String(), `^editor +https://codepen\.io/editor/`)

	w.write("a.js", "2")
	s.waitFor(t, `~ /a\.js`)
	eventually(t, func() bool { return w.api.Files(id)["/a.js"] == "2" }, func() string { return "remote a.js = 2" })

	w.write("src/new.js", "3")
	s.waitFor(t, `~ /src/new\.js`)
	eventually(t, func() bool { return w.api.Files(id)["/src/new.js"] == "3" }, func() string { return "remote src/new.js = 3" })

	if code := s.stop(); code != 0 {
		t.Errorf("exit code after cancel = %d, want 0", code)
	}
}

func TestSync_ReportsRemoteEdit(t *testing.T) {
	t.Parallel()
	w, id := connected(t, codepentest.Seed{Files: map[string]string{"a.js": "1"}})
	s := w.start("sync")
	s.waitFor(t, `watching`)
	w.api.Edit(id, map[string]string{"a.js": "from the editor"})
	w.write("a.js", "local")
	s.waitFor(t, `changed remotely`)
	if got := w.api.Files(id)["/a.js"]; got != "from the editor" {
		t.Errorf("remote a.js = %q, the remote edit was overwritten", got)
	}
}

func TestSync_ForceOverwritesRemoteEdit(t *testing.T) {
	t.Parallel()
	w, id := connected(t, codepentest.Seed{Files: map[string]string{"a.js": "1"}})
	s := w.start("sync", "--force")
	s.waitFor(t, `watching`)
	w.api.Edit(id, map[string]string{"a.js": "from the editor"})
	w.write("a.js", "local")
	s.waitFor(t, `~ /a\.js`)
	eventually(t, func() bool { return w.api.Files(id)["/a.js"] == "local" }, func() string { return "remote a.js = local" })
}
```

- [ ] **Step 2: Run it to verify it fails**

Run: `go test ./internal/cli/ -run TestSync`
Expected: FAIL, timed out waiting for `watching` (output shows `unknown command "sync"`).

- [ ] **Step 3: Implement** — `internal/cli/sync.go`

```go
package cli

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/nocksock/codepen-cli/internal/pen"
	"github.com/nocksock/codepen-cli/internal/tree"
	"github.com/nocksock/codepen-cli/internal/watch"
)

func newSyncCmd(a *app) *cobra.Command {
	var force bool
	cmd := &cobra.Command{
		Use:   "sync",
		Short: "push, then watch and push on every change",
		Args:  usageArgs(cobra.NoArgs),
		RunE: func(cmd *cobra.Command, _ []string) error {
			return a.syncPen(cmd.Context(), force)
		},
	}
	cmd.Flags().BoolVar(&force, "force", false, "skip the version check and overwrite remote edits")
	return cmd
}

// syncPen pushes, then pushes again whenever the directory changes, until ctx
// is done. Push errors are reported and watching goes on.
func (a *app) syncPen(ctx context.Context, force bool) error {
	m, cfg, err := a.mirror(ctx)
	if err != nil {
		return err
	}
	changes, err := watch.Changes(ctx, cfg.Dir, watch.Options{Skip: tree.IsIgnored})
	if err != nil {
		return err
	}
	push := func() {
		err := m.Push(ctx, pen.PushOptions{Force: force})
		if err != nil && ctx.Err() == nil {
			fmt.Fprintf(a.env.Stderr, "✗ %s\n", explain(err))
		}
	}
	push()
	if urls := m.URLs(); urls.Editor != "" {
		a.printf("editor  %s\npreview %s\n", urls.Editor, urls.Preview)
	}
	a.printf("watching %s\n", cfg.Dir)
	for range changes {
		push()
	}
	return nil
}
```

In `internal/cli/root.go`, change the `AddCommand` line to:

```go
	root.AddCommand(newInitCmd(a), newPushCmd(a), newPullCmd(a), newSyncCmd(a), newInfoCmd(a), newOpenCmd(a))
```

- [ ] **Step 4: Run tests**

Run: `go test -race -count=3 ./internal/cli/`
Expected: `ok` all three runs.

- [ ] **Step 5: Commit**

```bash
git add internal/cli
git commit -m "feat(cli): add sync command

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 17: `cmd/cdpn` entry point and binary smoke test

**Files:**
- Create: `cmd/cdpn/main.go`
- Test: `cmd/cdpn/main_test.go`

- [ ] **Step 1: Write the failing test** — `cmd/cdpn/main_test.go`

```go
package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

var binary string

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "cdpn-smoke-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	binary = filepath.Join(dir, "cdpn")
	build := exec.Command("go", "build", "-o", binary, ".")
	build.Stderr = os.Stderr
	if err := build.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "building cdpn:", err)
		os.Exit(1)
	}
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

func TestExitCodes(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		args   []string
		code   int
		output string
	}{
		{name: "help", args: []string{"--help"}, code: 0, output: "cdpn push"},
		{name: "unknown command", args: []string{"bogus"}, code: 2, output: `unknown command "bogus"`},
		{name: "missing key", args: []string{"push"}, code: 1, output: "CODEPEN_API_KEY is not set"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			cmd := exec.Command(binary, tt.args...)
			cmd.Dir = t.TempDir()
			cmd.Env = []string{"HOME=" + cmd.Dir} // nothing from the developer's environment
			out, err := cmd.CombinedOutput()
			code := 0
			var exitErr *exec.ExitError
			switch {
			case errors.As(err, &exitErr):
				code = exitErr.ExitCode()
			case err != nil:
				t.Fatal(err)
			}
			if code != tt.code || !strings.Contains(string(out), tt.output) {
				t.Errorf("cdpn %v: exit %d, want %d; output:\n%s", tt.args, code, tt.code, out)
			}
		})
	}
}
```

- [ ] **Step 2: Run it to verify it fails**

Run: `go test ./cmd/cdpn/`
Expected: FAIL, the build fails with `function main is undeclared in the main package`.

- [ ] **Step 3: Implement** — `cmd/cdpn/main.go`

```go
// Command cdpn mirrors a local directory to a CodePen 2.0 Pen.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/nocksock/codepen-cli/internal/cli"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	env, err := cli.System()
	if err != nil {
		fmt.Fprintf(os.Stderr, "✗ %v\n", err)
		os.Exit(1)
	}
	code := cli.Run(ctx, os.Args[1:], env)
	stop()
	os.Exit(code)
}
```

- [ ] **Step 4: Run all tests and try the binary**

Run: `go test -race -shuffle=on ./... && go run ./cmd/cdpn --version`
Expected: every package `ok`, then `cdpn version dev` (or a pseudo-version).

- [ ] **Step 5: Commit**

```bash
git add cmd
git commit -m "feat(cdpn): add entry point with signal handling and smoke test

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 18: Lint and goreleaser

**Files:**
- Create: `.goreleaser.yaml`
- Modify: whatever the linter flags

- [ ] **Step 1: Run the linter**

Run: `just lint`
Expected: either clean or a list of findings. Fix each finding in the code; don't disable linters globally. Where a finding is a deliberate choice (for example the ldflags-set `version` global), add a `//nolint:<linter> // <reason>` comment on that line. Re-run until clean.

- [ ] **Step 2: Re-run the tests**

Run: `go test -race -shuffle=on ./...`
Expected: all `ok`.

- [ ] **Step 3: Add `.goreleaser.yaml`**

```yaml
version: 2
project_name: cdpn

before:
  hooks:
    - go mod tidy

builds:
  - id: cdpn
    main: ./cmd/cdpn
    binary: cdpn
    env:
      - CGO_ENABLED=0
    flags:
      - -trimpath
    ldflags:
      - -s -w -X github.com/nocksock/codepen-cli/internal/cli.version={{ .Version }}
    goos: [linux, darwin, windows]
    goarch: [amd64, arm64]

archives:
  - formats: [tar.gz]
    format_overrides:
      - goos: windows
        formats: [zip]
    name_template: "{{ .ProjectName }}_{{ .Version }}_{{ .Os }}_{{ .Arch }}"

checksum:
  name_template: checksums.txt

snapshot:
  version_template: "{{ incpatch .Version }}-next"

changelog:
  disable: true

release:
  # The repository is private and has no remote yet. Publishing is a follow-up bean.
  disable: true
```

- [ ] **Step 4: Check it with a snapshot build**

Run: `just snapshot && ls dist/*.tar.gz dist/*.zip && ./dist/cdpn_linux_amd64_v1/cdpn --version`
Expected: six archives (linux/darwin/windows × amd64/arm64) plus `checksums.txt`; the binary prints a `-next` snapshot version. If the binary's directory name differs, use `ls dist` to find it. `dist/` must not be committed; Task 19 adds it to `.gitignore`. Run `rm -rf dist` now.

- [ ] **Step 5: Commit**

```bash
git add .goreleaser.yaml
git add -u
git commit -m "build: add goreleaser config with publishing disabled

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 19: Remove the Node prototype, update docs, file follow-ups

**Files:**
- Delete: `bin/cdpn`, `lib/`, `test/`, `package.json`, `pnpm-lock.yaml`
- Modify: `.gitignore`, `README.md`
- Hub: `README.md` (agent docs), new beans, the spec

- [ ] **Step 1: Check coverage against the Node suite**

Run: `grep -hE '^test\(' test/*.test.js | wc -l` (expect 37: 32 CLI + 5 ref) and `go test ./internal/cli/ ./internal/ref/ -v 2>&1 | grep -c '^=== RUN'`.
For every Node `test("…")` in `test/cli-*.test.js`, point to the Go test that covers it (Tasks 14–16). If any is missing, add it before deleting anything.

- [ ] **Step 2: Delete the Node code**

```bash
git rm -r bin/cdpn lib test package.json pnpm-lock.yaml
```

- [ ] **Step 3: Update `.gitignore`** to:

```
/bin/
/dist/
.env
.env.local
.env.*.local
.DS_Store
```

- [ ] **Step 4: Build into `bin/` and check `.envrc` still works**

Run: `just build && ./bin/cdpn --help | head -3`
Expected: the usage header. `.envrc` already has `PATH_add bin`; leave it unchanged.

- [ ] **Step 5: Rewrite `README.md`**

Replace the whole file with:

````markdown
# cdpn

Mirror a local directory to a CodePen 2.0 Pen. A single Go binary.

```sh
cdpn init [PEN]  # set up .env, .envrc and .gitignore here; with PEN, pull it too
cdpn push        # push local files (creates a Pen + writes CODEPEN_ID to .env.local if unset)
cdpn pull        # download the Pen's files (never deletes local files)
cdpn pull <PEN>  # connect this directory to PEN, then pull it
cdpn sync        # push, then watch and push on every change
cdpn info        # Pen details, build status, version count, local changes waiting to push
cdpn open [editor|preview|live]  # open the Pen in the browser (default: editor)
cdpn completion zsh              # shell completion (bash, zsh, fish, powershell)
```

`--force` skips the version check and overwrites edits made in the CodePen editor. With `pull <PEN>` it allows switching away from an already connected Pen.

## Install

```sh
just install   # go install into $GOBIN (needs Go ≥ 1.25)
```

Inside this repo, `just build` puts `cdpn` in `bin/`, which `.envrc` adds to `PATH`.

## Setup

In any folder, run `cdpn init`. It never overwrites anything:

- **`.env`** is created with `CODEPEN_API_KEY="op://Private/CodePen/API Key"` (or the value from `--key`). An existing `.env`, such as a 1Password mount, is left alone.
- **`.envrc`** and **`.gitignore`** only gain the lines they're missing.
- **`direnv allow`** runs if direnv is installed.

Both files are loaded by direnv via `.envrc`, and by `cdpn` itself as a fallback. The shell environment wins, then `.env.local`, then `.env`.

`.env` holds secrets, and cdpn only ever reads it, so it can be a 1Password Environments mount:

```sh
CODEPEN_API_KEY=cp_user_…   # or a ref like "op://Private/CodePen/API Key", resolved with `op read`
```

`.env.local` holds local state:

```sh
CODEPEN_DIR=pen   # what to mirror (default: .)
CODEPEN_ID=…      # written here on first push
```

## Behaviour

- **pull `<PEN>`** takes a Pen id (`01a1…-…` or Classic `PwWmgeN`) or any URL that contains one: the editor URL, a public `/pen/`, `/full/`, `/details/` or embed URL, or a `?template=` URL. It writes the id to `CODEPEN_ID`, so later commands use it. The API only reaches your own Pens, so fork other people's on codepen.io first. Classic Pens can be pulled but not pushed.
- **pull** without `<PEN>` and without `CODEPEN_ID` fails. Files are written atomically, and a remote path that would land outside the directory is refused.
- **push** mirrors local → remote: changed and new files are uploaded, and remote files missing locally are deleted. It refuses to push an empty directory.
- After every push, compiler patches (e.g. `.codepen/pen.config.json`, `tsconfig.json`) are written back locally.
- Empty files are skipped because the API rejects empty content.
- Ignored: `.git`, `node_modules`, `.env*`, `.envrc`, `.DS_Store`.
- In `sync`, a remote edit made while you're watching causes a 409. Run `cdpn pull` (or use `--force`). Besides file events, `sync` rescans every 5 s, so changes the OS failed to report are still pushed.
- Reads are retried on network errors, 429 and 5xx; writes only on 429, since a failed write may already have been applied.
- Exit codes: `0` ok, `1` error, `2` usage error.

See [API_OVERVIEW.md](API_OVERVIEW.md) for what else the API can do.

## Development

```sh
just test       # go test -race -shuffle=on ./...
just lint       # golangci-lint
just snapshot   # cross-compile release archives into dist/ (publishing is disabled)
```

The CLI tests in `internal/cli` run `cli.Run` in-process against an in-memory fake CodePen API (`internal/codepen/codepentest`), in temp directories, with a recording stand-in for `op`, `direnv` and the browser. They need no network or API key. `CODEPEN_API_URL` and `CODEPEN_POLL_MS` point cdpn at another API and speed up build polling.
````

- [ ] **Step 6: Run everything once more**

Run: `go test -race -shuffle=on ./... && just lint && git status --short`
Expected: all `ok`, lint clean, and `git status` shows only the intended deletions and edits (no `bin/` or `dist/`).

- [ ] **Step 7: Commit**

```bash
git add -A
git commit -m "chore: replace node prototype with the go implementation

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

- [ ] **Step 8: Hub follow-ups** (in `/home/nr/sandboxes/codepen-api-prototyping`, hub branch)

1. Load the `cc-skills-golang:golang-how-to` skill and follow its Configure mode to add the always-load Go skills directive to the hub's agent docs (`README.md`, which `CLAUDE.md` and `AGENTS.md` link to).
2. Create the follow-up bean:
   ```bash
   beans create "Publish cdpn: remote, CI, releases" -t task -d "Once the repo has a (private) GitHub remote: CI workflows (go test -race, golangci-lint, govulncheck), a release workflow on v* tags, set release.disable to false in .goreleaser.yaml, and optionally a brew tap. See docs/specs/2026-10-24-go-rewrite-design.md."
   ```
3. If anything in the implementation deviated from the spec, update `docs/specs/2026-10-24-go-rewrite-design.md` to match.
4. Commit only those paths in the hub:
   ```bash
   git -C /home/nr/sandboxes/codepen-api-prototyping add README.md docs/specs tasks
   git -C /home/nr/sandboxes/codepen-api-prototyping commit -m "docs: go skills directive and publishing follow-up (pen-rj8c)

   Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
   ```
   Check `git -C … status` first. Leave unrelated pending hub changes (for example `.beans.yml`, `skills-lock.json`) unstaged.
