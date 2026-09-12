package main

import (
	"context"
	"errors"
	"io"
	"os"
	"os/signal"
	"syscall"

	"github.com/tmc/cdp/cdpscript"
)

type scriptCmd struct {
	stdin  io.Reader
	stdout io.Writer
	stderr io.Writer
}

func newScriptCmd() *scriptCmd {
	return &scriptCmd{
		stdin:  os.Stdin,
		stdout: os.Stdout,
		stderr: os.Stderr,
	}
}

func (c *scriptCmd) run(args []string) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return cdpscript.RunCLI(ctx, args, cdpscript.CLIConfig{
		Command: "run",
		Usage:   "cdp run [options] <script.txtar>",
		Env:     scriptEnvironment(),
		Stdin:   c.stdin,
		Stdout:  c.stdout,
		Stderr:  c.stderr,
	})
}

func scriptEnvironment() []string {
	return os.Environ()
}

func scriptExitCode(err error) int {
	return cdpscript.ExitCode(err)
}

func scriptErrorType(err error) string {
	switch {
	case errors.Is(err, cdpscript.ErrUsage):
		return ErrorTypeUsage
	default:
		return ErrorTypeGeneral
	}
}
