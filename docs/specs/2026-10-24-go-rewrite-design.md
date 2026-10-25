# cdpn: rewrite in Go — design

Bean: `pen-rj8c`. Branch: `task/pen-rj8c`. Date: 2026-10-24.

## Goal

Replace the Node prototype of `cdpn` (on `main`: `bin/cdpn`, `lib/*.js`) with a Go
implementation that behaves the same, plus the robustness the Go spike
(`~/studies/cli-tool-writing/impl/go-fsnotify`) showed to be cheap: atomic writes,
retries, and a watcher that cannot silently go deaf. The spike targets a different mock
API, so only its techniques carry over, not its code.

## Decisions

| Topic | Decision |
|---|---|
| Scope | Node parity + spike hardening. One intentional change: usage errors exit **2** |
| Node code | Replaced in place; deleted once the Go suite covers it |
| Module | `github.com/nocksock/codepen-cli` (repo stays private), binary `cdpn` |
| Architecture | Small, focused packages under `internal/`; no hexagonal ports |
| DI | Manual constructors, wired in `internal/cli` |
| CLI | Cobra, no Viper (config is env files + process env) |
| Tests | In-process `cli.Run` against an `httptest` fake API; one binary smoke test |
| Release | `.goreleaser.yaml` with publishing disabled; snapshot builds only |
| Go | `go 1.25` (needs `os.Root.MkdirAll` / `Rename`) |

Dependencies: `github.com/spf13/cobra`, `github.com/fsnotify/fsnotify`,
`golang.org/x/sync` (errgroup), `go.uber.org/goleak` (tests only). Nothing else.

## Behaviour (unchanged from Node unless marked)

```
cdpn init [PEN] [--key K]   scaffold .env, .envrc, .gitignore; direnv allow; PEN → pull
cdpn push [--force]         push local files; creates a Pen if CODEPEN_ID is unset
cdpn pull [PEN] [--force]   download the Pen's files; PEN (id or URL) connects to it
cdpn sync [--force]         push, then watch and push on every change
cdpn info                   Pen details and local sync status
cdpn open [editor|preview|live]
cdpn completion …           new, free with Cobra
cdpn --version              new, from ldflags / debug.ReadBuildInfo
```

- Config precedence: process env > `.env.local` > `.env`. `.env` is only ever read, and
  read exactly once (it may be a 1Password Environments FIFO). cdpn writes only
  `.env.local` (`CODEPEN_ID`), mode `0600` when created.
- `CODEPEN_API_KEY` may be an `op://` ref, resolved with `op read`. It is resolved only when
  a command builds an API client, so `--help` and `init` without PEN never run `op`.
- `CODEPEN_DIR` (default `.`), `CODEPEN_ID`, and the test overrides `CODEPEN_API_URL`,
  `CODEPEN_POLL_MS` keep their meaning.
- Progress on stdout, `✗ <explanation>` on stderr. Exit 0 ok, 1 runtime/API error,
  **2 usage error** (no/unknown command, bad flags/args) — changed from Node's 1.
- `--help` prints usage and exits 0. Help text is Cobra's, with the env vars listed in the
  root command's long description.
- push: mirrors local → remote (uploads changed/new, deletes remote-only), refuses an
  empty directory when connected, skips empty files, sends `expected_version_id` unless
  `--force`, follows the build to completion, writes compiler patches back locally.
- pull: never deletes local files; switching to another Pen needs `--force`; without PEN
  and without `CODEPEN_ID` it fails.
- Ignored: `.git`, `node_modules`, `.env`, `.env.*`, `.envrc`, `.DS_Store` (any segment).
- Error hints: 409 → "the Pen changed remotely (editor?). run `cdpn pull` first, or push
  with --force"; 404 → "Pen not found. The API only reaches your own Pens (fork someone
  else's on codepen.io first)".

## Layout

```
cmd/cdpn/main.go       signal.NotifyContext(SIGINT, SIGTERM) → os.Exit(cli.Run(ctx, os.Args[1:], cli.System()))
internal/
  cli/                 Cobra commands, one file each: root, push, pull, sync, info, open, init
                       Env, explain(err), exit codes, render(pen.Event)
  pen/                 Mirror: a local directory mirrored to a Pen
  info/                Gather + Render for `cdpn info`
  setup/               Scaffold + AllowDirenv for `cdpn init`
  codepen/             CodePen REST API v2 client (leaf adapter)
  codepen/codepentest/ httptest fake API, port of test/support/fake-api.js
  tree/                local directory ↔ Snapshot
  diff/                local Snapshot vs remote files → ChangeSet (pure)
  ref/                 anything pointing at a Pen → its id (pure)
  config/              cdpn settings from env files + process env
  envfile/             dotenv read/parse and Set
  secret/              op:// resolution
  dotfile/             non-destructive writes to line-based files
  watch/               recursive fsnotify watcher with debounce and rescan
  human/               Bytes, Ago, Plural, Table
  browser/             opener command per GOOS
```

Dependencies point downward only: `cli` → `pen`, `info`, `setup` → building blocks.
Building blocks do not import each other, except `diff` using `tree.Snapshot` and
`codepen` types, and `config` using `envfile` and `ref`.

Rules:
- `pen` never prints and never touches env files. Progress goes through a one-method
  `Reporter`; a new Pen id goes through `Config.SaveID`.
- Interfaces are declared by their consumer, only where tests need a seam (`Runner`,
  `Reporter`). Constructors return concrete types.
- No globals, no `init()`. Every path resolves against `Env.Dir`; nothing calls
  `os.Chdir` or `os.Setenv`.
- `context.Context` flows from `main` into every HTTP request and `exec`.
- Style: early returns, named composite literals, ≤4 params (else an options struct),
  lowercase `%w`-wrapped errors with `%q` for paths, initialised maps/slices, unexport by
  default. Each error is handled once, in `cli.Run`.

## Packages

### cli

```go
type Env struct {
    Dir    string                          // working directory
    Getenv func(string) (string, bool)     // process env
    Stdout io.Writer
    Stderr io.Writer
    Runner Runner                          // op, direnv, browser
    GOOS   string
}
func System() Env                          // real os values, exec-based Runner
func Run(ctx context.Context, args []string, env Env) int   // exit code
```

`Runner` (declared in `cli`, satisfied by the exec implementation and the test recorder):

```go
type Runner interface {
    Output(ctx context.Context, name string, args ...string) ([]byte, error) // op read, direnv allow
    Start(name string, args ...string) error                                 // browser, not awaited
}
```

`secret` and `setup` declare the subset they use (`Output` only).

Commands build their dependencies explicitly: `config.Load(env.Dir, env.Getenv)` →
`secret.Resolve` → `codepen.New(key, opts…)` → `pen.Open(pen.Config{…})`.
`Root.SilenceUsage` and `SilenceErrors` are set; `Run` maps errors: Cobra arg/flag errors
and unknown commands → print usage, exit 2; everything else → `✗ explain(err)`, exit 1.

`render(pen.Event) string` produces the exact Node lines: `  ~ /path`, `  - /path`,
`  ~ /path (binary)`, `  patched <type> <path>`, `  ✗ <path>:<line> <message>`,
`  build <status>`, `  ↓ /path`, `created <editor url>`, `connected <editor url>`.

### pen

```go
type Config struct {
    API      *codepen.Client
    Root     string                 // absolute dir to mirror
    ID       string                 // "" when not connected
    SaveID   func(id string) error
    Reporter Reporter
}
type Reporter interface{ Report(Event) }
type Event interface{ event() }     // Planned{ChangeSet}, Built{Build}, Downloaded{Path},
                                    // Created{URLs}, Connected{URLs}

func Open(cfg Config) *Mirror
func (m *Mirror) ID() string
func (m *Mirror) URLs() codepen.URLs
func (m *Mirror) Push(ctx context.Context, opts PushOptions) error   // PushOptions{Force bool}
func (m *Mirror) Pull(ctx context.Context, opts PullOptions) error   // PullOptions{Ref string; Force bool}
```

`Mirror` keeps the tracked head (`version id`, files by path, urls) between calls so
`sync` only talks to the API when something changed. Not safe for concurrent use; `sync`
calls it from a single goroutine.

- `Push`: no id → `create`; head unknown → `fetch` (pen + files, concurrently);
  `tree.Read`; refuse empty; `diff.Between`; nothing to do → return; `Report(Planned)`;
  `api.WriteFiles` (expected version unless `Force`); `settle`; `writeBack`.
- `create`: `tree.Read`; `api.CreatePen` with title = dir basename and the files (none →
  CodePen starter files); `SaveID`; `Report(Created)`; `settle`; `writeBack`.
- `settle`: `api.Settle` → `Report(Built)` → `api.Version(build.VersionID)` → track it.
  Build errors are reported, not returned.
- `Pull`: resolve id (`ref.Parse(opts.Ref)` or tracked id, else error); switching without
  `Force` → error; `fetch`; id changed → `SaveID` + `Report(Connected)`; `writeBack`
  against the current local snapshot (missing dir = empty).
- `writeBack`: every remote file not `diff.IsSame` as local is written: text content
  directly, binary via `api.Download` streamed into `tree.Write`. `Report(Downloaded)`.
  Never deletes.

### codepen

```go
func New(key string, opts ...Option) *Client   // WithBaseURL, WithPollInterval, WithHTTPClient
func (c *Client) Pen(ctx, id) (Pen, error)
func (c *Client) Files(ctx, id) ([]File, error)
func (c *Client) Version(ctx, id) (Version, error)
func (c *Client) Build(ctx, id) (Build, error)
func (c *Client) CountVersions(ctx, penID) (int, error)       // per_page=1 + Link rel="last"
func (c *Client) CreatePen(ctx, NewPen) (Created, error)      // NewPen{Title; Changes ChangeSet}
func (c *Client) WriteFiles(ctx, id string, cs ChangeSet, expected string) (Build, error)
func (c *Client) Download(ctx, url string) (io.ReadCloser, error) // no Authorization header
func (c *Client) Settle(ctx, Build) (Build, error)            // Queued/Running → poll; Canceled+next_id → follow

type ChangeSet struct { Writes []Write; Deletes []string; Uploads []Upload }
func (cs ChangeSet) IsEmpty() bool
type File struct { ID, Path, Name string; Content *string; ContentHash, URL string; IsBinary bool }
type APIError struct { Status int; Code, Message string }   // Is(target) matches ErrConflict / ErrNotFound / ErrUnauthenticated by Code
```

All wire structs carry `json` tags. Text-only change sets go as JSON; any upload switches
to multipart (`pen` or `changes` JSON part, `expected_version_id` part, one file part per
path), as in Node.

Transport: `http.Client{Timeout: 30s}`, every request built with ctx. Retries: GET on
network error / 429 / 5xx; POST and PUT only on 429 (a 5xx may already have applied).
Up to 4 retries, backoff `200ms·2ⁿ`, `Retry-After` honoured, ctx checked between
attempts via `select` on `ctx.Done()` and a timer.

### codepentest

Port of `fake-api.js`: in-memory Pens and versions, starter file on empty create, config
patch on first build (`PatchConfig`), `QueueBuilds` (Queued, then Success on GET),
409 on a stale `expected_version_id`, `Link` header for versions, JSON + multipart
parsing, `/raw/{pen}/{file}` downloads, 401 on a wrong key, request log.

```go
func Start(t testing.TB) *Server          // closes on t.Cleanup
func (s *Server) URL() string
func (s *Server) Seed(files map[string][]byte, meta PenMeta) string  // binary when not valid UTF-8 text
func (s *Server) Edit(id string, files map[string]string)
func (s *Server) Files(id string) map[string]string
func (s *Server) Calls(pattern string) []Request
const Key = "cp_user_test"
```

### tree

```go
type Snapshot map[string][]byte                  // "/src/app.js" → content
func Read(root string) (Snapshot, error)         // skips ignored paths and non-regular files
func Write(root, path string, r io.Reader) error // os.Root: MkdirAll + <name>.cdpn-tmp + Rename
func IsIgnored(path string) bool
func IsText(b []byte) bool                       // no NUL and utf8.Valid
```

`Write` opens `os.OpenRoot(root)` and does everything through it, so a remote path such
as `/../../.bashrc` cannot escape; it returns an error naming the path.

### diff

```go
func Between(local tree.Snapshot, remote map[string]codepen.File) codepen.ChangeSet
func IsSame(local []byte, remote codepen.File) bool    // text: string equality; binary: sha256 hex
func ByPath(files []codepen.File) map[string]codepen.File
```

Empty local files are skipped. Text → `Writes`, non-text → `Uploads`, remote-only →
`Deletes`. Each list is sorted by path, so output and requests are deterministic (Go maps
are unordered); `writeBack` likewise walks remote paths in sorted order.

### ref, config, envfile, secret, dotfile, setup, info, human, browser

- `ref.Parse(s string) (string, error)` — port of `ref.js` (UUID, Classic id, view
  segments, `?template=`, scheme-less URLs). Regexps compiled at package level.
- `config.Load(dir string, getenv func(string) (string, bool)) (Config, error)` →
  `Config{KeyRef, Dir, PenID, APIURL, PollInterval}`; `Config.Key()` errors with
  "CODEPEN_API_KEY is not set (check .env.local / .env)" when empty. `PenID` is passed
  through `ref.Parse`.
- `envfile.Read(path) (map[string]string, error)` — `KEY=value`, `export KEY=…`, single
  and double quotes (with `\n` escapes in double quotes), `#` comments, blank lines;
  missing file = empty map. `envfile.Set(path, key, value) error` replaces the line or
  appends it, mode `0600` on create.
- `secret.Resolve(ctx, ref string, run outputter) (string, error)` — `op://…` →
  `op read <ref>`, trimmed; anything else is returned as is.
- `dotfile.Ensure(path, content) (Status, error)`, `dotfile.EnsureLines(path, lines)
  (Status, error)` with `Status` = `Created | Updated | Kept` (zero value `Unknown`).
- `setup.Scaffold(dir, key string) ([]Result, error)` and
  `setup.AllowDirenv(ctx, run) bool`; `DefaultKey = "op://Private/CodePen/API Key"`.
- `info.Gather(ctx, api, id, root) (Status, error)` fetches Pen, files and version
  count with an errgroup, then reads the local snapshot (unreadable = empty);
  `info.Render(Status, now time.Time) string` — same rows and wording as `info.js`.
- `human.Bytes`, `human.Ago(t, now)`, `human.Plural(n, word)`, `human.Table(rows)`.
- `browser.Command(goos, url string) (string, []string)` — `open` / `cmd /c start ""` /
  `xdg-open`. A failed launch prints "could not launch a browser; open <url>".

### watch

```go
type Options struct {
    Skip     func(path string) bool  // tree.IsIgnored on "/"-relative paths
    Debounce time.Duration           // default 400ms
    Rescan   time.Duration           // default 5s
}
func Changes(ctx context.Context, dir string, opts Options) (<-chan struct{}, error)
```

- Adds every non-skipped directory to fsnotify; a `Create` that is a directory adds its
  subtree immediately.
- Events only arm a debounce timer (`time.NewTimer` + `Reset`). When it fires, one
  non-blocking send on a channel with buffer 1: bursts collapse, nothing is lost.
- Every `Rescan`, compares a fingerprint (paths, sizes, mtimes of files; no content
  hashing) with the previous one and signals only on change. Runtime watcher errors
  (overflow, a failed watch) leave the rescan to catch up instead of failing. A failure
  to watch at startup (e.g. `ENOSPC`, the inotify limit) is returned as an error, so the
  user can act on it.
- One goroutine owns and closes the channel when ctx is done.

### sync (in cli)

Push once (errors printed, not fatal), print `editor`/`preview` URLs and
`watching <dir>`, then `for range changes { push }` with errors printed as `✗ …`.
ctx cancelled (SIGINT/SIGTERM) → loop ends → exit 0.

## Testing

`go test -race -shuffle=on ./...`. Standard library assertions, table-driven tests with
named lowercase subtests, one `_test.go` per source file, `t.Parallel()` wherever there is
no shared state.

- `internal/cli`: all 30 Node CLI scenarios (basics, init, push/pull, sync, info/open),
  in-process. A `workspace(t, files)` helper gives a `t.TempDir`, a map `Getenv`
  (`CODEPEN_API_KEY`, `CODEPEN_API_URL`, `CODEPEN_POLL_MS=5`), stdout/stderr buffers and a
  recording `Runner` (replaces the PATH stubs; can simulate "direnv not installed").
  Sync tests run `Run` in a goroutine, poll output, then cancel ctx. goleak in `TestMain`.
- Unit tests: `ref` (all `ref.test.js` cases + `FuzzParse`), `diff`, `tree` (ignore,
  text detection, atomic write, traversal rejected), `envfile`, `dotfile`, `human`,
  `browser`, `codepen` (retry policy per method, multipart shape, Settle following
  `next_id`), `watch` (new file, new subdirectory, ignored path, rescan, goleak).
- `cmd/cdpn`: smoke test builds the binary once; `--help` → 0, unknown command → 2,
  missing key → 1.

## Tooling and release

- `justfile`: `build` (→ `bin/cdpn`, version ldflags), `test`, `lint`
  (golangci-lint v2, small `.golangci.yml`), `snapshot`
  (`goreleaser release --snapshot --clean`), `install` (`go install ./cmd/cdpn`).
- `.envrc` keeps `PATH_add bin`; `.gitignore` adds `bin/`, `dist/`.
- `.goreleaser.yaml` (v2): `cmd/cdpn`, `CGO_ENABLED=0`, linux/darwin/windows ×
  amd64/arm64, `-s -w -X github.com/nocksock/codepen-cli/internal/cli.version={{.Version}}`,
  tar.gz (zip on windows), checksums, **`release.disable: true`** while the repo is private.

## Migration

1. Go module, packages and tests alongside the Node code.
2. When the Go CLI suite covers all 30 Node scenarios: delete `bin/cdpn`, `lib/`,
   `test/`, `package.json`, `pnpm-lock.yaml`. Keep `pen/` and `API_OVERVIEW.md`.
3. README: install, Tests and Development sections for Go; Behaviour updated (retries,
   atomic writes, exit 2, `completion`, `--version`).
4. Hub agent docs: a "Required Go skills" note (golang-how-to convention).

## Out of scope / follow-up beans

- Publishing: GitHub remote, CI (test, lint, govulncheck), release workflow, brew tap.
- Watching the remote side (pull on remote edits).

## Changes made during implementation

Review findings that changed behaviour or API compared to the sections above:

- **Ignored paths go neither way.** `tree.Write` refuses ignored paths (`.git`, `.env*`, `.envrc`, `node_modules`, …), so a Pen can't plant `.git/hooks` or replace `.env`. `pen.writeBack` skips them on pull, and `diff.Between` never plans a remote delete for them.
- **Stricter `tree`:**
  - `cleanName` refuses `//`, `\`, NUL, a trailing `/`, `/.` and anything outside the root.
  - `Read` goes through `os.Root`, so a symlinked root is followed.
  - Temp files are created with `O_EXCL`.
  - A replaced file keeps its mode.
- **`dotfile.Ensure` uses `O_EXCL`**, so it never follows a dangling symlink or races another creator.
- **`envfile` parses like Node's `util.parseEnv`:** quoted values may span lines, and an unquoted value ends at `#`. `Set` refuses values containing line breaks.
- **`human` rounds like JS** (`toFixed`, `Math.round` on negative halves).
- **`ref` reads URL paths percent-encoded**, as the WHATWG `URL.pathname` does.
- **`codepen` client:**
  - A huge `Retry-After` can't overflow.
  - ctx errors are wrapped with the request.
  - `Download` errors drop the URL's query and userinfo.
  - `CreatePen` sends `files: []` for upload-only trees, as Node does.
- **`pen` safety:**
  - `Created` is reported before `SaveID`, and a failed save names the new Pen's id.
  - `Pull` restores its state if the fetch or `SaveID` fails.
- **`watch`:**
  - The root is resolved with `EvalSymlinks`, so a symlinked root no longer makes the watcher deaf.
  - A vanished directory doesn't abort the walk.
  - Watcher errors re-add the tree.
- **`cli`:**
  - `init` gained `--force`; Node passed it through to pull.
  - The browser is started in its own session (`Setsid`), like Node's `detached: true`.
  - `exitCode` is named `report`.
- **Lint policy:** errcheck ignores terminal output, closes of read-only resources, and test files. Closes after writes are checked in code.
- **Dependencies:** `golang.org/x/sync` v0.20.0 and `golang.org/x/sys` v0.47.0, the newest versions that need only Go 1.25.
- **Final review:**
  - Ignored names match regardless of letter case and trailing dots or spaces, because case-insensitive filesystems resolve `/.GIT/config` to `.git/config`.
  - `browser.Command` opens only http(s) URLs and returns an error otherwise. On Windows it uses `rundll32 url.dll,FileProtocolHandler`, not `cmd /c start`.
  - Bad `completion` arguments exit 2.
  - `sync` stays quiet when interrupted during its first push, and a second Ctrl-C force-quits.
- **Smaller API differences from the sections above:**
  - `cli.System()` returns `(Env, error)`.
  - `dotfile.Ensure` takes a `perm`.
  - `codepen.WithBackoff` exists, for tests.
  - Root help is the Node-style usage text; subcommands use Cobra's help.
  - Usage errors, including `open <unknown target>`, print the usage text to stderr and exit 2.
