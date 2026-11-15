package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/networkteam/sdd/internal/cliapp"
	"github.com/networkteam/sdd/internal/cliout"
	"github.com/networkteam/slogutils"
	"github.com/urfave/cli/v3"
)

// version is stamped at release time via -ldflags "-X main.version=...".
var version = "dev"

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	app := cliapp.New(cliapp.Options{Version: version, Reader: os.Stdin, Writer: os.Stdout, ErrWriter: os.Stderr})
	if err := app.Run(ctx, os.Args); err != nil {
		var exit cli.ExitCoder
		if errors.As(err, &exit) && exit.ExitCode() == 0 {
			return
		}
		logger := slog.New(slogutils.NewCLIHandler(os.Stderr, nil))
		if errors.Is(err, cliout.ErrUserCancelled) || errors.Is(err, context.Canceled) {
			fmt.Fprintln(os.Stderr, "cancelled.")
			if uncommitted := cliout.Uncommitted(err); uncommitted != nil {
				logger.Error(uncommitted.Error())
				os.Exit(1)
			}
			os.Exit(130)
		}
		logger.Error(err.Error())
		if exit != nil {
			os.Exit(exit.ExitCode())
		}
		os.Exit(1)
	}
}
