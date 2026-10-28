# pen

Edit a CodePen in your own editor. `pen` mirrors a local folder to a [CodePen 2.0](https://codepen.io) Pen, so you can work with your editor, git and tools while CodePen builds and hosts the result.

```console
$ pen push
  + /index.html
  + /style.css
  + /script.js
created "my-demo"
  editor   https://codepen.io/editor/you/pen/01J…
  preview  https://8f3c….codepen.dev/

$ pen sync
already up to date
watching .
12:03:41   ~ /style.css
12:03:42 pushed 1 file to "my-demo"
```

## Install

Download the archive for your system from the [latest release](https://github.com/nocksock/codepen-cli/releases/latest), unpack it, and put `pen` on your `PATH`. Builds exist for Linux, macOS and Windows, on amd64 and arm64.

With Go 1.26 or newer you can install it from source instead:

```sh
go install github.com/nocksock/codepen-cli/cmd/pen@latest
```

## Quick start

1. **Get an API key.** Create one in your [CodePen settings](https://codepen.io/settings/api) <!-- TODO: confirm URL -->. It starts with `cp_user_`.
2. **Hand it to `pen`.** Export it in your shell, or put it in a `.env` file in the folder you run `pen` from:

   ```sh
   CODEPEN_API_KEY=cp_user_…
   ```

   `pen` never uploads `.env` files, but it doesn't touch your `.gitignore` either, so keep `.env` out of git yourself.

3. **Push a folder.** In the folder you want on CodePen, run `pen push`. The first push creates a Pen named after the folder and connects the folder to it. An empty folder gets CodePen's starter files instead.
4. **Keep it in sync.** Run `pen sync` while you work. It pushes every time you save. Stop it with Ctrl-C.

`pen open` shows the Pen in your browser. `pen info` tells you where things stand.

## Work on a Pen you already have

```sh
pen pull https://codepen.io/editor/you/pen/01J…
```

This connects the folder to the Pen and downloads its files. Any id or URL that contains one works: the editor URL, `/pen/`, `/full/`, `/details/` and embed URLs, and Classic ids like `PwWmgeN`.

The API only reaches your own Pens. To work on someone else's, fork it on codepen.io first. Classic Pens can be pulled, but not pushed.

## Editing in both places

You can change files in the CodePen editor and locally. `pen` remembers the files as they were at the last push or pull, so it can tell who changed what.

- **`pen pull`** brings in edits made on CodePen. It updates files you haven't touched locally and keeps your local edits. If a file changed on both sides, it changes nothing and lists the files. `pen pull --force` takes CodePen's version of them.
- **`pen push`** refuses when the Pen changed on CodePen since your last sync, so it can't overwrite edits it hasn't seen. Run `pen pull`, then push again. `pen push --force` overwrites the Pen anyway.
- **`pen sync`** stops pushing on such a change, too. Stop it, run `pen pull`, then start `pen sync` again.

Pull never deletes local files, and push only deletes files on CodePen that you deleted locally.

## Fork

```sh
pen fork                      # copy the Pen, with your unpushed edits, and switch this folder to the copy
pen fork remote               # copy the Pen as it is on CodePen
pen fork <PEN>                # copy another of your Pens into this folder
pen fork --into ../variant    # put the copy in a new folder; this one stays on the original
pen fork --detach             # a fresh Pen with the same files, without CodePen's link to the original
```

Forks count toward CodePen's daily limit on new Pens (3 a day on the free plan).

## Commands

| Command | What it does |
|---|---|
| `pen push` | Upload local changes. Creates a Pen if the folder has none. |
| `pen pull [PEN]` | Download changes from CodePen. With `PEN`, connect the folder to that Pen first. |
| `pen sync` | Push, then push again on every change until you stop it with Ctrl-C. |
| `pen fork [SOURCE]` | Copy a Pen and connect a folder to the copy. |
| `pen info` | Show the Pen's details, build status and local changes waiting to push. |
| `pen open [editor\|preview\|live]` | Open the Pen in your browser (default: editor). |
| `pen completion SHELL` | Print a completion script for bash, zsh, fish or powershell. |

`pen help <command>` or `pen <command> --help` shows a command's options and examples.

## Output

Each changed file gets a line: `+` new, `~` changed, `-` deleted. A summary line follows, such as `pushed 3 files to "my-demo"` or `already up to date`. The editor and preview URLs are printed when a folder connects to a Pen; after that, `pen info` and `pen open` have them.

- `-q`, `--quiet` prints errors only.
- `pen push --dry-run` lists what would change, without changing anything.
- `pen info --json` prints the details as JSON.

When CodePen's build fails, `pen` lists the errors on stderr. Your files are still saved on CodePen, but the command exits with 1. `pen sync` reports the failure and keeps watching.

| Exit code | Meaning |
|---|---|
| `0` | Done. |
| `1` | Something failed, including a build with errors. |
| `2` | The command was used wrongly. The message shows how to use it. |
| `130` | Interrupted with Ctrl-C. |

## Configuration

`pen` reads these settings from the shell environment first, then `.env.local`, then `.env`, both in the folder you run it from.

| Variable | Meaning |
|---|---|
| `CODEPEN_API_KEY` | Your API key. Required. |
| `CODEPEN_DIR` | The folder to mirror, if not the current one. |
| `CODEPEN_ID` | The Pen to use, as an id or URL. Overrides the connected one. |

`pen` doesn't resolve secret-manager references. If your key lives in one, run `pen` through it, e.g. `op run -- pen push`.

The connected Pen is stored as `"id"` in `.codepen.config.json`, at the top of the mirrored folder. Commit it, and every clone knows its Pen. It's never uploaded, and a pull never overwrites it.

What `pen` remembers about the last sync lives outside your project, in `$XDG_STATE_HOME/pen` (usually `~/.local/state/pen`). Moving the folder makes `pen` forget it. The next pull then changes no local file that differs from CodePen unless you pass `--force`.

## What is never synced

`.git`, `node_modules`, `.env`, `.env.*`, `.envrc`, `.DS_Store` and `.codepen.config.json`, in any letter case. These are never uploaded, never written by a pull, and never deleted from CodePen. So a Pen can't plant a git hook or replace your secrets.

Empty files aren't uploaded, since CodePen rejects them. Emptying a file locally deletes it from CodePen on the next push.

After a push, CodePen sometimes adjusts files: it adds a `tsconfig.json` or rewrites `.codepen/pen.config.json`, for example. `pen` writes those changes back to your folder.

## More

- [CONTRIBUTING.md](CONTRIBUTING.md): building, testing and releasing.
- [API_OVERVIEW.md](API_OVERVIEW.md): what else the CodePen API can do.
