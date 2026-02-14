// Package scriptengine provides a CDP script engine based on rsc.io/script.
package scriptengine

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"
	"github.com/tmc/misc/chrome-to-har/internal/browser"
	"github.com/tmc/misc/chrome-to-har/internal/chromeprofiles"
	"github.com/tmc/misc/chrome-to-har/internal/discovery"
	"github.com/tmc/misc/chrome-to-har/internal/recorder"
	"golang.org/x/tools/txtar"
	"gopkg.in/yaml.v3"
	"rsc.io/script"
)

// Engine executes CDP scripts using the rsc.io/script framework.
type Engine struct {
	engine     *script.Engine
	browser    *browser.Browser
	profileMgr chromeprofiles.ProfileManager
	verbose    bool
	outputDir  string
	metadata   Metadata

	// Remote tab connection
	remoteTabID string
	remotePort  int

	// Sourced commands (dynamically loaded from source command)
	sourcedCmds map[string]script.Cmd
	traceExec   bool // -x flag for source command

	// Element refs from last snapshot (for @e1 style refs)
	refMap *browser.RefMap

	// HAR recorder for capturing network activity with tags
	recorder *recorder.Recorder
}

// Metadata represents script metadata from meta.yaml.
type Metadata struct {
	Name        string            `yaml:"name"`
	Description string            `yaml:"description"`
	Version     string            `yaml:"version"`
	Browser     string            `yaml:"browser"`
	Profile     string            `yaml:"profile"`
	Headless    bool              `yaml:"headless"`
	Timeout     time.Duration     `yaml:"timeout"`
	Env         map[string]string `yaml:"env"`
}

// New creates a new CDP script engine.
func New(opts ...Option) *Engine {
	e := &Engine{
		sourcedCmds: make(map[string]script.Cmd),
	}
	for _, opt := range opts {
		opt(e)
	}

	// Create script engine with CDP commands
	e.engine = &script.Engine{
		Cmds:  e.commands(),
		Conds: e.conditions(),
	}

	return e
}

// Option configures the Engine.
type Option func(*Engine)

// WithVerbose enables verbose output.
func WithVerbose(v bool) Option {
	return func(e *Engine) { e.verbose = v }
}

// WithOutputDir sets the output directory.
func WithOutputDir(dir string) Option {
	return func(e *Engine) { e.outputDir = dir }
}

// WithRemoteTab configures the engine to connect to an existing browser tab.
func WithRemoteTab(tabID string, port int) Option {
	return func(e *Engine) {
		e.remoteTabID = tabID
		e.remotePort = port
	}
}

// WithRecorder sets the recorder for HAR capture with tagging support.
func WithRecorder(rec *recorder.Recorder) Option {
	return func(e *Engine) {
		e.recorder = rec
	}
}

// ExecuteTxtar runs a script from a txtar archive.
func (e *Engine) ExecuteTxtar(ctx context.Context, path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("failed to read script: %w", err)
	}

	archive := txtar.Parse(data)

	// Parse metadata
	var mainScript string
	for _, f := range archive.Files {
		switch f.Name {
		case "meta.yaml":
			if err := yaml.Unmarshal(f.Data, &e.metadata); err != nil {
				return fmt.Errorf("failed to parse meta.yaml: %w", err)
			}
		case "main.cdp":
			mainScript = string(f.Data)
		}
	}

	if mainScript == "" {
		return fmt.Errorf("no main.cdp found in archive")
	}

	// Initialize browser
	if err := e.initBrowser(ctx); err != nil {
		return fmt.Errorf("failed to init browser: %w", err)
	}
	defer e.cleanup()

	// Build initial environment
	env := []string{}
	for k, v := range e.metadata.Env {
		env = append(env, k+"="+v)
	}

	// Create script state
	workDir, err := os.MkdirTemp("", "cdpscript-*")
	if err != nil {
		return fmt.Errorf("failed to create workdir: %w", err)
	}
	defer os.RemoveAll(workDir)

	// Write embedded files to workdir so commands like jsfile can access them
	for _, f := range archive.Files {
		if f.Name == "meta.yaml" || f.Name == "main.cdp" {
			continue // Skip metadata and main script
		}
		filePath := filepath.Join(workDir, f.Name)
		// Create parent directories if needed
		if dir := filepath.Dir(filePath); dir != workDir {
			if err := os.MkdirAll(dir, 0755); err != nil {
				return fmt.Errorf("failed to create directory for %s: %w", f.Name, err)
			}
		}
		if err := os.WriteFile(filePath, f.Data, 0644); err != nil {
			return fmt.Errorf("failed to write embedded file %s: %w", f.Name, err)
		}
		if e.verbose {
			fmt.Fprintf(os.Stderr, "[engine] Extracted embedded file: %s (%d bytes)\n", f.Name, len(f.Data))
		}
	}

	state, err := script.NewState(ctx, workDir, env)
	if err != nil {
		return fmt.Errorf("failed to create state: %w", err)
	}

	// Execute
	var logWriter io.Writer = io.Discard
	if e.verbose {
		logWriter = os.Stderr
	}

	return e.engine.Execute(state, "main.cdp", bufio.NewReader(strings.NewReader(mainScript)), logWriter)
}

// initBrowser initializes the browser instance.
func (e *Engine) initBrowser(ctx context.Context) error {
	// If a remote tab is specified, connect to it instead of launching a new browser
	if e.remoteTabID != "" {
		return e.connectToRemoteTab(ctx)
	}

	// Detect browser path
	chromePath := e.detectBrowserPath()
	if chromePath == "" {
		return fmt.Errorf("no Chromium-based browser found")
	}
	if e.verbose {
		fmt.Fprintf(os.Stderr, "[engine] Using browser: %s\n", chromePath)
	}

	// Setup profile if specified
	if e.metadata.Profile != "" {
		var err error
		e.profileMgr, err = chromeprofiles.NewProfileManager(
			chromeprofiles.WithVerbose(e.verbose),
		)
		if err != nil {
			return fmt.Errorf("failed to create profile manager: %w", err)
		}
		if err := e.profileMgr.SetupWorkdir(); err != nil {
			return fmt.Errorf("failed to setup profile workdir: %w", err)
		}
		if err := e.profileMgr.CopyProfile(e.metadata.Profile, nil); err != nil {
			return fmt.Errorf("failed to copy profile: %w", err)
		}
	}

	// Build browser options
	timeout := e.metadata.Timeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}

	browserOpts := []browser.Option{
		browser.WithHeadless(e.metadata.Headless),
		browser.WithTimeout(int(timeout.Seconds())),
		browser.WithChromePath(chromePath),
	}
	if e.verbose {
		browserOpts = append(browserOpts, browser.WithVerbose(true))
	}
	if e.metadata.Profile != "" {
		browserOpts = append(browserOpts, browser.WithProfile(e.metadata.Profile))
	}

	// Create and launch browser
	br, err := browser.New(ctx, e.profileMgr, browserOpts...)
	if err != nil {
		return fmt.Errorf("failed to create browser: %w", err)
	}

	if err := br.Launch(ctx); err != nil {
		return fmt.Errorf("failed to launch browser: %w", err)
	}

	e.browser = br
	return nil
}

// connectToRemoteTab connects to an existing browser tab.
func (e *Engine) connectToRemoteTab(ctx context.Context) error {
	port := e.remotePort
	if port == 0 {
		port = 9222
	}

	if e.verbose {
		fmt.Fprintf(os.Stderr, "[engine] Connecting to remote tab %s on port %d\n", e.remoteTabID, port)
	}

	// Create browser with remote options
	browserOpts := []browser.Option{
		browser.WithRemoteChrome("localhost", port),
		browser.WithRemoteTab(e.remoteTabID),
	}
	if e.verbose {
		browserOpts = append(browserOpts, browser.WithVerbose(true))
	}

	br, err := browser.New(ctx, nil, browserOpts...)
	if err != nil {
		return fmt.Errorf("failed to create browser: %w", err)
	}

	if err := br.Launch(ctx); err != nil {
		return fmt.Errorf("failed to connect to tab: %w", err)
	}

	e.browser = br
	return nil
}

func (e *Engine) detectBrowserPath() string {
	browserType := strings.ToLower(e.metadata.Browser)
	candidates := discovery.DiscoverBrowsers()

	if browserType != "" {
		for _, c := range candidates {
			if strings.Contains(strings.ToLower(c.Name), browserType) {
				return c.Path
			}
		}
	}
	return discovery.FindBestBrowser()
}

func (e *Engine) cleanup() {
	if e.browser != nil {
		e.browser.Close()
	}
	if e.profileMgr != nil {
		e.profileMgr.Cleanup()
	}
}

// commands returns the CDP commands for the script engine.
func (e *Engine) commands() map[string]script.Cmd {
	cmds := map[string]script.Cmd{
		// Navigation
		"goto":    e.cmdGoto(),
		"back":    e.cmdBack(),
		"forward": e.cmdForward(),
		"reload":  e.cmdReload(),

		// Waiting
		"wait": e.cmdWait(),

		// Interaction
		"click": e.cmdClick(),
		"fill":  e.cmdFill(),
		"type":  e.cmdType(),
		"hover": e.cmdHover(),
		"press": e.cmdPress(),

		// JavaScript
		"js":     e.cmdJS(),
		"jsfile": e.cmdJSFile(),

		// Extraction & inspection
		"extract": e.cmdExtract(),
		"title":   e.cmdTitle(),
		"url":     e.cmdURL(),

		// Assertions
		"assert": e.cmdAssert(),

		// Output
		"screenshot": e.cmdScreenshot(),
		"pdf":        e.cmdPDF(),
		"log":        e.cmdLog(),

		// Network
		"block": e.cmdBlock(),

		// Scripting
		"source": e.cmdSource(),

		// Accessibility & refs
		"snapshot": e.cmdSnapshot(),

		// HAR recording & tagging
		"tag":     e.cmdTag(),
		"har":     e.cmdHAR(),
		"note":    e.cmdNote(),
		"capture": e.cmdCapture(),
	}

	// Add any dynamically loaded commands from sourced scripts
	for name, cmd := range e.sourcedCmds {
		cmds[name] = cmd
	}

	return cmds
}

// conditions returns the conditions for the script engine.
func (e *Engine) conditions() map[string]script.Cond {
	return map[string]script.Cond{
		"headless": script.BoolCondition("running headless", e.metadata.Headless),
		"has-tab":  script.BoolCondition("connected to existing tab", e.remoteTabID != ""),
	}
}

// Helper to create a simple command
func simpleCmd(summary, args string, run func(s *script.State, args []string) error) script.Cmd {
	return script.Command(
		script.CmdUsage{Summary: summary, Args: args},
		func(s *script.State, args ...string) (script.WaitFunc, error) {
			return nil, run(s, args)
		},
	)
}

func (e *Engine) cmdGoto() script.Cmd {
	return simpleCmd("navigate to URL", "url", func(s *script.State, args []string) error {
		if len(args) < 1 {
			return fmt.Errorf("goto requires a URL")
		}
		url := args[0]
		if e.verbose {
			fmt.Fprintf(os.Stderr, "[goto] %s\n", url)
		}
		return e.browser.Navigate(url)
	})
}

func (e *Engine) cmdWait() script.Cmd {
	return simpleCmd("wait for duration or selector", "[duration|selector]", func(s *script.State, args []string) error {
		if len(args) < 1 {
			return fmt.Errorf("wait requires duration or selector")
		}
		arg := strings.Join(args, " ")

		// Try parsing as duration first
		if d, err := time.ParseDuration(arg); err == nil {
			if e.verbose {
				fmt.Fprintf(os.Stderr, "[wait] %v\n", d)
			}
			time.Sleep(d)
			return nil
		}

		// Otherwise treat as selector
		if e.verbose {
			fmt.Fprintf(os.Stderr, "[wait] for selector: %s\n", arg)
		}
		timeout := e.metadata.Timeout
		if timeout == 0 {
			timeout = 30 * time.Second
		}
		return e.browser.WaitForSelector(arg, timeout)
	})
}

func (e *Engine) cmdClick() script.Cmd {
	return simpleCmd("click element", "selector|@ref", func(s *script.State, args []string) error {
		if len(args) < 1 {
			return fmt.Errorf("click requires a selector or ref")
		}
		target := strings.Join(args, " ")
		if e.verbose {
			fmt.Fprintf(os.Stderr, "[click] %s\n", target)
		}
		page := e.browser.GetCurrentPage()
		if page == nil {
			return fmt.Errorf("no active page")
		}

		// Check if target is a ref
		if role, name, nth, isRef := e.resolveRef(target); isRef {
			if e.verbose {
				fmt.Fprintf(os.Stderr, "[click] resolved ref to role=%s name=%q nth=%d\n", role, name, nth)
			}
			return page.ClickByRole(role, name, nth)
		}

		return page.Click(target)
	})
}

func (e *Engine) cmdFill() script.Cmd {
	return simpleCmd("fill input field", "selector|@ref value", func(s *script.State, args []string) error {
		if len(args) < 2 {
			return fmt.Errorf("fill requires selector/ref and value")
		}
		target := args[0]
		value := strings.Join(args[1:], " ")
		if e.verbose {
			fmt.Fprintf(os.Stderr, "[fill] %s = %s\n", target, value)
		}
		page := e.browser.GetCurrentPage()
		if page == nil {
			return fmt.Errorf("no active page")
		}

		// Check if target is a ref
		if role, name, nth, isRef := e.resolveRef(target); isRef {
			if e.verbose {
				fmt.Fprintf(os.Stderr, "[fill] resolved ref to role=%s name=%q nth=%d\n", role, name, nth)
			}
			return page.TypeByRole(role, name, value, nth)
		}

		return page.Type(target, value)
	})
}

func (e *Engine) cmdType() script.Cmd {
	// type is an alias for fill with the same ref support
	return e.cmdFill()
}

func (e *Engine) cmdScreenshot() script.Cmd {
	return simpleCmd("take screenshot", "filename", func(s *script.State, args []string) error {
		if len(args) < 1 {
			return fmt.Errorf("screenshot requires filename")
		}
		filename := args[0]
		if e.outputDir != "" && !filepath.IsAbs(filename) {
			filename = filepath.Join(e.outputDir, filename)
		}

		if err := os.MkdirAll(filepath.Dir(filename), 0755); err != nil {
			return fmt.Errorf("failed to create directory: %w", err)
		}

		if e.verbose {
			fmt.Fprintf(os.Stderr, "[screenshot] %s\n", filename)
		}

		var data []byte
		if err := chromedp.Run(e.browser.Context(), chromedp.FullScreenshot(&data, 90)); err != nil {
			return fmt.Errorf("failed to take screenshot: %w", err)
		}
		if err := os.WriteFile(filename, data, 0644); err != nil {
			return fmt.Errorf("failed to write screenshot: %w", err)
		}
		fmt.Fprintf(os.Stderr, "Saved screenshot to %s (%d bytes)\n", filename, len(data))
		return nil
	})
}

func (e *Engine) cmdJS() script.Cmd {
	return simpleCmd("execute JavaScript", "code", func(s *script.State, args []string) error {
		if len(args) < 1 {
			return fmt.Errorf("js requires code")
		}
		code := strings.Join(args, " ")
		if e.verbose {
			fmt.Fprintf(os.Stderr, "[js] %s\n", code)
		}
		_, err := e.browser.ExecuteScript(code)
		return err
	})
}

func (e *Engine) cmdJSFile() script.Cmd {
	return simpleCmd("execute JavaScript from file", "filename", func(s *script.State, args []string) error {
		if len(args) < 1 {
			return fmt.Errorf("jsfile requires filename")
		}
		filename := args[0]
		// If path is relative, resolve it relative to the script's workdir
		if !filepath.IsAbs(filename) {
			filename = filepath.Join(s.Getwd(), filename)
		}
		data, err := os.ReadFile(filename)
		if err != nil {
			return fmt.Errorf("failed to read JS file: %w", err)
		}
		code := string(data)
		if e.verbose {
			fmt.Fprintf(os.Stderr, "[jsfile] %s (%d bytes)\n", filename, len(data))
		}
		result, err := e.browser.ExecuteScript(code)
		if err != nil {
			return err
		}
		// If the result is meaningful, print it
		if result != nil {
			fmt.Printf("%v\n", result)
		}
		return nil
	})
}

func (e *Engine) cmdLog() script.Cmd {
	return simpleCmd("log message", "message", func(s *script.State, args []string) error {
		msg := strings.Join(args, " ")
		fmt.Println(msg)
		return nil
	})
}

func (e *Engine) cmdPDF() script.Cmd {
	return simpleCmd("save page as PDF", "filename", func(s *script.State, args []string) error {
		if len(args) < 1 {
			return fmt.Errorf("pdf requires filename")
		}
		filename := args[0]
		if e.outputDir != "" && !filepath.IsAbs(filename) {
			filename = filepath.Join(e.outputDir, filename)
		}

		if err := os.MkdirAll(filepath.Dir(filename), 0755); err != nil {
			return fmt.Errorf("failed to create directory: %w", err)
		}

		if e.verbose {
			fmt.Fprintf(os.Stderr, "[pdf] %s\n", filename)
		}

		var data []byte
		if err := chromedp.Run(e.browser.Context(), chromedp.ActionFunc(func(ctx context.Context) error {
			var err error
			data, _, err = page.PrintToPDF().Do(ctx)
			return err
		})); err != nil {
			return fmt.Errorf("failed to generate PDF: %w", err)
		}

		return os.WriteFile(filename, data, 0644)
	})
}

func (e *Engine) cmdExtract() script.Cmd {
	return simpleCmd("extract text from element", "selector", func(s *script.State, args []string) error {
		if len(args) < 1 {
			return fmt.Errorf("extract requires selector")
		}
		selector := strings.Join(args, " ")
		page := e.browser.GetCurrentPage()
		if page == nil {
			return fmt.Errorf("no active page")
		}
		text, err := page.GetText(selector)
		if err != nil {
			return err
		}
		s.Setenv("EXTRACTED", text)
		fmt.Println(text)
		return nil
	})
}

func (e *Engine) cmdHover() script.Cmd {
	return simpleCmd("hover over element", "selector", func(s *script.State, args []string) error {
		if len(args) < 1 {
			return fmt.Errorf("hover requires selector")
		}
		selector := strings.Join(args, " ")
		page := e.browser.GetCurrentPage()
		if page == nil {
			return fmt.Errorf("no active page")
		}
		return page.Hover(selector)
	})
}

func (e *Engine) cmdPress() script.Cmd {
	return simpleCmd("press keyboard key", "key", func(s *script.State, args []string) error {
		if len(args) < 1 {
			return fmt.Errorf("press requires key")
		}
		key := args[0]
		page := e.browser.GetCurrentPage()
		if page == nil {
			return fmt.Errorf("no active page")
		}
		return page.Press(key)
	})
}

func (e *Engine) cmdBack() script.Cmd {
	return simpleCmd("go back in history", "", func(s *script.State, args []string) error {
		return chromedp.Run(e.browser.Context(), chromedp.NavigateBack())
	})
}

func (e *Engine) cmdForward() script.Cmd {
	return simpleCmd("go forward in history", "", func(s *script.State, args []string) error {
		return chromedp.Run(e.browser.Context(), chromedp.NavigateForward())
	})
}

func (e *Engine) cmdReload() script.Cmd {
	return simpleCmd("reload page", "", func(s *script.State, args []string) error {
		return chromedp.Run(e.browser.Context(), chromedp.Reload())
	})
}

func (e *Engine) cmdSource() script.Cmd {
	return script.Command(
		script.CmdUsage{
			Summary: "source a CDP script file",
			Args:    "[-x] [-as name] path",
			Detail: []string{
				"Loads and executes a .cdp script file.",
				"  -x       trace execution (show each command before running)",
				"  -as name register script as a new command with given name",
			},
		},
		func(s *script.State, args ...string) (script.WaitFunc, error) {
			// Parse flags
			trace := false
			asName := ""
			var scriptPath string

			i := 0
			for i < len(args) {
				switch args[i] {
				case "-x":
					trace = true
					i++
				case "-as":
					if i+1 >= len(args) {
						return nil, fmt.Errorf("source: -as requires a name argument")
					}
					asName = args[i+1]
					i += 2
				default:
					scriptPath = args[i]
					i++
				}
			}

			if scriptPath == "" {
				return nil, fmt.Errorf("source: path required")
			}

			// Read the script file
			data, err := os.ReadFile(scriptPath)
			if err != nil {
				return nil, fmt.Errorf("source: failed to read %s: %w", scriptPath, err)
			}

			scriptContent := string(data)

			// If -as is specified, register as a command instead of executing
			if asName != "" {
				e.registerSourcedCommand(asName, scriptContent, trace)
				if e.verbose {
					fmt.Fprintf(os.Stderr, "[source] Registered command: %s\n", asName)
				}
				// Rebuild the engine commands to include the new one
				e.engine.Cmds = e.commands()
				return nil, nil
			}

			// Execute the script inline
			return nil, e.executeSourcedScript(s, scriptContent, trace)
		},
	)
}

// registerSourcedCommand registers a sourced script as a callable command.
func (e *Engine) registerSourcedCommand(name, scriptContent string, trace bool) {
	e.sourcedCmds[name] = script.Command(
		script.CmdUsage{
			Summary: fmt.Sprintf("sourced command from script"),
			Args:    "[args...]",
		},
		func(s *script.State, args ...string) (script.WaitFunc, error) {
			// Set arguments as environment variables
			for i, arg := range args {
				s.Setenv(fmt.Sprintf("ARG%d", i+1), arg)
			}
			s.Setenv("ARGC", fmt.Sprintf("%d", len(args)))

			return nil, e.executeSourcedScript(s, scriptContent, trace)
		},
	)
}

func (e *Engine) cmdTitle() script.Cmd {
	return simpleCmd("get page title", "", func(s *script.State, args []string) error {
		var title string
		if err := chromedp.Run(e.browser.Context(), chromedp.Title(&title)); err != nil {
			return fmt.Errorf("failed to get title: %w", err)
		}
		s.Setenv("TITLE", title)
		fmt.Println(title)
		return nil
	})
}

func (e *Engine) cmdURL() script.Cmd {
	return simpleCmd("get current URL", "", func(s *script.State, args []string) error {
		var url string
		if err := chromedp.Run(e.browser.Context(), chromedp.Location(&url)); err != nil {
			return fmt.Errorf("failed to get URL: %w", err)
		}
		s.Setenv("URL", url)
		fmt.Println(url)
		return nil
	})
}

func (e *Engine) cmdAssert() script.Cmd {
	return script.Command(
		script.CmdUsage{
			Summary: "assert condition on page",
			Args:    "exists|text|visible selector [expected]",
			Detail: []string{
				"Assert conditions on the page:",
				"  assert exists selector    - element exists in DOM",
				"  assert text selector text - element contains text",
				"  assert visible selector   - element is visible",
			},
		},
		func(s *script.State, args ...string) (script.WaitFunc, error) {
			if len(args) < 2 {
				return nil, fmt.Errorf("assert requires condition and selector")
			}
			condition := args[0]
			selector := args[1]

			switch condition {
			case "exists":
				var count int
				err := chromedp.Run(e.browser.Context(),
					chromedp.Evaluate(fmt.Sprintf(`document.querySelectorAll(%q).length`, selector), &count),
				)
				if err != nil {
					return nil, fmt.Errorf("failed to check existence: %w", err)
				}
				if count == 0 {
					return nil, fmt.Errorf("assertion failed: no elements found for selector %q", selector)
				}
				if e.verbose {
					fmt.Fprintf(os.Stderr, "[assert] exists %s: found %d elements\n", selector, count)
				}

			case "text":
				if len(args) < 3 {
					return nil, fmt.Errorf("assert text requires expected text")
				}
				expected := strings.Join(args[2:], " ")
				var text string
				err := chromedp.Run(e.browser.Context(), chromedp.Text(selector, &text))
				if err != nil {
					return nil, fmt.Errorf("failed to get text: %w", err)
				}
				if !strings.Contains(text, expected) {
					return nil, fmt.Errorf("assertion failed: text %q does not contain %q", text, expected)
				}
				if e.verbose {
					fmt.Fprintf(os.Stderr, "[assert] text %s contains %q\n", selector, expected)
				}

			case "visible":
				var visible bool
				err := chromedp.Run(e.browser.Context(),
					chromedp.Evaluate(fmt.Sprintf(`
						(function() {
							const el = document.querySelector(%q);
							if (!el) return false;
							const style = window.getComputedStyle(el);
							return style.display !== 'none' && style.visibility !== 'hidden' && style.opacity !== '0';
						})()
					`, selector), &visible),
				)
				if err != nil {
					return nil, fmt.Errorf("failed to check visibility: %w", err)
				}
				if !visible {
					return nil, fmt.Errorf("assertion failed: element %q is not visible", selector)
				}
				if e.verbose {
					fmt.Fprintf(os.Stderr, "[assert] visible %s: true\n", selector)
				}

			default:
				return nil, fmt.Errorf("unknown assertion type: %s (use exists, text, or visible)", condition)
			}

			return nil, nil
		},
	)
}

func (e *Engine) cmdBlock() script.Cmd {
	return simpleCmd("block URLs matching pattern", "pattern", func(s *script.State, args []string) error {
		if len(args) < 1 {
			return fmt.Errorf("block requires a URL pattern")
		}
		pattern := args[0]
		if e.verbose {
			fmt.Fprintf(os.Stderr, "[block] %s\n", pattern)
		}
		// Use the browser's blocking mechanism
		return e.browser.BlockURLPattern(pattern)
	})
}

// executeSourcedScript executes script content with optional tracing.
func (e *Engine) executeSourcedScript(s *script.State, content string, trace bool) error {
	// Process line by line
	lines := strings.Split(content, "\n")

	for lineNo, line := range lines {
		line = strings.TrimSpace(line)

		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Expand environment variables in the line
		line = s.ExpandEnv(line, false)

		// Trace output if enabled
		if trace {
			fmt.Fprintf(os.Stderr, "+ %s\n", line)
		}

		// Parse the line into command and args
		parts := strings.Fields(line)
		if len(parts) == 0 {
			continue
		}

		cmdName := parts[0]
		cmdArgs := parts[1:]

		// Look up the command
		cmd, ok := e.engine.Cmds[cmdName]
		if !ok {
			return fmt.Errorf("source: line %d: unknown command: %s", lineNo+1, cmdName)
		}

		// Execute it
		waitFn, err := cmd.Run(s, cmdArgs...)
		if err != nil {
			return fmt.Errorf("source: line %d: %s: %w", lineNo+1, cmdName, err)
		}
		if waitFn != nil {
			if _, _, err := waitFn(s); err != nil {
				return fmt.Errorf("source: line %d: %s (wait): %w", lineNo+1, cmdName, err)
			}
		}
	}

	return nil
}

func (e *Engine) cmdSnapshot() script.Cmd {
	return script.Command(
		script.CmdUsage{
			Summary: "get accessibility snapshot with element refs",
			Args:    "[-i] [--depth N] [--compact] [--selector CSS]",
			Detail: []string{
				"Returns an accessibility tree with refs that can be used to interact with elements.",
				"  -i, --interactive  only include interactive elements (buttons, links, inputs)",
				"  --depth N          limit tree depth (0 = unlimited)",
				"  --compact          remove structural elements without meaningful content",
				"  --selector CSS     scope snapshot to elements matching CSS selector",
				"",
				"Example output:",
				"  - heading \"Example\" [ref=e1] [level=1]",
				"  - button \"Submit\" [ref=e2]",
				"  - textbox \"Email\" [ref=e3]",
				"",
				"Use refs with click/fill/type commands: click @e2",
			},
		},
		func(s *script.State, args ...string) (script.WaitFunc, error) {
			opts := browser.SnapshotOptions{}

			// Parse flags
			for i := 0; i < len(args); i++ {
				switch args[i] {
				case "-i", "--interactive":
					opts.Interactive = true
				case "--compact":
					opts.Compact = true
				case "--depth":
					if i+1 >= len(args) {
						return nil, fmt.Errorf("snapshot: --depth requires a number")
					}
					i++
					var depth int
					if _, err := fmt.Sscanf(args[i], "%d", &depth); err != nil {
						return nil, fmt.Errorf("snapshot: invalid depth: %s", args[i])
					}
					opts.MaxDepth = depth
				case "--selector":
					if i+1 >= len(args) {
						return nil, fmt.Errorf("snapshot: --selector requires a CSS selector")
					}
					i++
					opts.Selector = args[i]
				default:
					return nil, fmt.Errorf("snapshot: unknown flag: %s", args[i])
				}
			}

			page := e.browser.GetCurrentPage()
			if page == nil {
				return nil, fmt.Errorf("no active page")
			}

			snapshot, err := page.GetAccessibilitySnapshot(opts)
			if err != nil {
				return nil, fmt.Errorf("getting snapshot: %w", err)
			}

			// Store refs for later use
			e.refMap = snapshot.Refs

			// Print the tree
			fmt.Println(snapshot.Tree)

			// Print stats if verbose
			if e.verbose {
				stats := browser.GetSnapshotStats(snapshot.Tree, snapshot.Refs)
				fmt.Fprintf(os.Stderr, "[snapshot] %d refs, %d interactive, %d lines\n",
					stats["refs"], stats["interactive"], stats["lines"])
			}

			// Set environment variable with ref count
			s.Setenv("SNAPSHOT_REFS", fmt.Sprintf("%d", len(snapshot.Refs.Refs)))

			return nil, nil
		},
	)
}

// resolveRef resolves a ref (like "@e1" or "e1") to role, name, nth for interaction.
// Returns empty strings if not a ref.
func (e *Engine) resolveRef(target string) (role, name string, nth int, isRef bool) {
	ref := browser.ParseRef(target)
	if ref == "" {
		return "", "", 0, false
	}

	if e.refMap == nil || e.refMap.Refs == nil {
		return "", "", 0, false
	}

	entry, ok := e.refMap.Refs[ref]
	if !ok {
		return "", "", 0, false
	}

	return entry.Role, entry.Name, entry.Nth, true
}

func (e *Engine) cmdTag() script.Cmd {
	return script.Command(
		script.CmdUsage{
			Summary: "set tag for network activity",
			Args:    "[tag-name]",
			Detail: []string{
				"Tags subsequent network requests until a new tag is set or cleared.",
				"Tags are included in HAR output for filtering and organization.",
				"",
				"Examples:",
				"  tag login-flow    # Tag requests as 'login-flow'",
				"  tag               # Clear current tag",
				"  tag dashboard     # Tag requests as 'dashboard'",
			},
		},
		func(s *script.State, args ...string) (script.WaitFunc, error) {
			if e.recorder == nil {
				// Create a recorder if one doesn't exist
				rec, err := recorder.New(recorder.WithVerbose(e.verbose))
				if err != nil {
					return nil, fmt.Errorf("creating recorder: %w", err)
				}
				e.recorder = rec

				// Enable network events first
				if err := chromedp.Run(e.browser.Context(), network.Enable()); err != nil {
					return nil, fmt.Errorf("enabling network events: %w", err)
				}

				// Start recording network events
				chromedp.ListenTarget(e.browser.Context(), e.recorder.HandleNetworkEvent(e.browser.Context()))
				if e.verbose {
					fmt.Fprintf(os.Stderr, "[tag] Started HAR recording\n")
				}
			}

			tag := ""
			if len(args) > 0 {
				tag = strings.Join(args, "-")
			}

			e.recorder.SetTag(tag)
			s.Setenv("CURRENT_TAG", tag)

			if e.verbose {
				if tag != "" {
					fmt.Fprintf(os.Stderr, "[tag] Set to: %s\n", tag)
				} else {
					fmt.Fprintf(os.Stderr, "[tag] Cleared\n")
				}
			}

			return nil, nil
		},
	)
}

func (e *Engine) cmdHAR() script.Cmd {
	return script.Command(
		script.CmdUsage{
			Summary: "write HAR file",
			Args:    "filename",
			Detail: []string{
				"Writes captured network activity to a HAR file.",
				"Includes tags, annotations, and tag ranges.",
				"",
				"Example:",
				"  har output.har",
			},
		},
		func(s *script.State, args ...string) (script.WaitFunc, error) {
			if len(args) < 1 {
				return nil, fmt.Errorf("har requires filename")
			}

			if e.recorder == nil {
				return nil, fmt.Errorf("no HAR recording active (use 'tag' command first)")
			}

			filename := args[0]
			if e.outputDir != "" && !filepath.IsAbs(filename) {
				filename = filepath.Join(e.outputDir, filename)
			}

			if err := os.MkdirAll(filepath.Dir(filename), 0755); err != nil {
				return nil, fmt.Errorf("creating directory: %w", err)
			}

			if err := e.recorder.WriteHAR(filename); err != nil {
				return nil, fmt.Errorf("writing HAR: %w", err)
			}

			if e.verbose {
				fmt.Fprintf(os.Stderr, "[har] Written to %s\n", filename)
			}
			fmt.Printf("HAR saved to %s\n", filename)

			return nil, nil
		},
	)
}

func (e *Engine) cmdNote() script.Cmd {
	return simpleCmd("add note to HAR", "description", func(s *script.State, args []string) error {
		if len(args) < 1 {
			return fmt.Errorf("note requires description")
		}

		if e.recorder == nil {
			return fmt.Errorf("no HAR recording active (use 'tag' command first)")
		}

		description := strings.Join(args, " ")
		if err := e.recorder.AddNote(e.browser.Context(), description); err != nil {
			return fmt.Errorf("adding note: %w", err)
		}

		if e.verbose {
			fmt.Fprintf(os.Stderr, "[note] Added: %s\n", description)
		}

		return nil
	})
}

func (e *Engine) cmdCapture() script.Cmd {
	return script.Command(
		script.CmdUsage{
			Summary: "capture screenshot or DOM to HAR",
			Args:    "screenshot|dom [description]",
			Detail: []string{
				"Captures a screenshot or DOM snapshot and adds to HAR annotations.",
				"",
				"Examples:",
				"  capture screenshot Login page loaded",
				"  capture dom Before form submission",
			},
		},
		func(s *script.State, args ...string) (script.WaitFunc, error) {
			if len(args) < 1 {
				return nil, fmt.Errorf("capture requires type (screenshot or dom)")
			}

			if e.recorder == nil {
				return nil, fmt.Errorf("no HAR recording active (use 'tag' command first)")
			}

			captureType := strings.ToLower(args[0])
			description := ""
			if len(args) > 1 {
				description = strings.Join(args[1:], " ")
			}

			switch captureType {
			case "screenshot":
				if err := e.recorder.AddScreenshot(e.browser.Context(), description); err != nil {
					return nil, fmt.Errorf("capturing screenshot: %w", err)
				}
				if e.verbose {
					fmt.Fprintf(os.Stderr, "[capture] Screenshot: %s\n", description)
				}

			case "dom":
				if err := e.recorder.AddDOMSnapshot(e.browser.Context(), description); err != nil {
					return nil, fmt.Errorf("capturing DOM: %w", err)
				}
				if e.verbose {
					fmt.Fprintf(os.Stderr, "[capture] DOM: %s\n", description)
				}

			default:
				return nil, fmt.Errorf("unknown capture type: %s (use screenshot or dom)", captureType)
			}

			return nil, nil
		},
	)
}
