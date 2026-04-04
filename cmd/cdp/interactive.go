package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/chromedp/cdproto/target"
	"github.com/chromedp/chromedp"
)

// InteractiveMode represents an interactive CDP session
type InteractiveMode struct {
	browserCtx   context.Context // browser-level context for creating/listing tabs
	ctx          context.Context // active tab context for executing commands
	cancel       context.CancelFunc
	launched     bool // true if we launched the browser (should close on exit)
	cfg          fullCaptureConfig
	registry     *CommandRegistry
	help         *HelpSystem
	history      []string
	verbose      bool
	baseOutputDir string                    // root output dir from --output-dir
	contextStack  []string                  // stack of context names for push/pop
	recorder      recorderWithOutputDir     // optional recorder for output dir switching
}

// recorderWithOutputDir is the subset of recorder.Recorder needed for context switching.
type recorderWithOutputDir interface {
	SetOutputDir(dir string)
}

// NewInteractiveMode creates a new interactive session
func NewInteractiveMode(ctx context.Context, cancel context.CancelFunc, launched bool, cfg fullCaptureConfig) *InteractiveMode {
	registry := NewCommandRegistry()
	return &InteractiveMode{
		browserCtx:    ctx,
		ctx:           ctx,
		cancel:        cancel,
		launched:      launched,
		cfg:           cfg,
		registry:      registry,
		help:          NewHelpSystem(registry),
		history:       make([]string, 0),
		verbose:       cfg.Verbose,
		baseOutputDir: cfg.OutputDir,
	}
}

// SetRecorder sets the recorder for output dir switching with push/pop context.
func (im *InteractiveMode) SetRecorder(rec recorderWithOutputDir, baseOutputDir string) {
	im.recorder = rec
	im.baseOutputDir = baseOutputDir
}

// Run starts the interactive session
func (im *InteractiveMode) Run() error {
	im.showWelcome()

	scanner := bufio.NewScanner(os.Stdin)

	for {
		fmt.Print("cdp> ")

		if !scanner.Scan() {
			break
		}

		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		// Add to history
		im.history = append(im.history, line)

		// Handle special commands
		if im.handleSpecialCommand(line) {
			continue
		}

		// Check for exit
		if line == "exit" || line == "quit" || line == "q" {
			if im.launched && im.cancel != nil {
				im.cancel()
				fmt.Println("Browser closed.")
			}
			fmt.Println("Goodbye!")
			break
		}

		// Execute command
		if err := im.executeCommand(line); err != nil {
			fmt.Printf("Error: %v\n", err)
		}
	}

	// Close the browser on exit (EOF, Ctrl-D) if we launched it.
	if im.launched && im.cancel != nil {
		im.cancel()
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("error reading input: %w", err)
	}

	return nil
}

// showWelcome displays the welcome message
func (im *InteractiveMode) showWelcome() {
	fmt.Println("\n╭─────────────────────────────────────────────────────────╮")
	fmt.Println("│      Welcome to CDP Interactive Mode                    │")
	fmt.Println("│      Chrome DevTools Protocol Command Line Interface    │")
	fmt.Println("╰─────────────────────────────────────────────────────────╯")
	fmt.Println()
	fmt.Println("Type 'help' for available commands or 'quick' for quick reference")
	fmt.Println("Type 'exit' or 'quit' to leave")
	fmt.Println()
}

// handleSpecialCommand handles special non-CDP commands
func (im *InteractiveMode) handleSpecialCommand(line string) bool {
	parts := strings.Fields(line)
	if len(parts) == 0 {
		return false
	}

	cmd := parts[0]
	args := parts[1:]

	switch cmd {
	case "help", "h", "?":
		im.help.ShowHelp(args)
		return true

	case "list", "ls":
		im.help.ListCommands()
		return true

	case "search", "find":
		if len(args) > 0 {
			im.help.SearchCommands(strings.Join(args, " "))
		} else {
			fmt.Println("Usage: search <term>")
		}
		return true

	case "quick", "qr", "ref":
		im.help.ShowQuickReference()
		return true

	case "history", "hist":
		im.showHistory()
		return true

	case "clear", "cls":
		im.clearScreen()
		return true

	case "verbose":
		im.verbose = !im.verbose
		fmt.Printf("Verbose mode: %v\n", im.verbose)
		return true

	case "version", "ver":
		fmt.Println("CDP Tool v1.0.0")
		return true

	case "refresh-profile", "rp":
		if im.cfg.UseProfile == "" {
			fmt.Println("No profile configured. Use --use-profile when launching.")
			return true
		}
		fmt.Printf("Re-copying profile '%s' and reconnecting...\n", im.cfg.UseProfile)
		if err := im.reconnect(); err != nil {
			fmt.Printf("Error: %v\n", err)
		}
		return true

	case "reconnect", "rc":
		fmt.Println("Reconnecting to browser...")
		if err := im.reconnect(); err != nil {
			fmt.Printf("Error: %v\n", err)
		}
		return true

	case "tabs", "list-tabs", "lt":
		im.listTabs()
		return true

	case "newtab", "nt":
		url := "about:blank"
		if len(args) > 0 {
			url = args[0]
		}
		im.newTab(url)
		return true

	case "tab", "t":
		if len(args) == 0 {
			fmt.Println("Usage: tab <index|id>")
			return true
		}
		im.switchTab(args[0])
		return true

	case "push-context", "push":
		if len(args) == 0 {
			fmt.Println("Usage: push-context <name>")
			return true
		}
		im.pushContext(args[0])
		return true

	case "pop-context", "pop":
		im.popContext()
		return true

	case "context":
		im.showContext()
		return true

	default:
		return false
	}
}

// reconnect attempts to re-establish the browser connection.
func (im *InteractiveMode) reconnect() error {
	fmt.Println("Reconnecting to browser...")
	if im.cancel != nil {
		im.cancel()
	}
	ctx, cancel, launched, err := setupChromeForEnhanced(context.Background(), im.cfg)
	if err != nil {
		return fmt.Errorf("reconnect failed: %w", err)
	}
	im.browserCtx = ctx
	im.ctx = ctx
	im.cancel = cancel
	im.launched = launched
	fmt.Println("Reconnected.")
	return nil
}

// listTabs lists all open browser tabs.
func (im *InteractiveMode) listTabs() {
	targets, err := chromedp.Targets(im.browserCtx)
	if err != nil {
		fmt.Printf("Error listing tabs: %v\n", err)
		return
	}

	// Get the active target ID for marking.
	activeTarget := chromedp.FromContext(im.ctx).Target
	var activeID string
	if activeTarget != nil {
		activeID = string(activeTarget.TargetID)
	}

	fmt.Println("Open tabs:")
	idx := 0
	for _, t := range targets {
		if t.Type != "page" {
			continue
		}
		marker := "  "
		if string(t.TargetID) == activeID {
			marker = "* "
		}
		title := t.Title
		if title == "" {
			title = "(untitled)"
		}
		fmt.Printf("  %s[%d] %s — %s\n", marker, idx, title, t.URL)
		idx++
	}
	if idx == 0 {
		fmt.Println("  (no tabs)")
	}
}

// newTab creates a new browser tab and switches to it.
func (im *InteractiveMode) newTab(url string) {
	tabCtx, _ := chromedp.NewContext(im.browserCtx)
	if err := chromedp.Run(tabCtx, chromedp.Navigate(url)); err != nil {
		fmt.Printf("Error creating tab: %v\n", err)
		return
	}
	im.ctx = tabCtx
	fmt.Printf("New tab: %s\n", url)
}

// switchTab switches the active context to an existing tab by index or target ID.
func (im *InteractiveMode) switchTab(selector string) {
	targets, err := chromedp.Targets(im.browserCtx)
	if err != nil {
		fmt.Printf("Error listing tabs: %v\n", err)
		return
	}

	// Filter to page targets only.
	var pages []*target.Info
	for _, t := range targets {
		if t.Type == "page" {
			pages = append(pages, t)
		}
	}

	// Try as numeric index first.
	var targetInfo *target.Info
	if idx, err := strconv.Atoi(selector); err == nil {
		if idx >= 0 && idx < len(pages) {
			targetInfo = pages[idx]
		} else {
			fmt.Printf("Tab index %d out of range (0-%d)\n", idx, len(pages)-1)
			return
		}
	} else {
		// Try as target ID prefix.
		for _, t := range pages {
			if strings.HasPrefix(string(t.TargetID), selector) {
				targetInfo = t
				break
			}
		}
	}

	if targetInfo == nil {
		fmt.Printf("No tab matching '%s'\n", selector)
		return
	}

	tabCtx, _ := chromedp.NewContext(im.browserCtx, chromedp.WithTargetID(targetInfo.TargetID))
	// Run a no-op to attach to the target.
	if err := chromedp.Run(tabCtx); err != nil {
		fmt.Printf("Error switching to tab: %v\n", err)
		return
	}
	im.ctx = tabCtx

	title := targetInfo.Title
	if title == "" {
		title = "(untitled)"
	}
	fmt.Printf("Switched to: %s — %s\n", title, targetInfo.URL)
}

// contextOutputDir returns the output directory for the current context stack.
func (im *InteractiveMode) contextOutputDir() string {
	dir := im.baseOutputDir
	for _, name := range im.contextStack {
		dir = filepath.Join(dir, name)
	}
	return dir
}

// pushContext pushes a named context, directing HAR/HARL writes to a subdirectory.
func (im *InteractiveMode) pushContext(name string) {
	if im.baseOutputDir == "" {
		fmt.Println("No --output-dir configured; push-context has no effect.")
		return
	}
	im.contextStack = append(im.contextStack, name)
	dir := im.contextOutputDir()
	if err := os.MkdirAll(dir, 0755); err != nil {
		fmt.Printf("Error creating context dir: %v\n", err)
		return
	}
	if im.recorder != nil {
		im.recorder.SetOutputDir(dir)
	}
	fmt.Printf("Context: %s — %s\n", strings.Join(im.contextStack, "/"), dir)
}

// popContext pops the current context, returning to the parent directory.
func (im *InteractiveMode) popContext() {
	if len(im.contextStack) == 0 {
		fmt.Println("No context to pop.")
		return
	}
	im.contextStack = im.contextStack[:len(im.contextStack)-1]
	dir := im.contextOutputDir()
	if im.recorder != nil {
		im.recorder.SetOutputDir(dir)
	}
	if len(im.contextStack) == 0 {
		fmt.Printf("Context: (root) — %s\n", dir)
	} else {
		fmt.Printf("Context: %s — %s\n", strings.Join(im.contextStack, "/"), dir)
	}
}

// showContext shows the current context stack and output directory.
func (im *InteractiveMode) showContext() {
	if im.baseOutputDir == "" {
		fmt.Println("No --output-dir configured.")
		return
	}
	if len(im.contextStack) == 0 {
		fmt.Printf("Context: (root) — %s\n", im.baseOutputDir)
	} else {
		fmt.Printf("Context: %s — %s\n", strings.Join(im.contextStack, "/"), im.contextOutputDir())
	}
}

// isDisconnected reports whether an error indicates the browser connection is lost.
func isDisconnected(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "context canceled") ||
		strings.Contains(msg, "connection reset") ||
		strings.Contains(msg, "broken pipe") ||
		strings.Contains(msg, "use of closed network connection")
}

// executeCommand executes a CDP command
func (im *InteractiveMode) executeCommand(line string) error {
	parts := strings.Fields(line)
	if len(parts) == 0 {
		return nil
	}

	cmdName := parts[0]
	args := parts[1:]

	// Check if it's a registered command
	if cmd, found := im.registry.GetCommand(cmdName); found {
		if im.verbose {
			fmt.Printf("Executing: %s\n", cmd.Name)
		}
		err := cmd.Handler(im.ctx, args)
		if isDisconnected(err) {
			if reconnErr := im.reconnect(); reconnErr != nil {
				return reconnErr
			}
			// Retry the command once after reconnecting.
			return cmd.Handler(im.ctx, args)
		}
		return err
	}

	// Try to execute as raw CDP command
	if strings.Contains(cmdName, ".") {
		return im.executeRawCDP(line)
	}

	// Try to get completions
	completions := im.help.GetCompletions(cmdName)
	if len(completions) > 0 {
		fmt.Printf("Unknown command '%s'. Did you mean:\n", cmdName)
		for _, c := range completions {
			fmt.Printf("  • %s\n", c)
		}
		return nil
	}

	return fmt.Errorf("unknown command: %s", cmdName)
}

// executeRawCDP executes a raw CDP command
func (im *InteractiveMode) executeRawCDP(command string) error {
	// Parse Domain.method {params}
	parts := strings.SplitN(command, " ", 2)
	if len(parts) == 0 {
		return fmt.Errorf("empty command")
	}

	method := parts[0]
	if !strings.Contains(method, ".") {
		return fmt.Errorf("invalid CDP format: expected 'Domain.method'")
	}

	// Parse parameters
	params := "{}"
	if len(parts) > 1 {
		params = strings.TrimSpace(parts[1])
	}

	if im.verbose {
		fmt.Printf("Raw CDP: %s %s\n", method, params)
	}

	// Execute using chromedp (simplified - would need full CDP implementation)
	fmt.Printf("Executing CDP: %s with params: %s\n", method, params)
	fmt.Println("(Note: Raw CDP execution requires full implementation)")

	return nil
}

// showHistory displays command history
func (im *InteractiveMode) showHistory() {
	if len(im.history) == 0 {
		fmt.Println("No command history")
		return
	}

	fmt.Println("\nCommand History:")
	fmt.Println("────────────────")
	for i, cmd := range im.history {
		fmt.Printf("%3d: %s\n", i+1, cmd)
	}
	fmt.Println()
}

// clearScreen clears the terminal screen
func (im *InteractiveMode) clearScreen() {
	// ANSI escape code to clear screen
	fmt.Print("\033[2J\033[H")
	im.showWelcome()
}

// TabComplete provides tab completion for commands
func (im *InteractiveMode) TabComplete(partial string) []string {
	return im.help.GetCompletions(partial)
}

// ExecuteScript executes a script file
func (im *InteractiveMode) ExecuteScript(filename string) error {
	file, err := os.Open(filename)
	if err != nil {
		return fmt.Errorf("opening script file: %w", err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	lineNum := 0

	fmt.Printf("Executing script: %s\n", filename)
	fmt.Println("────────────────────────")

	for scanner.Scan() {
		lineNum++
		line := strings.TrimSpace(scanner.Text())

		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "//") {
			continue
		}

		fmt.Printf("[%d] %s\n", lineNum, line)

		// Execute command
		if err := im.executeCommand(line); err != nil {
			return fmt.Errorf("line %d: %w", lineNum, err)
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("reading script: %w", err)
	}

	fmt.Println("\nScript execution completed")
	return nil
}

// BatchExecute executes multiple commands in batch
func (im *InteractiveMode) BatchExecute(commands []string) error {
	fmt.Println("Executing batch commands:")
	fmt.Println("─────────────────────────")

	for i, cmd := range commands {
		fmt.Printf("[%d/%d] %s\n", i+1, len(commands), cmd)

		if err := im.executeCommand(cmd); err != nil {
			return fmt.Errorf("command %d: %w", i+1, err)
		}
	}

	fmt.Println("\nBatch execution completed")
	return nil
}

// SaveSession saves the current session to a file
func (im *InteractiveMode) SaveSession(filename string) error {
	file, err := os.Create(filename)
	if err != nil {
		return fmt.Errorf("creating session file: %w", err)
	}
	defer file.Close()

	fmt.Fprintf(file, "# CDP Session - %s\n", time.Now().Format(time.RFC3339))
	fmt.Fprintf(file, "# Commands: %d\n\n", len(im.history))

	for _, cmd := range im.history {
		fmt.Fprintf(file, "%s\n", cmd)
	}

	fmt.Printf("Session saved to: %s (%d commands)\n", filename, len(im.history))
	return nil
}

// LoadSession loads and executes a saved session
func (im *InteractiveMode) LoadSession(filename string) error {
	return im.ExecuteScript(filename)
}