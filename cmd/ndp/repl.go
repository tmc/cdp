package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/chromedp/chromedp"
	"github.com/chromedp/cdproto/debugger"
	"github.com/chromedp/cdproto/runtime"
	"github.com/pkg/errors"
)

// REPL represents an interactive Read-Eval-Print Loop
type REPL struct {
	session   *Session
	manager   *SessionManager
	verbose   bool
	history   []string
	running   bool
}

// NewREPL creates a new REPL instance
func NewREPL(verbose bool) *REPL {
	return &REPL{
		manager: NewSessionManager(verbose),
		verbose: verbose,
		history: []string{},
		running: false,
	}
}

// Start starts the interactive REPL
func (r *REPL) Start(ctx context.Context, targetID string) error {
	fmt.Println("NDP Interactive REPL")
	fmt.Println("====================")

	// Connect to target if specified
	if targetID != "" {
		if err := r.connectToTarget(ctx, targetID); err != nil {
			return errors.Wrap(err, "failed to connect to target")
		}
	} else {
		// List available targets
		if err := r.showTargets(ctx); err != nil {
			return errors.Wrap(err, "failed to list targets")
		}
	}

	r.running = true
	r.showHelp()

	scanner := bufio.NewScanner(os.Stdin)

	for r.running {
		fmt.Print("ndp> ")

		if !scanner.Scan() {
			break
		}

		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		// Add to history
		r.history = append(r.history, line)

		// Process command
		if err := r.processCommand(ctx, line); err != nil {
			fmt.Printf("Error: %v\n", err)
		}
	}

	if err := scanner.Err(); err != nil {
		return errors.Wrap(err, "input error")
	}

	return nil
}

// processCommand processes a REPL command
func (r *REPL) processCommand(ctx context.Context, line string) error {
	parts := strings.Fields(line)
	if len(parts) == 0 {
		return nil
	}

	command := parts[0]
	args := parts[1:]

	switch command {
	case "help", "h", "?":
		r.showHelp()

	case "targets", "list":
		return r.showTargets(ctx)

	case "connect", "attach":
		if len(args) == 0 {
			return errors.New("usage: connect <target-number>")
		}
		targetNum, err := strconv.Atoi(args[0])
		if err != nil {
			return errors.New("invalid target number")
		}
		return r.connectToTargetByNumber(ctx, targetNum)

	case "disconnect":
		return r.disconnect()

	case "status":
		r.showStatus()

	case "exec", "eval", "js":
		if len(args) == 0 {
			return errors.New("usage: exec <javascript>")
		}
		return r.executeJavaScript(ctx, strings.Join(args, " "))

	case "break", "bp":
		if len(args) == 0 {
			return errors.New("usage: break <file:line> [condition]")
		}
		location := args[0]
		condition := ""
		if len(args) > 1 {
			condition = strings.Join(args[1:], " ")
		}
		return r.setBreakpoint(ctx, location, condition)

	case "breakpoints", "bps":
		r.listBreakpoints()

	case "continue", "c":
		return r.continueExecution(ctx)

	case "step", "s":
		return r.stepInto(ctx)

	case "next", "n":
		return r.stepOver(ctx)

	case "out", "o":
		return r.stepOut(ctx)

	case "pause":
		return r.pauseExecution(ctx)

	case "inspect", "i":
		if len(args) == 0 {
			return errors.New("usage: inspect <selector>")
		}
		return r.inspectElement(ctx, args[0])

	case "navigate", "goto":
		if len(args) == 0 {
			return errors.New("usage: navigate <url>")
		}
		return r.navigate(ctx, args[0])

	case "screenshot":
		filename := "screenshot.png"
		if len(args) > 0 {
			filename = args[0]
		}
		return r.takeScreenshot(ctx, filename)

	case "history":
		r.showHistory()

	case "clear":
		fmt.Print("\033[2J\033[H") // Clear screen

	case "exit", "quit", "q":
		r.running = false

	default:
		// Try to execute as JavaScript if connected
		if r.session != nil {
			return r.executeJavaScript(ctx, line)
		} else {
			return fmt.Errorf("unknown command: %s (type 'help' for commands)", command)
		}
	}

	return nil
}

// showHelp displays help information
func (r *REPL) showHelp() {
	fmt.Println("\nNDP REPL Commands:")
	fmt.Println("==================")
	fmt.Println("Connection:")
	fmt.Println("  targets, list         - List available debug targets")
	fmt.Println("  connect <num>         - Connect to target by number")
	fmt.Println("  disconnect            - Disconnect from current target")
	fmt.Println("  status                - Show connection status")
	fmt.Println()
	fmt.Println("Execution:")
	fmt.Println("  exec <js>             - Execute JavaScript expression")
	fmt.Println("  navigate <url>        - Navigate to URL (Chrome only)")
	fmt.Println("  inspect <selector>    - Inspect DOM element (Chrome only)")
	fmt.Println("  screenshot [file]     - Take screenshot (Chrome only)")
	fmt.Println()
	fmt.Println("Debugging:")
	fmt.Println("  break <file:line> [condition] - Set breakpoint")
	fmt.Println("  breakpoints, bps      - List breakpoints")
	fmt.Println("  continue, c           - Continue execution")
	fmt.Println("  step, s               - Step into")
	fmt.Println("  next, n               - Step over")
	fmt.Println("  out, o                - Step out")
	fmt.Println("  pause                 - Pause execution")
	fmt.Println()
	fmt.Println("Utility:")
	fmt.Println("  history               - Show command history")
	fmt.Println("  clear                 - Clear screen")
	fmt.Println("  help, h, ?            - Show this help")
	fmt.Println("  exit, quit, q         - Exit REPL")
	fmt.Println()
	fmt.Println("Note: Any unrecognized command will be executed as JavaScript if connected.")
	fmt.Println()
}

// showTargets lists available debug targets
func (r *REPL) showTargets(ctx context.Context) error {
	targets, err := r.manager.ListTargets(ctx)
	if err != nil {
		return err
	}

	if len(targets) == 0 {
		fmt.Println("No debug targets found.")
		fmt.Println("Make sure you have:")
		fmt.Println("  - Node.js running with --inspect flag")
		fmt.Println("  - Chrome running with --remote-debugging-port flag")
		return nil
	}

	fmt.Printf("\nAvailable targets (%d):\n", len(targets))
	fmt.Println("========================")

	for i, target := range targets {
		status := ""
		if r.session != nil && r.session.Target.ID == target.ID {
			status = " [CONNECTED]"
		}

		fmt.Printf("[%d] %s - %s%s\n", i+1, target.Type, target.Title, status)
		fmt.Printf("    URL: %s\n", target.URL)
		fmt.Printf("    Port: %s\n", target.Port)
		fmt.Println()
	}

	return nil
}

// connectToTargetByNumber connects to a target by its number in the list
func (r *REPL) connectToTargetByNumber(ctx context.Context, targetNum int) error {
	targets, err := r.manager.ListTargets(ctx)
	if err != nil {
		return err
	}

	if targetNum < 1 || targetNum > len(targets) {
		return fmt.Errorf("invalid target number: %d (available: 1-%d)", targetNum, len(targets))
	}

	target := targets[targetNum-1]
	return r.connectToTarget(ctx, target.ID)
}

// connectToTarget connects to a specific target
func (r *REPL) connectToTarget(ctx context.Context, targetID string) error {
	targets, err := r.manager.ListTargets(ctx)
	if err != nil {
		return err
	}

	var target *DebugTarget
	for _, t := range targets {
		if t.ID == targetID {
			target = &t
			break
		}
	}

	if target == nil {
		return fmt.Errorf("target %s not found", targetID)
	}

	// Disconnect from current session if any
	if r.session != nil {
		r.manager.CloseSession(r.session.ID)
	}

	// Create new session
	session, err := r.manager.CreateSession(ctx, *target)
	if err != nil {
		return err
	}

	r.session = session

	// Enable necessary domains
	err = chromedp.Run(session.ChromeCtx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			if err := runtime.Enable().Do(ctx); err != nil {
				return err
			}
			_, err := debugger.Enable().Do(ctx)
			return err
		}),
	)
	if err != nil {
		return err
	}

	fmt.Printf("✓ Connected to %s: %s\n", target.Type, target.Title)

	return nil
}

// disconnect disconnects from the current target
func (r *REPL) disconnect() error {
	if r.session == nil {
		fmt.Println("Not connected to any target")
		return nil
	}

	if err := r.manager.CloseSession(r.session.ID); err != nil {
		return err
	}

	r.session = nil
	fmt.Println("✓ Disconnected")

	return nil
}

// showStatus shows the current connection status
func (r *REPL) showStatus() {
	if r.session == nil {
		fmt.Println("Status: Not connected")
		return
	}

	fmt.Printf("Status: Connected to %s\n", r.session.Target.Type)
	fmt.Printf("Target: %s\n", r.session.Target.Title)
	fmt.Printf("URL: %s\n", r.session.Target.URL)
	fmt.Printf("Session ID: %s\n", r.session.ID)
	fmt.Printf("Connected: %s\n", r.session.Created.Format("15:04:05"))
}

// executeJavaScript executes JavaScript in the connected target
func (r *REPL) executeJavaScript(ctx context.Context, expression string) error {
	if r.session == nil {
		return errors.New("not connected to any target")
	}

	result, err := r.session.Execute(ctx, expression)
	if err != nil {
		return err
	}

	fmt.Printf("→ %v\n", result)

	return nil
}

// setBreakpoint sets a breakpoint
func (r *REPL) setBreakpoint(ctx context.Context, location string, condition string) error {
	if r.session == nil {
		return errors.New("not connected to any target")
	}

	// Create breakpoint manager
	manager := NewBreakpointManager(r.verbose)
	manager.SetSession(r.session)

	return manager.SetBreakpoint(ctx, location, condition)
}

// listBreakpoints lists all breakpoints
func (r *REPL) listBreakpoints() {
	// TODO: Implement breakpoint listing
	fmt.Println("Breakpoint listing not yet implemented")
}

// continueExecution continues execution after a breakpoint
func (r *REPL) continueExecution(ctx context.Context) error {
	if r.session == nil {
		return errors.New("not connected to any target")
	}

	debugger := NewNodeDebugger(r.verbose)
	debugger.session = r.session

	return debugger.resume()
}

// stepInto steps into the next function call
func (r *REPL) stepInto(ctx context.Context) error {
	if r.session == nil {
		return errors.New("not connected to any target")
	}

	debugger := NewNodeDebugger(r.verbose)
	debugger.session = r.session

	return debugger.stepInto()
}

// stepOver steps over the next line
func (r *REPL) stepOver(ctx context.Context) error {
	if r.session == nil {
		return errors.New("not connected to any target")
	}

	debugger := NewNodeDebugger(r.verbose)
	debugger.session = r.session

	return debugger.stepOver()
}

// stepOut steps out of the current function
func (r *REPL) stepOut(ctx context.Context) error {
	if r.session == nil {
		return errors.New("not connected to any target")
	}

	debugger := NewNodeDebugger(r.verbose)
	debugger.session = r.session

	return debugger.stepOut()
}

// pauseExecution pauses JavaScript execution
func (r *REPL) pauseExecution(ctx context.Context) error {
	if r.session == nil {
		return errors.New("not connected to any target")
	}

	// Execute debugger statement to pause
	_, err := r.session.Execute(ctx, "debugger")
	return err
}

// inspectElement inspects a DOM element (Chrome only)
func (r *REPL) inspectElement(ctx context.Context, selector string) error {
	if r.session == nil {
		return errors.New("not connected to any target")
	}

	if r.session.Target.Type != SessionTypeChrome {
		return errors.New("DOM inspection only available for Chrome targets")
	}

	// Simple DOM inspection
	expression := fmt.Sprintf(`
		(function() {
			const el = document.querySelector('%s');
			if (!el) return 'Element not found';
			return {
				tagName: el.tagName,
				id: el.id,
				className: el.className,
				textContent: el.textContent.substring(0, 100),
				attributes: Array.from(el.attributes).map(a => a.name + '=' + a.value)
			};
		})()
	`, selector)

	return r.executeJavaScript(ctx, expression)
}

// navigate navigates to a URL (Chrome only)
func (r *REPL) navigate(ctx context.Context, url string) error {
	if r.session == nil {
		return errors.New("not connected to any target")
	}

	if r.session.Target.Type != SessionTypeChrome {
		return errors.New("navigation only available for Chrome targets")
	}

	expression := fmt.Sprintf("window.location.href = '%s'", url)
	_, err := r.session.Execute(ctx, expression)

	if err == nil {
		fmt.Printf("✓ Navigated to: %s\n", url)
	}

	return err
}

// takeScreenshot takes a screenshot (Chrome only)
func (r *REPL) takeScreenshot(ctx context.Context, filename string) error {
	if r.session == nil {
		return errors.New("not connected to any target")
	}

	if r.session.Target.Type != SessionTypeChrome {
		return errors.New("screenshots only available for Chrome targets")
	}

	// TODO: Implement actual screenshot functionality
	fmt.Printf("Screenshot functionality not yet implemented: %s\n", filename)

	return nil
}

// showHistory shows command history
func (r *REPL) showHistory() {
	if len(r.history) == 0 {
		fmt.Println("No command history")
		return
	}

	fmt.Println("\nCommand History:")
	fmt.Println("================")

	start := 0
	if len(r.history) > 20 {
		start = len(r.history) - 20
		fmt.Println("(showing last 20 commands)")
	}

	for i := start; i < len(r.history); i++ {
		fmt.Printf("%3d  %s\n", i+1, r.history[i])
	}

	fmt.Println()
}