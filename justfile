set shell := ["bash", "-euo", "pipefail", "-c"]

version := `git describe --tags --always --dirty 2>/dev/null || echo dev`
ldflags := "-s -w -X github.com/nocksock/codepen-cli/internal/cli.version=" + version

# List recipes
default:
    @just --list

# Build bin/pen (on PATH via .envrc)
build:
    go build -trimpath -ldflags '{{ldflags}}' -o bin/pen ./cmd/pen
    ./bin/pen --version

# Run all tests with the race detector, in random order
test:
    go test -race -shuffle=on ./...

# Lint with golangci-lint (falls back to nix)
lint:
    if command -v golangci-lint >/dev/null; then golangci-lint run ./...; else nix run nixpkgs#golangci-lint -- run ./...; fi

# Scan for known vulnerabilities (same command as CI)
vuln:
    go run golang.org/x/vuln/cmd/govulncheck@v1.9.0 ./...

# Cross-compile all release archives into dist/ without publishing
snapshot:
    if command -v goreleaser >/dev/null; then goreleaser release --snapshot --clean; else nix run nixpkgs#goreleaser -- release --snapshot --clean; fi

# Install pen into $GOBIN
install:
    go install -trimpath -ldflags '{{ldflags}}' ./cmd/pen
