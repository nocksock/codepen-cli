set shell := ["bash", "-cu"]

version := `git describe --tags --always --dirty 2>/dev/null || echo dev`
ldflags := "-s -w -X github.com/nocksock/codepen-cli/internal/cli.version=" + version

# List recipes
default:
    @just --list

# Build bin/cdpn (on PATH via .envrc)
build:
    go build -trimpath -ldflags '{{ldflags}}' -o bin/cdpn ./cmd/cdpn

# Run all tests with the race detector, in random order
test:
    go test -race -shuffle=on ./...

# Lint with golangci-lint (falls back to nix)
lint:
    if command -v golangci-lint >/dev/null; then golangci-lint run ./...; else nix run nixpkgs#golangci-lint -- run ./...; fi

# Cross-compile all release archives into dist/ without publishing
snapshot:
    if command -v goreleaser >/dev/null; then goreleaser release --snapshot --clean; else nix run nixpkgs#goreleaser -- release --snapshot --clean; fi

# Install cdpn into $GOBIN
install:
    go install -trimpath -ldflags '{{ldflags}}' ./cmd/cdpn
