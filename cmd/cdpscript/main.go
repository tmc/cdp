package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/tmc/cdp/cdpscript"
)

type scriptCmd struct {
	fs *flag.FlagSet

	verbose bool
	output  string
	tabID   string
	port    int
}

func newScriptCmd() *scriptCmd {
	c := &scriptCmd{
		fs: flag.NewFlagSet("cdpscript", flag.ContinueOnError),
	}
	c.fs.BoolVar(&c.verbose, "verbose", false, "Enable verbose logging")
	c.fs.BoolVar(&c.verbose, "v", false, "Enable verbose logging (short)")
	c.fs.StringVar(&c.output, "output", "", "Output directory for artifacts")
	c.fs.StringVar(&c.output, "o", "", "Output directory (short)")
	c.fs.StringVar(&c.tabID, "tab", "", "Connect to existing browser tab by ID (from /json/list)")
	c.fs.IntVar(&c.port, "port", 9222, "Chrome remote debugging port")
	return c
}

func main() {
	cmd := newScriptCmd()
	if err := cmd.run(os.Args[1:]); err != nil {
		if err == flag.ErrHelp {
			return
		}
		fmt.Fprintf(os.Stderr, "cdpscript: %v\n", err)
		os.Exit(1)
	}
}

func (c *scriptCmd) run(args []string) error {
	if err := c.fs.Parse(args); err != nil {
		return err
	}

	if c.fs.NArg() < 1 {
		return fmt.Errorf("usage: cdpscript [options] <script.txtar>")
	}

	scriptPath := c.fs.Arg(0)

	// Create engine with options
	opts := []cdpscript.Option{
		cdpscript.WithVerbose(c.verbose),
	}
	if c.output != "" {
		opts = append(opts, cdpscript.WithOutputDir(c.output))
	}
	if c.tabID != "" {
		opts = append(opts, cdpscript.WithRemoteTab(c.tabID, c.port))
	}

	engine := cdpscript.New(opts...)

	// Execute
	ctx := context.Background()
	return engine.ExecuteTxtar(ctx, scriptPath)
}
