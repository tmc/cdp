package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
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

func main() {
	cmd := newScriptCmd()
	if err := cmd.run(os.Args[1:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			os.Exit(0)
		}
		fmt.Fprintf(os.Stderr, "cdpscript: %v\n", err)
		os.Exit(scriptExitCode(err))
	}
}

func (c *scriptCmd) run(args []string) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return cdpscript.RunCLI(ctx, args, cdpscript.CLIConfig{
		Command: "cdpscript",
		Usage:   "cdpscript [options] <script.txtar>",
		Env:     scriptEnvironment(),
		Stdin:   c.stdin,
		Stdout:  c.stdout,
		Stderr:  c.stderr,
	})
}

func scriptEnvironment() []string {
	return os.Environ()
}

func scriptHelpWanted(args []string) bool {
	return cdpscript.HelpWanted(args)
}

func scriptExitCode(err error) int {
	return cdpscript.ExitCode(err)
}
