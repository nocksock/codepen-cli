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
cdpn --version
```

`--force` skips the version check and overwrites edits made in the CodePen editor. With `pull <PEN>` and `init <PEN>` it allows switching away from an already connected Pen.

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
- Ignored, in any letter case: `.git`, `node_modules`, `.env`, `.env.*`, `.envrc`, `.DS_Store`. Ignored paths are never pushed, never written by pull (so a Pen can't plant `.git/hooks` or replace `.env`), and never deleted remotely.
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
