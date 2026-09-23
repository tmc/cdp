package cdpscript

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"time"
)

// CLIConfig configures RunCLI. Zero fields take the defaults noted below.
type CLIConfig struct {
	// Command names the program in flag errors and usage.
	// Empty means "cdpscript".
	Command string

	// Usage is the synopsis printed when no script is given.
	// Empty means Command + " [options] <script.txtar>".
	Usage string

	// Env is the script's initial environment. Nil means os.Environ().
	Env []string

	// Stdin supplies the script when the script path is "-".
	// Nil means os.Stdin.
	Stdin io.Reader

	// Stdout and Stderr receive command output and diagnostics.
	// Nil means os.Stdout and os.Stderr.
	Stdout io.Writer
	Stderr io.Writer
}

// RunCLI runs the shared cdpscript command-line interface with the
// command-line arguments args (not including the program name).
//
// When -h is given, or the script arguments include -h or --help, RunCLI
// prints help and returns flag.ErrHelp. Callers should pass every result,
// including flag.ErrHelp, to ExitCode to get the process exit status.
func RunCLI(ctx context.Context, args []string, cfg CLIConfig) error {
	if cfg.Command == "" {
		cfg.Command = "cdpscript"
	}
	if cfg.Usage == "" {
		cfg.Usage = cfg.Command + " [options] <script.txtar>"
	}
	if cfg.Stdin == nil {
		cfg.Stdin = os.Stdin
	}
	if cfg.Stdout == nil {
		cfg.Stdout = os.Stdout
	}
	if cfg.Stderr == nil {
		cfg.Stderr = os.Stderr
	}
	if cfg.Env == nil {
		cfg.Env = os.Environ()
	}

	fs := flag.NewFlagSet(cfg.Command, flag.ContinueOnError)
	fs.SetOutput(cfg.Stderr)

	var verbose bool
	var output string
	var headless bool
	var timeout time.Duration
	var tabID string
	var port int

	fs.BoolVar(&verbose, "verbose", false, "Enable verbose logging")
	fs.BoolVar(&verbose, "v", false, "Enable verbose logging (short)")
	fs.StringVar(&output, "output", "", "Output directory for artifacts")
	fs.StringVar(&output, "o", "", "Output directory (short)")
	fs.BoolVar(&headless, "headless", false, "Run launched browser headless")
	fs.DurationVar(&timeout, "timeout", 30*time.Second, "Default timeout for browser startup and selector waits")
	fs.StringVar(&tabID, "tab", "", "Connect to existing browser tab by ID (from /json/list)")
	fs.IntVar(&port, "port", 9222, "Chrome remote debugging port")

	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return err
		}
		return fmt.Errorf("%w: %v", ErrUsage, err)
	}
	if fs.NArg() < 1 {
		return fmt.Errorf("%w: usage: %s", ErrUsage, cfg.Usage)
	}

	scriptPath := fs.Arg(0)
	scriptArgs := fs.Args()[1:]
	if helpWanted(scriptArgs) {
		text, err := helpTextForPath(scriptPath, cfg.Stdin)
		if err != nil {
			return err
		}
		if _, err := io.WriteString(cfg.Stdout, text); err != nil {
			return err
		}
		return flag.ErrHelp
	}

	opts := []Option{
		WithVerbose(verbose),
		WithHeadless(headless),
		WithTimeout(timeout),
		WithEnv(cfg.Env...),
		WithStdout(cfg.Stdout),
		WithStderr(cfg.Stderr),
	}
	if output != "" {
		opts = append(opts, WithOutputDir(output))
	}
	if tabID != "" {
		opts = append(opts, WithRemoteTab(tabID, port))
	}

	engine := New(opts...)
	if scriptPath == "-" {
		return engine.ExecuteReader(ctx, "stdin", cfg.Stdin, scriptArgs)
	}
	return engine.ExecuteTxtar(ctx, scriptPath, scriptArgs)
}

// helpWanted reports whether script arguments request script-scoped help.
func helpWanted(args []string) bool {
	for _, arg := range args {
		if arg == "-h" || arg == "--help" {
			return true
		}
	}
	return false
}

// ExitCode maps an error from RunCLI or Engine execution to a process exit
// code. The mapping is part of the script contract:
//
//	0    err is nil or flag.ErrHelp
//	2    err wraps ErrUsage
//	3    err wraps ErrAssertionFailed
//	130  err wraps context.Canceled
//	1    any other error
func ExitCode(err error) int {
	switch {
	case err == nil:
		return 0
	case errors.Is(err, flag.ErrHelp):
		return 0
	case errors.Is(err, ErrUsage):
		return 2
	case errors.Is(err, ErrAssertionFailed):
		return 3
	case errors.Is(err, context.Canceled):
		return 130
	default:
		return 1
	}
}

func helpTextForPath(path string, stdin io.Reader) (string, error) {
	if path != "-" {
		return HelpText(path)
	}
	archive, err := readArchiveReader(stdin)
	if err != nil {
		return "", err
	}
	var text string
	text += "stdin\n\n"
	if archive.Comment != "" {
		text += archive.Comment + "\n\n"
	}
	text += archiveUsage(archive.Comment, "stdin")
	text += "\n"
	return text, nil
}
