# Contributing

```sh
just build      # build bin/pen (on PATH via .envrc)
just install    # go install into $GOBIN (needs Go ≥ 1.26)
just test       # go test -race -shuffle=on ./...
just lint       # golangci-lint
just vuln       # govulncheck
just snapshot   # cross-compile release archives into dist/ without publishing
```

## Tests

The CLI tests in `internal/cli` run `cli.Run` in-process against an in-memory fake CodePen API (`internal/codepen/codepentest`). They run in temp directories and use a recording stand-in for the browser, so they need no network or API key.

Two environment variables help when trying `pen` by hand:

- `CODEPEN_API_URL` points `pen` at another API, such as the fake one.
- `CODEPEN_POLL_MS` speeds up build polling.

## CI and releases

CI runs on every push to `main` and every pull request. It runs the checks above, plus `golangci-lint fmt --diff` and `goreleaser check`.

Pushing a `v*` tag runs the checks again, and publishes a release only if they pass. Release notes are grouped by conventional-commit type, so write commit messages like `feat(cli): …` or `fix(pen): …`. A `!` marks a breaking change.
