---
# pen-jngl
title: 'Publish pen: remote, CI, releases'
status: completed
type: task
priority: normal
created_at: 2026-10-25T14:10:41Z
updated_at: 2026-10-27T22:13:23Z
---

Once the repo has a (private) GitHub remote:
- CI workflows: go test -race -shuffle=on, golangci-lint, govulncheck
- a release workflow on v* tags
- set release.disable to false in .goreleaser.yaml
- optionally a brew tap
See docs/specs/2026-10-24-go-rewrite-design.md. Watch out: newer golang.org/x/sync and x/sys versions require Go 1.26.

Done 2026-10-27:
- Remote github.com/nocksock/codepen-cli (private); main pushed, CI green.
- .github/workflows/ci.yml: test, lint + formatting, govulncheck.
- .github/workflows/release.yml: goreleaser on v* tags.
- release.disable removed; notes from the GitHub compare API, grouped by type.
- v0.1.0 published with all six archives and checksums.
- Go moved to 1.26 with a toolchain pin: 1.25 could not pass govulncheck
  (11 reachable standard-library vulnerabilities).
- The brew tap is now pen-qw3o.