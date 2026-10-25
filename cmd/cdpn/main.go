// Command cdpn mirrors a local directory to a CodePen 2.0 Pen.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/nocksock/codepen-cli/internal/cli"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	go func() {
		<-ctx.Done()
		stop() // a second Ctrl-C now kills the process instead of waiting for shutdown
	}()
	env, err := cli.System()
	if err != nil {
		fmt.Fprintf(os.Stderr, "✗ %v\n", err)
		os.Exit(1)
	}
	code := cli.Run(ctx, os.Args[1:], env)
	stop()
	os.Exit(code)
}
