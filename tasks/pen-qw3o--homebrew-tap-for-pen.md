---
# pen-qw3o
title: Homebrew tap for pen
status: todo
type: task
priority: low
created_at: 2026-10-27T22:13:11Z
updated_at: 2026-10-27T22:13:11Z
---

pen-jngl set up the remote, CI and releases; each tag carries linux/macOS/Windows archives (amd64, arm64) plus checksums.

A tap would need a separate repository (e.g. nocksock/homebrew-tap) and a token with write access to it, plus goreleaser's homebrew_casks section in .goreleaser.yaml (v2 removed brews) or a hand-written formula. The release workflow already has contents: write on this repository only.