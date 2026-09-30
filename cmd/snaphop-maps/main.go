// Command snaphop-maps is SnapHop Maps from the command line, for AI agents. Run it without
// arguments for help, or `snaphop-maps schema` for the same as JSON.
package main

import (
	"context"
	"os"
	"os/signal"
	"runtime/debug"
	"syscall"
	"time"

	"github.com/snaphop/snaphop-maps-cli/internal/cli"
)

// version is set at build time with -ldflags "-X main.version=...".
var version = "dev"

// exit ends the process; tests replace it.
var exit = os.Exit

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	code := cli.Run(ctx, cli.Env{
		Args:      os.Args[1:],
		Stdin:     os.Stdin,
		Stdout:    os.Stdout,
		Stderr:    os.Stderr,
		Getenv:    os.Getenv,
		ConfigDir: os.UserConfigDir,
		Now:       time.Now,
		Version:   cli.Version(version, debug.ReadBuildInfo),
	})
	stop()
	exit(code)
}
