# cdpn

Mirror a local directory to a CodePen 2.0 Pen. No dependencies, Node ≥ 22.

```sh
cdpn push    # push local files (creates a Pen + writes CODEPEN_ID to .env if unset)
cdpn pull    # download the Pen's files (never deletes local files)
cdpn sync    # push, then watch and push on every change
```

`--force` skips the version check and overwrites edits made in the CodePen editor.

## Setup

Both files are loaded by direnv via `.envrc`, and by `cdpn` itself as a fallback. `.env.local` wins.

`.env` holds secrets, and cdpn only ever reads it, so it can be a 1Password Environments mount:

```sh
CODEPEN_API_KEY=cp_user_…   # or a ref like "op://Private/CodePen/API Key", resolved with `op read`
```

`.env.local` holds local state:

```sh
CODEPEN_DIR=pen   # what to mirror (default: .)
CODEPEN_ID=…      # written here on first push
```

`.envrc` puts `bin/` on `PATH`. Elsewhere, run `npm link` once.

## Behaviour

- **push** mirrors local → remote: changed and new files are uploaded, and remote files missing locally are deleted. It refuses to push an empty directory.
- After every push, compiler patches (e.g. `.codepen/pen.config.json`, `tsconfig.json`) are written back locally.
- Empty files are skipped because the API rejects empty content.
- Ignored: `.git`, `node_modules`, `.env*`, `.envrc`, `.DS_Store`.
- In `sync`, a remote edit made while you're watching causes a 409. Run `cdpn pull` (or use `--force`).

See [API_OVERVIEW.md](API_OVERVIEW.md) for what else the API can do.
