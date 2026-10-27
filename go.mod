module github.com/nocksock/codepen-cli

go 1.26.0

// Pinned so CI, releases and govulncheck all run the same toolchain. 1.25 no
// longer receives standard-library fixes, and govulncheck fails on it.
toolchain go1.26.10

require (
	github.com/fsnotify/fsnotify v1.10.1
	github.com/spf13/cobra v1.10.2
	go.uber.org/goleak v1.3.0
	golang.org/x/sync v0.20.0
)

require (
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/spf13/pflag v1.0.9 // indirect
	golang.org/x/sys v0.47.0 // indirect
)
