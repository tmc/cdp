package cdpscripttest

import (
	"context"

	"github.com/tmc/cdp/cdpscript"
)

// CDPScriptRunOptions configures RunCDPScript.
type CDPScriptRunOptions struct {
	// Args are exposed to the script as ARG1..ARGN and ARGC.
	Args []string

	// Env is added to the runtime environment before execution.
	Env []string

	// OutputDir sets the output directory for relative artifacts.
	OutputDir string

	// Verbose enables runtime logging.
	Verbose bool
}

// RunCDPScript executes a cdpscript txtar archive through the real runtime
// engine. This keeps fixture execution aligned with cdpscript and cdp run.
func RunCDPScript(ctx context.Context, path string, opts CDPScriptRunOptions) error {
	engineOpts := []cdpscript.Option{
		cdpscript.WithVerbose(opts.Verbose),
		cdpscript.WithEnv(opts.Env...),
	}
	if opts.OutputDir != "" {
		engineOpts = append(engineOpts, cdpscript.WithOutputDir(opts.OutputDir))
	}

	engine := cdpscript.New(engineOpts...)
	return engine.ExecuteTxtar(ctx, path, opts.Args)
}
