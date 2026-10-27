---
# pen-6saz
title: 'pen: hardening follow-ups from the final review'
status: completed
type: task
priority: normal
created_at: 2026-10-25T14:21:57Z
updated_at: 2026-10-27T15:58:23Z
---

Minor items from the final review of pen-rj8c:
- [x] CreatePen: send files: [] when the local tree has only empty files, as Node did, instead of getting starter files.
- [x] Download: no 30s total timeout for large bodies (use ResponseHeaderTimeout). Optionally require http(s) and cap the size.
- [x] Strip or quote control characters in remote strings (paths, titles, build messages) before printing them.
- [x] Use the same 'takes no arguments' wording for all commands that take none.
- [x] info/open: check for a connected Pen (CODEPEN_ID or .codepen.config.json) before resolving the key, which avoids a pointless 1Password prompt.
- [x] .golangci.yml: (io.Closer).Close exclusion is broad; narrow it if closes after writes appear.

## Outcome

- CreatePen takes an explicit `Starter` flag; the mirror sets it only for an empty directory, so a tree of empty files sends `files: []`.
- Downloads use their own client: no total timeout, 30s `ResponseHeaderTimeout`; only http(s) URLs; a 100 MiB cap (`WithMaxDownload`) that fails instead of truncating.
- `human.Clean` escapes C0/C1 and bidi controls and replaces invalid UTF-8; used for rendered events, the info table, open/sync URLs and error output (line by line).
- push/sync/info and `completion <shell>` say "pen X takes no arguments"; `completion bogus` says "unknown shell".
- `connected()` loads the config and checks for a Pen before resolving the key.
- The `(io.Closer).Close` exclusion was not needed by any code and is removed.

