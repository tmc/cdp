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
	"time"

	"github.com/tmc/cdp/cdpscript"
)

type scriptCmd struct {
	fs *flag.FlagSet

	verbose  bool
	output   string
	headless bool
	timeout  time.Duration
	tabID    string
	port     int
	stdout   io.Writer
	stderr   io.Writer
}

func newScriptCmd() *scriptCmd {
	c := &scriptCmd{
		fs:     flag.NewFlagSet("cdpscript", flag.ContinueOnError),
		stdout: os.Stdout,
		stderr: os.Stderr,
	}
	c.fs.SetOutput(c.stderr)
	c.fs.BoolVar(&c.verbose, "verbose", false, "Enable verbose logging")
	c.fs.BoolVar(&c.verbose, "v", false, "Enable verbose logging (short)")
	c.fs.StringVar(&c.output, "output", "", "Output directory for artifacts")
	c.fs.StringVar(&c.output, "o", "", "Output directory (short)")
	c.fs.BoolVar(&c.headless, "headless", false, "Run launched browser headless")
	c.fs.DurationVar(&c.timeout, "timeout", 30*time.Second, "Default timeout for browser startup and selector waits")
	c.fs.StringVar(&c.tabID, "tab", "", "Connect to existing browser tab by ID (from /json/list)")
	c.fs.IntVar(&c.port, "port", 9222, "Chrome remote debugging port")
	return c
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
	if err := c.fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return err
		}
		return fmt.Errorf("%w: %v", cdpscript.ErrUsage, err)
	}

	if c.fs.NArg() < 1 {
		return fmt.Errorf("%w: usage: cdpscript [options] <script.txtar>", cdpscript.ErrUsage)
	}

	scriptPath := c.fs.Arg(0)
	scriptArgs := c.fs.Args()[1:]
	if scriptHelpWanted(scriptArgs) {
		text, err := cdpscript.HelpText(scriptPath)
		if err != nil {
			return err
		}
		if _, err := io.WriteString(c.stdout, text); err != nil {
			return err
		}
		return flag.ErrHelp
	}

	// Create engine with options
	opts := []cdpscript.Option{
		cdpscript.WithVerbose(c.verbose),
		cdpscript.WithHeadless(c.headless),
		cdpscript.WithTimeout(c.timeout),
		cdpscript.WithEnv(scriptEnvironment()...),
	}
	if c.output != "" {
		opts = append(opts, cdpscript.WithOutputDir(c.output))
	}
	if c.tabID != "" {
		opts = append(opts, cdpscript.WithRemoteTab(c.tabID, c.port))
	}

	engine := cdpscript.New(opts...)

	// Execute
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return engine.ExecuteTxtar(ctx, scriptPath, scriptArgs)
}

func scriptEnvironment() []string {
	return os.Environ()
}

func scriptHelpWanted(args []string) bool {
	for _, arg := range args {
		if arg == "-h" || arg == "--help" {
			return true
		}
	}
	return false
}

func scriptExitCode(err error) int {
	switch {
	case err == nil:
		return 0
	case errors.Is(err, flag.ErrHelp):
		return 0
	case errors.Is(err, cdpscript.ErrUsage):
		return 2
	case errors.Is(err, cdpscript.ErrAssertionFailed):
		return 3
	case errors.Is(err, context.Canceled):
		return 130
	default:
		return 1
	}
}
