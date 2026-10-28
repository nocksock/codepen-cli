# pen

Mirror a local directory to a CodePen 2.0 Pen. A single Go binary.

```sh
pen init [PEN]  # add env files to .gitignore here; with PEN, pull it too
pen push        # push local files (creates a Pen if none is connected yet)
pen pull        # download the Pen's files (never deletes local files)
pen pull <PEN>  # connect this directory to PEN, then pull it
pen sync        # push, then watch and push on every change
pen info        # Pen details, build status, version count, local changes waiting to push
pen open [editor|preview|live]  # open the Pen in the browser (default: editor)
pen completion zsh              # shell completion (bash, zsh, fish, powershell)
pen --version
```

`--force` skips the version check and overwrites edits made in the CodePen editor. With `pull <PEN>` and `init <PEN>` it allows switching away from an already connected Pen, and replacing local files that differ from the Pen (without it, connecting refuses to replace them).

## Install

```sh
just install   # go install into $GOBIN (needs Go ≥ 1.26)
```

Every tagged release carries archives for linux, macOS and Windows, amd64 and arm64. Inside this repo, `just build` puts `pen` in `bin/`, which `.envrc` adds to `PATH`.

## Setup

In any folder, run `pen init`. It never overwrites anything: **`.gitignore`** gains `.env` and `.env.local` if they're missing. **`.env` and `.envrc`** are yours to create and manage; `pen` never writes them.

`pen` reads `CODEPEN_API_KEY` from the shell environment, then `.env.local`, then `.env`. Use whichever fits: export it, let direnv or a secret manager inject it, or put it in `.env`.

```sh
CODEPEN_API_KEY=cp_user_…
```

`pen` does not resolve secret-manager references. If yours holds `op://…`, run the command through your tool, e.g. `op run -- pen push`.

`.env.local` holds local settings:

```sh
CODEPEN_DIR=pen   # what to mirror (default: .)
```

The connected Pen is kept as `"id"` in `.codepen.config.json` at the top of the mirrored directory, so you can commit it and a clone knows its Pen. The file is local only: it is never pushed, and a pull never writes it, not even when a Pen carries one. A `CODEPEN_ID` environment variable overrides it.

## Behaviour

- **pull `<PEN>`** takes a Pen id (`01a1…-…` or Classic `PwWmgeN`) or any URL that contains one: the editor URL, a public `/pen/`, `/full/`, `/details/` or embed URL, or a `?template=` URL. It stores the id in `.codepen.config.json`, so later commands use it. The API only reaches your own Pens, so fork other people's on codepen.io first. Classic Pens can be pulled but not pushed.
- **pull** without `<PEN>` and without a connected Pen fails. Files are written atomically, and a remote path that would land outside the directory is refused.
- **push** mirrors local → remote: changed and new files are uploaded, and remote files missing locally are deleted. It refuses to push an empty directory.
- After **push**, **pull**, **sync** and **init `<PEN>`**, the Pen's editor and preview URLs are printed, even when nothing changed.
- After every push, compiler patches (e.g. `.codepen/pen.config.json`, `tsconfig.json`) are written back locally.
- Empty files are skipped because the API rejects empty content.
- Ignored, in any letter case: `.git`, `node_modules`, `.env`, `.env.*`, `.envrc`, `.DS_Store`, `.codepen.config.json`. Ignored paths are never pushed, never written by pull (so a Pen can't plant `.git/hooks` or replace `.env`), and never deleted remotely.
- In `sync`, a remote edit made while you're watching causes a 409. Run `pen pull` (or use `--force`). Besides file events, `sync` rescans every 5 s, so changes the OS failed to report are still pushed.
- Reads are retried on network errors, 429 and 5xx; writes only on 429, since a failed write may already have been applied.
- Exit codes: `0` ok, `1` error, `2` usage error, `130` interrupted (Ctrl-C).

See [API_OVERVIEW.md](API_OVERVIEW.md) for what else the API can do.

## Development

```sh
just test       # go test -race -shuffle=on ./...
just lint       # golangci-lint
just vuln       # govulncheck
just snapshot   # cross-compile release archives into dist/ without publishing
```

CI runs these checks, plus `golangci-lint fmt --diff` and `goreleaser check`, on every push to `main` and every pull request. Pushing a `v*` tag runs them again and publishes a release only if they pass.

The CLI tests in `internal/cli` run `cli.Run` in-process against an in-memory fake CodePen API (`internal/codepen/codepentest`), in temp directories, with a recording stand-in for the browser. They need no network or API key. `CODEPEN_API_URL` and `CODEPEN_POLL_MS` point `pen` at another API and speed up build polling.
