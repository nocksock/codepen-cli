---
# pen-jngl
title: 'Publish pen: remote, CI, releases'
status: in-progress
type: task
priority: normal
created_at: 2026-10-25T14:10:41Z
updated_at: 2026-10-27T21:46:55Z
---

Once the repo has a (private) GitHub remote:
- CI workflows: go test -race -shuffle=on, golangci-lint, govulncheck
- a release workflow on v* tags
- set release.disable to false in .goreleaser.yaml
- optionally a brew tap
See docs/specs/2026-10-24-go-rewrite-design.md. Watch out: newer golang.org/x/sync and x/sys versions require Go 1.26.