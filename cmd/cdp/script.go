package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/tmc/misc/chrome-to-har/internal/cdpscript/executor"
	"github.com/tmc/misc/chrome-to-har/internal/cdpscript/lexer"
	"github.com/tmc/misc/chrome-to-har/internal/cdpscript/parser"
	"github.com/tmc/misc/chrome-to-har/internal/cdpscript/types"
	"golang.org/x/tools/txtar"
	"gopkg.in/yaml.v3"
)

type scriptCmd struct {
	fs *flag.FlagSet

	// flags
	verbose bool
	output  string
}

func newScriptCmd() *scriptCmd {
	c := &scriptCmd{
		fs: flag.NewFlagSet("script", flag.ExitOnError),
	}
	c.fs.BoolVar(&c.verbose, "verbose", false, "Enable verbose logging")
	c.fs.StringVar(&c.output, "output", "", "Output directory for artifacts")
	return c
}

func (c *scriptCmd) run(args []string) error {
	c.fs.Parse(args)

	if c.fs.NArg() < 1 {
		return fmt.Errorf("script file required")
	}

	scriptPath := c.fs.Arg(0)

	// Read file
	data, err := os.ReadFile(scriptPath)
	if err != nil {
		return fmt.Errorf("failed to read script file: %w", err)
	}

	var script types.Script
	script.Helpers = make(map[string]string)

	// Check if it's a txtar archive
	if filepath.Ext(scriptPath) == ".txtar" || filepath.Ext(scriptPath) == ".ar" {
		archive := txtar.Parse(data)

		// Parse files
		for _, f := range archive.Files {
			if f.Name == "meta.yaml" {
				if err := yaml.Unmarshal(f.Data, &script.Metadata); err != nil {
					return fmt.Errorf("failed to parse meta.yaml: %w", err)
				}
			} else if f.Name == "main.cdp" {
				// Parse main script
				l := lexer.New(string(f.Data))
				p := parser.NewParser(l)
				cmd := p.ParseCommands()
				if len(p.Errors()) > 0 {
					return fmt.Errorf("failed to parse main.cdp: %v", p.Errors())
				}
				script.Main = &types.CommandList{
					Commands: cmd,
				}
			} else {
				// Other files are helpers or assets
				script.Helpers[f.Name] = string(f.Data)
			}
		}
	} else {
		// Single file mode
		l := lexer.New(string(data))
		p := parser.NewParser(l)
		cmd := p.ParseCommands()
		if len(p.Errors()) > 0 {
			return fmt.Errorf("failed to parse script: %v", p.Errors())
		}
		script.Main = &types.CommandList{
			Commands: cmd,
		}
	}

	if script.Main == nil {
		return fmt.Errorf("no main.cdp found (or empty script)")
	}

	// Create executor
	ctx := context.Background()
	opts := []executor.Option{
		executor.WithVerbose(c.verbose),
	}
	if c.output != "" {
		opts = append(opts, executor.WithOutputDir(c.output))
	}

	exec, err := executor.NewExecutor(ctx, &script, opts...)
	if err != nil {
		return fmt.Errorf("failed to create executor: %w", err)
	}

	// Execute
	if err := exec.Execute(); err != nil {
		return fmt.Errorf("script execution failed: %w", err)
	}

	return nil
}
