package executor

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/chromedp/cdproto/log"
	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/cdproto/runtime"
	"github.com/chromedp/chromedp"
	"github.com/tmc/misc/chrome-to-har/internal/browser"
	"github.com/tmc/misc/chrome-to-har/internal/cdpscript/ast"
	"github.com/tmc/misc/chrome-to-har/internal/cdpscript/types"
	"github.com/tmc/misc/chrome-to-har/internal/chromeprofiles"
	"github.com/tmc/misc/chrome-to-har/internal/recorder"
)

// Executor manages the execution of CDP scripts.
type Executor struct {
	browser     *browser.Browser
	ctx         context.Context
	script      *types.Script
	variables   map[string]interface{}
	outputDir   string
	verbose     bool
	assertions  *AssertionTracker
	console     *ConsoleTracker
	network     *NetworkTracker
	performance *PerformanceTracker
	rec         *recorder.Recorder
}

// NewExecutor creates a new script executor.
func NewExecutor(ctx context.Context, script *types.Script, opts ...Option) (*Executor, error) {
	e := &Executor{
		ctx:         ctx,
		script:      script,
		variables:   make(map[string]interface{}),
		assertions:  NewAssertionTracker(),
		console:     NewConsoleTracker(),
		network:     NewNetworkTracker(),
		performance: NewPerformanceTracker(),
	}

	// Apply options
	for _, opt := range opts {
		opt(e)
	}

	// Initialize variables from script metadata
	for k, v := range script.Metadata.Env {
		e.variables[k] = v
	}

	return e, nil
}

// Option is a functional option for configuring the Executor.
type Option func(*Executor)

// WithOutputDir sets the output directory for generated files.
func WithOutputDir(dir string) Option {
	return func(e *Executor) {
		e.outputDir = dir
	}
}

// WithVerbose enables verbose logging.
func WithVerbose(verbose bool) Option {
	return func(e *Executor) {
		e.verbose = verbose
	}
}

// WithVariables sets additional variables for script execution.
func WithVariables(vars map[string]string) Option {
	return func(e *Executor) {
		for k, v := range vars {
			e.variables[k] = v
		}
	}
}

// Execute runs the CDP script.
func (e *Executor) Execute() error {
	// Initialize browser
	if err := e.initBrowser(); err != nil {
		return fmt.Errorf("failed to initialize browser: %w", err)
	}
	defer e.cleanup()

	// Execute main commands
	if e.script.Main != nil && e.script.Main.Commands != nil {
		commands, ok := e.script.Main.Commands.([]ast.Command)
		if !ok {
			return fmt.Errorf("invalid command list type")
		}

		for i, cmd := range commands {
			if e.verbose {
				fmt.Printf("Executing command %d: %s\n", i+1, cmd.String())
			}

			if err := e.executeCommand(cmd); err != nil {
				return fmt.Errorf("command %d failed: %w", i+1, err)
			}
		}
	}

	// Run assertions if present
	if len(e.script.Assertions) > 0 {
		if err := e.runAssertions(); err != nil {
			return fmt.Errorf("assertions failed: %w", err)
		}
	}

	// Report assertion results
	if e.verbose || e.assertions.FailCount() > 0 {
		e.printAssertionSummary()
	}

	// Fail if any assertions failed
	if e.assertions.FailCount() > 0 {
		return fmt.Errorf("%s", e.assertions.Summary())
	}

	return nil
}

// printAssertionSummary prints a summary of assertion results.
func (e *Executor) printAssertionSummary() {
	fmt.Println("\n" + strings.Repeat("=", 60))
	fmt.Println("Assertion Summary")
	fmt.Println(strings.Repeat("=", 60))

	results := e.assertions.GetResults()
	if len(results) == 0 {
		fmt.Println("No assertions executed")
		return
	}

	passCount := e.assertions.PassCount()
	failCount := e.assertions.FailCount()

	fmt.Printf("Total: %d | Passed: %d | Failed: %d\n\n", len(results), passCount, failCount)

	// Print failed assertions
	if failCount > 0 {
		fmt.Println("Failed Assertions:")
		fmt.Println(strings.Repeat("-", 60))
		for _, result := range results {
			if !result.Passed {
				fmt.Printf("✗ %s\n", result.Name)
				fmt.Printf("  Type: %s\n", result.Type)
				fmt.Printf("  Message: %s\n", result.Message)
				if len(result.Details) > 0 {
					fmt.Printf("  Details: %+v\n", result.Details)
				}
				fmt.Println()
			}
		}
	}

	// Print passed assertions in verbose mode
	if e.verbose && passCount > 0 {
		fmt.Println("Passed Assertions:")
		fmt.Println(strings.Repeat("-", 60))
		for _, result := range results {
			if result.Passed {
				fmt.Printf("✓ %s\n", result.Name)
			}
		}
		fmt.Println()
	}

	fmt.Println(strings.Repeat("=", 60))
}

// initBrowser initializes the browser instance based on script metadata.
func (e *Executor) initBrowser() error {
	// Create profile manager (nil for now, can be configured)
	var profileMgr chromeprofiles.ProfileManager

	// Build browser options from script metadata
	browserOpts := []browser.Option{
		browser.WithHeadless(e.script.Metadata.Headless),
		browser.WithTimeout(int(e.script.Metadata.Timeout.Seconds())),
	}

	if e.verbose {
		browserOpts = append(browserOpts, browser.WithVerbose(true))
	}

	// Create browser instance
	br, err := browser.New(e.ctx, profileMgr, browserOpts...)
	if err != nil {
		return fmt.Errorf("failed to create browser: %w", err)
	}

	// Launch browser
	if err := br.Launch(e.ctx); err != nil {
		return fmt.Errorf("failed to launch browser: %w", err)
	}

	e.browser = br
	e.monitorNetwork()
	e.monitorConsole()
	return nil
}

// cleanup closes the browser and performs cleanup.
func (e *Executor) cleanup() {
	if e.browser != nil {
		e.browser.Close()
	}
}

// executeCommand dispatches a command to the appropriate handler.
func (e *Executor) executeCommand(cmd ast.Command) error {
	switch c := cmd.(type) {
	case *ast.NavigationCommand:
		return e.executeNavigation(c)
	case *ast.WaitCommand:
		return e.executeWait(c)
	case *ast.InteractionCommand:
		return e.executeInteraction(c)
	case *ast.ExtractionCommand:
		return e.executeExtraction(c)
	case *ast.SaveCommand:
		return e.executeSave(c)
	case *ast.AssertionCommand:
		return e.executeAssertion(c)
	case *ast.NetworkCommand:
		return e.executeNetwork(c)
	case *ast.OutputCommand:
		return e.executeOutput(c)
	case *ast.JavaScriptCommand:
		return e.executeJavaScript(c)
	case *ast.ControlFlowCommand:
		return e.executeControlFlow(c)
	case *ast.DebugCommand:
		return e.executeDebug(c)
	case *ast.CompareCommand:
		return e.executeCompare(c)
	default:
		return fmt.Errorf("unknown command type: %T", cmd)
	}
}

// executeNavigation handles navigation commands.
func (e *Executor) executeNavigation(cmd *ast.NavigationCommand) error {
	switch cmd.Type {
	case "goto":
		return e.browser.Navigate(cmd.URL)
	case "back":
		// Use JavaScript to go back
		_, err := e.browser.ExecuteScript("window.history.back()")
		return err
	case "forward":
		// Use JavaScript to go forward
		_, err := e.browser.ExecuteScript("window.history.forward()")
		return err
	case "reload":
		// Use JavaScript to reload
		_, err := e.browser.ExecuteScript("window.location.reload()")
		return err
	default:
		return fmt.Errorf("unknown navigation type: %s", cmd.Type)
	}
}

// executeWait handles wait commands.
func (e *Executor) executeWait(cmd *ast.WaitCommand) error {
	switch cmd.Type {
	case "for":
		// Wait for selector
		timeout := e.script.Metadata.Timeout
		return e.browser.WaitForSelector(cmd.Selector, timeout)
	case "until":
		// Wait for condition (network idle, dom stable, etc.)
		// For now, implement a simple time-based wait
		// TODO: Implement proper condition waiting
		time.Sleep(2 * time.Second)
		return nil
	case "duration":
		// Parse duration and wait
		duration, err := time.ParseDuration(cmd.Duration)
		if err != nil {
			return fmt.Errorf("invalid duration: %w", err)
		}
		time.Sleep(duration)
		return nil
	default:
		return fmt.Errorf("unknown wait type: %s", cmd.Type)
	}
}

// executeInteraction handles interaction commands.
func (e *Executor) executeInteraction(cmd *ast.InteractionCommand) error {
	page := e.browser.GetCurrentPage()
	if page == nil {
		return fmt.Errorf("no active page")
	}

	switch cmd.Type {
	case "click":
		return page.Click(cmd.Selector)
	case "fill", "type":
		// Both fill and type use the Type method
		return page.Type(cmd.Selector, cmd.Value)
	case "hover":
		return page.Hover(cmd.Selector)
	case "press":
		return page.Press(cmd.Value)
	case "select":
		return page.SelectOption(cmd.Selector, cmd.Value)
	case "scroll":
		target := cmd.Target
		if target == "top" {
			_, err := e.browser.ExecuteScript("window.scrollTo(0, 0)")
			return err
		}
		if target == "bottom" {
			_, err := e.browser.ExecuteScript("window.scrollTo(0, document.body.scrollHeight)")
			return err
		}
		// Assume target is a selector
		script := fmt.Sprintf(`document.querySelector("%s").scrollIntoView()`, target)
		_, err := e.browser.ExecuteScript(script)
		return err
	default:
		return fmt.Errorf("unknown interaction type: %s", cmd.Type)
	}
}

// executeExtraction handles extraction commands.
func (e *Executor) executeExtraction(cmd *ast.ExtractionCommand) error {
	page := e.browser.GetCurrentPage()
	if page == nil {
		return fmt.Errorf("no active page")
	}

	var value string
	var err error

	if cmd.Attribute != "" {
		// Extract attribute
		value, err = page.GetAttribute(cmd.Selector, cmd.Attribute)
	} else {
		// Extract text content
		value, err = page.GetText(cmd.Selector)
	}

	if err != nil {
		return fmt.Errorf("extraction failed: %w", err)
	}

	// Store in variables
	e.variables[cmd.Variable] = value

	if e.verbose {
		fmt.Printf("Extracted %s = %s\n", cmd.Variable, value)
	}

	return nil
}

// Note: executeSave, executeAssertion, executeNetwork, executeControlFlow,
// executeCompare, and helper functions are now in commands.go

// executeOutput handles output commands (screenshot, PDF, HAR).
func (e *Executor) executeOutput(cmd *ast.OutputCommand) error {
	page := e.browser.GetCurrentPage()
	if page == nil {
		return fmt.Errorf("no active page")
	}

	switch cmd.Type {
	case "screenshot":
		data, err := page.Screenshot()
		if err != nil {
			return fmt.Errorf("screenshot failed: %w", err)
		}
		return e.saveOutputFile(cmd.Filename, data)
	case "pdf":
		data, err := page.PDF()
		if err != nil {
			return fmt.Errorf("PDF generation failed: %w", err)
		}
		return e.saveOutputFile(cmd.Filename, data)
	case "har":
		if e.rec == nil {
			return fmt.Errorf("HAR recording not enabled (use 'capture network' first)")
		}

		filename := cmd.Filename
		if filename == "" {
			filename = fmt.Sprintf("capture-%d.har", time.Now().Unix())
		}

		outputPath := filename
		if e.outputDir != "" && !filepath.IsAbs(outputPath) {
			outputPath = filepath.Join(e.outputDir, outputPath)
		}

		if err := os.MkdirAll(filepath.Dir(outputPath), 0755); err != nil {
			return fmt.Errorf("failed to create directory: %w", err)
		}

		if err := e.rec.WriteHAR(outputPath); err != nil {
			return fmt.Errorf("HAR export failed: %w", err)
		}

		if e.verbose {
			fmt.Printf("Saved HAR to %s\n", outputPath)
		}
		return nil
	default:
		return fmt.Errorf("unknown output type: %s", cmd.Type)
	}
}

// executeJavaScript handles JavaScript execution.
func (e *Executor) executeJavaScript(cmd *ast.JavaScriptCommand) error {
	var script string

	if cmd.Filename != "" {
		// Load script from helpers
		if helper, ok := e.script.Helpers[cmd.Filename]; ok {
			script = helper
		} else {
			return fmt.Errorf("helper script not found: %s", cmd.Filename)
		}
	} else {
		script = cmd.Code
	}

	result, err := e.browser.ExecuteScript(script)
	if err != nil {
		return fmt.Errorf("JavaScript execution failed: %w", err)
	}

	// Store result in variable if specified
	if cmd.Variable != "" {
		e.variables[cmd.Variable] = result
		if e.verbose {
			fmt.Printf("JavaScript result %s = %v\n", cmd.Variable, result)
		}
	}

	return nil
}

// executeDebug handles debug commands.
func (e *Executor) executeDebug(cmd *ast.DebugCommand) error {
	switch cmd.Type {
	case "log":
		fmt.Println(cmd.Message)
		return nil
	case "debug":
		fmt.Printf("DEBUG: %s\n", cmd.Message)
		return nil
	case "breakpoint":
		fmt.Println("Breakpoint hit - press Enter to continue...")
		fmt.Scanln()
		return nil
	case "devtools":
		fmt.Println("DevTools command not yet implemented")
		return nil
	default:
		return fmt.Errorf("unknown debug type: %s", cmd.Type)
	}
}

// runAssertions runs all script assertions.
func (e *Executor) runAssertions() error {
	// TODO: Implement assertion execution
	return nil
}

func (e *Executor) monitorNetwork() {
	// Enable network events
	go func() {
		if err := chromedp.Run(e.browser.Context(), network.Enable()); err != nil {
			if e.verbose {
				fmt.Printf("Failed to enable network events: %v\n", err)
			}
		}
	}()

	chromedp.ListenTarget(e.browser.Context(), func(ev interface{}) {
		switch ev := ev.(type) {
		case *network.EventResponseReceived:
			e.network.AddRequest(NetworkRequest{
				URL:        ev.Response.URL,
				Status:     int(ev.Response.Status),
				StatusText: ev.Response.StatusText,
				Timestamp:  time.Now(),
			})
		}
	})
}

func (e *Executor) monitorConsole() {
	// Enable runtime and log events
	go func() {
		if err := chromedp.Run(e.browser.Context(), runtime.Enable(), log.Enable()); err != nil {
			if e.verbose {
				fmt.Printf("Failed to enable console events: %v\n", err)
			}
		}
	}()

	chromedp.ListenTarget(e.browser.Context(), func(ev interface{}) {
		switch ev := ev.(type) {
		case *runtime.EventConsoleAPICalled:
			var parts []string
			for _, arg := range ev.Args {
				if len(arg.Value) > 0 {
					parts = append(parts, string(arg.Value))
				} else {
					parts = append(parts, arg.Description)
				}
			}
			msg := strings.Join(parts, " ")

			e.console.AddLog(ConsoleLog{
				Level:     string(ev.Type),
				Message:   msg,
				Timestamp: time.Now(),
				Source:    "console-api",
			})

		case *log.EventEntryAdded:
			e.console.AddLog(ConsoleLog{
				Level:     string(ev.Entry.Level),
				Message:   ev.Entry.Text,
				Timestamp: time.Now(),
				Source:    string(ev.Entry.Source),
			})
		}
	})
}
