package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"errors"

	"github.com/chromedp/cdproto/debugger"
	"github.com/chromedp/cdproto/domdebugger"
	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/cdproto/runtime"
	"github.com/chromedp/chromedp"
	"github.com/gorilla/websocket"
	"golang.org/x/tools/txtar"

	"github.com/fsnotify/fsnotify"
	"github.com/go-sourcemap/sourcemap"
)

// REPL represents an interactive Read-Eval-Print Loop
type REPL struct {
	session     *Session
	manager     *SessionManager
	verbose     bool
	history     []string
	running     bool
	pausedState *debugger.EventPaused

	// I/O
	in  io.Reader
	out io.Writer

	// Raw WebSocket fields
	rawConn   *websocket.Conn
	msgID     int64
	responses map[int64]chan map[string]interface{}
	mu        sync.Mutex

	// Source Maps
	sourceMaps map[string]*sourcemap.Consumer // scriptID -> Consumer
	scriptURLs map[string]string              // scriptID -> URL

	// Workspace
	watcher *fsnotify.Watcher

	// Network
	networkEnabled bool

	// DevTools Proxy
	devtoolsConn  *websocket.Conn
	proxyListener net.Listener
}

// NewREPL creates a new REPL instance
func NewREPL(verbose bool) *REPL {
	watcher, _ := fsnotify.NewWatcher() // Ignore error for now, purely optional

	r := &REPL{
		manager:    NewSessionManager(verbose),
		verbose:    verbose,
		history:    []string{},
		running:    false,
		in:         os.Stdin,
		out:        os.Stdout,
		responses:  make(map[int64]chan map[string]interface{}),
		sourceMaps: make(map[string]*sourcemap.Consumer),
		scriptURLs: make(map[string]string),
		watcher:    watcher,
	}

	if watcher != nil {
		go r.watchLoop()
	}

	return r
}

func (r *REPL) SetOutput(w io.Writer) {
	r.out = w
}

func (r *REPL) print(a ...interface{}) {
	fmt.Fprint(r.out, a...)
}

func (r *REPL) println(a ...interface{}) {
	fmt.Fprintln(r.out, a...)
}

func (r *REPL) printf(format string, a ...interface{}) {
	fmt.Fprintf(r.out, format, a...)
}

// Start starts the interactive REPL
func (r *REPL) Start(ctx context.Context, targetID string) error {
	r.println("NDP Interactive REPL")
	r.println("====================")

	// Connect to target if specified
	if targetID != "" {
		if err := r.connectToTarget(ctx, targetID); err != nil {
			return fmt.Errorf("failed to connect to target: %w", err)
		}
	} else {
		// List available targets
		if err := r.showTargets(ctx); err != nil {
			return fmt.Errorf("failed to list targets: %w", err)
		}
	}

	r.running = true
	r.showHelp()

	scanner := bufio.NewScanner(r.in) // Use r.in

	for r.running {
		r.print("ndp> ")

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
			r.printf("Error: %v\n", err)
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("input error: %w", err)
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

	case "break-xhr", "bx":
		urlPattern := ""
		if len(args) > 0 {
			urlPattern = args[0]
		}
		return r.setXHRBreakpoint(ctx, urlPattern)

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

	case "backtrace", "bt":
		r.printBacktrace()

	case "vars", "locals":
		return r.printVars(ctx)

	case "args":
		return r.printArgs(ctx)

	case "sources":
		r.dumpSources()

	case "devtools":
		r.openDevTools()

	case "network":
		return r.toggleNetwork(ctx, args)

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
	r.println("\nNDP REPL Commands:")
	r.println("==================")
	r.println("Connection:")
	r.println("  targets, list         - List available debug targets")
	r.println("  connect <num>         - Connect to target by number")
	r.println("  disconnect            - Disconnect from current target")
	r.println("  status                - Show connection status")
	r.println()
	r.println("Execution:")
	r.println("  exec <js>             - Execute JavaScript expression")
	r.println("  navigate <url>        - Navigate to URL (Chrome only)")
	r.println("  inspect <selector>    - Inspect DOM element (Chrome only)")
	r.println("  screenshot [file]     - Take screenshot (Chrome only)")
	r.println()
	r.println("Debugging:")
	r.println("  break <file:line> [condition] - Set breakpoint")
	r.println("  break-xhr, bx <url>   - Break on XHR/Fetch URL match")
	r.println("  breakpoints, bps      - List breakpoints")
	r.println("  continue, c           - Continue execution")
	r.println("  step, s               - Step into")
	r.println("  next, n               - Step over")
	r.println("  out, o                - Step out")
	r.println("  pause                 - Pause execution")
	r.println("  backtrace, bt         - Print stack trace (supports Source Maps)")
	r.println("  vars, locals          - Print local variables")
	r.println("  args                  - Print arguments")
	r.println("  sources               - Dump all script sources (txtar format)")
	r.println("  devtools              - Launch Chrome DevTools for current session")
	r.println("  network <on|off>      - Toggle Network domain inspection")
	r.println()
	r.println("Utility:")
	r.println("  history               - Show command history")
	r.println("  clear                 - Clear screen")
	r.println("  help, h, ?            - Show this help")
	r.println("  exit, quit, q         - Exit REPL")
	r.println()
	r.println("Note: Any unrecognized command will be executed as JavaScript if connected.")
	r.println()
}

// showTargets lists available debug targets
func (r *REPL) showTargets(ctx context.Context) error {
	targets, err := r.manager.ListTargets(ctx)
	if err != nil {
		return err
	}

	if len(targets) == 0 {
		r.println("No debug targets found.")
		r.println("Make sure you have:")
		r.println("  - Node.js running with --inspect flag")
		r.println("  - Chrome running with --remote-debugging-port flag")
		return nil
	}

	r.printf("\nAvailable targets (%d):\n", len(targets))
	r.println("========================")

	for i, target := range targets {
		status := ""
		if r.session != nil && r.session.Target.ID == target.ID {
			status = " [CONNECTED]"
		}

		r.printf("[%d] %s - %s%s\n", i+1, target.Type, target.Title, status)
		r.printf("    URL: %s\n", target.URL)
		r.printf("    Port: %s\n", target.Port)
		r.println()
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

	// For Node.js, use Raw WebSocket connection to avoid Target domain issues
	if target.Type == SessionTypeNode {
		if err := r.connectToTargetRaw(ctx, target); err != nil {
			return err
		}
		fmt.Printf("✓ Connected to %s: %s (Raw WS)\n", target.Type, target.Title)
		return nil
	}

	// Enable necessary domains via Chromedp for Chrome
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

	// Setup event listeners for Chrome
	chromedp.ListenTarget(session.ChromeCtx, func(ev interface{}) {
		switch ev := ev.(type) {
		case *debugger.EventPaused:
			r.pausedState = ev
			fmt.Printf("\n[Debugger Paused] Reason: %s\n", ev.Reason)
			if len(ev.CallFrames) > 0 {
				cf := ev.CallFrames[0]
				fmt.Printf("  at %s (%s:%d:%d)\n", cf.FunctionName, cf.Location.ScriptID, cf.Location.LineNumber+1, cf.Location.ColumnNumber+1)
			}
			fmt.Print("ndp> ")
		case *debugger.EventResumed:
			r.pausedState = nil
			fmt.Printf("\n[Debugger Resumed]\nndp> ")

		case *runtime.EventConsoleAPICalled:
			args := make([]string, len(ev.Args))
			for i, arg := range ev.Args {
				val := string(arg.Value)
				if val == "" && arg.Description != "" {
					val = arg.Description
				}
				args[i] = val
			}
			fmt.Printf("\n[Console] %s\nndp> ", strings.Join(args, " "))
		}
	})

	return nil
}

// disconnect disconnects from the current target
func (r *REPL) disconnect() error {
	if r.session == nil {
		fmt.Println("Not connected to any target")
		return nil
	}

	// Close raw conn if exists
	if r.rawConn != nil {
		r.rawConn.Close()
		r.rawConn = nil
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

	connType := ""
	if r.rawConn != nil {
		connType = " (Raw WS)"
	}

	fmt.Printf("Status: Connected to %s%s\n", r.session.Target.Type, connType)
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

	// Raw WS path
	if r.rawConn != nil {
		p := runtime.Evaluate(expression)
		b, _ := json.Marshal(p)
		var params map[string]interface{}
		json.Unmarshal(b, &params)

		res, err := r.sendRequest("Runtime.evaluate", params)
		if err != nil {
			return err
		}

		// Parse result
		if resVal, ok := res["result"].(map[string]interface{}); ok {
			// Extract value
			if val, ok := resVal["value"]; ok {
				fmt.Printf("→ %v\n", val)
			} else if desc, ok := resVal["description"]; ok {
				fmt.Printf("→ %s\n", desc)
			} else {
				fmt.Printf("→ %v\n", resVal)
			}
		}
		return nil
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

	parts := strings.Split(location, ":")
	if len(parts) != 2 {
		return errors.New("invalid location format, expected file:line")
	}
	file := parts[0]
	line, err := strconv.Atoi(parts[1])
	if err != nil {
		return fmt.Errorf("invalid line number: %v", err)
	}

	var bId string

	// Raw WS Path
	if r.rawConn != nil {
		p := debugger.SetBreakpointByURL(int64(line - 1))
		p.URLRegex = fmt.Sprintf(".*%s", file)
		p.Condition = condition
		b, _ := json.Marshal(p)
		var params map[string]interface{}
		json.Unmarshal(b, &params)

		res, err := r.sendRequest("Debugger.setBreakpointByUrl", params)
		if err != nil {
			return err
		}

		if id, ok := res["breakpointId"].(string); ok {
			bId = id
		}
	} else {
		// Chrome Path
		err = chromedp.Run(r.session.ChromeCtx,
			chromedp.ActionFunc(func(ctx context.Context) error {
				id, _, err := debugger.SetBreakpointByURL(int64(line - 1)).
					WithURLRegex(fmt.Sprintf(".*%s", file)).
					WithCondition(condition).
					Do(ctx)
				if err != nil {
					return err
				}
				bId = string(id)
				return nil
			}),
		)
		if err != nil {
			return err
		}
	}

	fmt.Printf("Breakpoint set: %s at %s:%d\n", bId, file, line)
	return nil
}

// setXHRBreakpoint sets an XHR breakpoint
func (r *REPL) setXHRBreakpoint(ctx context.Context, urlPattern string) error {
	if r.session == nil {
		return errors.New("not connected to any target")
	}

	// Raw WS Path
	if r.rawConn != nil {
		p := domdebugger.SetXHRBreakpoint(urlPattern)
		b, _ := json.Marshal(p)
		var params map[string]interface{}
		json.Unmarshal(b, &params)

		_, err := r.sendRequest("DOMDebugger.setXHRBreakpoint", params)
		if err != nil {
			return err
		}
	} else {
		err := chromedp.Run(r.session.ChromeCtx,
			domdebugger.SetXHRBreakpoint(urlPattern),
		)
		if err != nil {
			return err
		}
	}
	fmt.Printf("XHR breakpoint set for pattern: %q\n", urlPattern)
	return nil
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

	if r.rawConn != nil {
		_, err := r.sendRequest("Debugger.resume", nil)
		return err
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

	if r.rawConn != nil {
		_, err := r.sendRequest("Debugger.stepInto", nil)
		return err
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

	if r.rawConn != nil {
		_, err := r.sendRequest("Debugger.stepOver", nil)
		return err
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

	if r.rawConn != nil {
		_, err := r.sendRequest("Debugger.stepOut", nil)
		return err
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

	if r.rawConn != nil {
		_, err := r.sendRequest("Debugger.pause", nil)
		return err
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

// printBacktrace prints the current stack trace
func (r *REPL) printBacktrace() {
	if r.pausedState == nil {
		fmt.Println("Not paused")
		return
	}

	frames := r.pausedState.CallFrames
	fmt.Printf("Stack Trace (%d frames):\n", len(frames))
	for i, frame := range frames {
		loc := fmt.Sprintf("%s:%d:%d", frame.Location.ScriptID, frame.Location.LineNumber+1, frame.Location.ColumnNumber+1)

		// Attempt source map resolution
		r.mu.Lock()
		consumer, ok := r.sourceMaps[string(frame.Location.ScriptID)]
		r.mu.Unlock()

		if ok {
			// CDP lines are 0-based, sourcemap expects 1-based?
			// go-sourcemap Source(line, column)
			// checking standard usage: commonly 1-based line.
			source, name, line, col, ok := consumer.Source(int(frame.Location.LineNumber)+1, int(frame.Location.ColumnNumber))
			if ok {
				sourceLoc := fmt.Sprintf("%s:%d:%d", source, line, col)
				if name != "" {
					sourceLoc += " (" + name + ")"
				}
				fmt.Printf("#%d %s at %s (gen: %s)\n", i, frame.FunctionName, sourceLoc, loc)
				continue
			}
		}

		fmt.Printf("#%d %s at %s\n", i, frame.FunctionName, loc)
	}
}

// printVars prints local variables
func (r *REPL) printVars(ctx context.Context) error {
	if r.pausedState == nil {
		return errors.New("not paused")
	}

	if len(r.pausedState.CallFrames) == 0 {
		return errors.New("no call frames")
	}

	frame := r.pausedState.CallFrames[0]

	// Find local scope
	var localScope *debugger.Scope
	for _, scope := range frame.ScopeChain {
		if scope.Type == "local" {
			localScope = scope
			break
		}
	}

	if localScope == nil {
		fmt.Println("No local scope found")
		return nil
	}

	// Inspect content
	return r.printScopeContent(ctx, localScope, "Local Variables")
}

// printArgs prints arguments
func (r *REPL) printArgs(ctx context.Context) error {
	if r.pausedState == nil {
		return errors.New("not paused")
	}

	// In JS, arguments are often just locals. We interpret this request as inspecting local scope
	// or potentially "Closure" scopes if meaningful. For now, aliases to vars.
	fmt.Println("(Note: Arguments are typically included in Local Variables in JS)")
	return r.printVars(ctx)
}

func (r *REPL) printScopeContent(ctx context.Context, scope *debugger.Scope, label string) error {
	if r.session == nil {
		return errors.New("not connected")
	}

	var props []*runtime.PropertyDescriptor
	var err error

	// Raw WS Path
	if r.rawConn != nil {
		p := runtime.GetProperties(scope.Object.ObjectID)
		p.OwnProperties = true
		b, _ := json.Marshal(p)
		var params map[string]interface{}
		json.Unmarshal(b, &params)

		res, err := r.sendRequest("Runtime.getProperties", params)
		if err != nil {
			return err
		}

		propsBytes, _ := json.Marshal(res["result"])
		if err := json.Unmarshal(propsBytes, &props); err != nil {
			return fmt.Errorf("failed to unmarshal properties: %w", err)
		}
	} else {
		// Chrome Path
		err = chromedp.Run(r.session.ChromeCtx,
			chromedp.ActionFunc(func(ctx context.Context) error {
				res, _, _, _, err := runtime.GetProperties(scope.Object.ObjectID).
					WithOwnProperties(true).
					Do(ctx)
				props = res
				return err
			}),
		)
		if err != nil {
			return err
		}
	}

	fmt.Printf("%s:\n", label)
	for _, prop := range props {
		val := "undefined"
		if prop.Value != nil {
			val = string(prop.Value.Description)
			if val == "" {
				val = string(prop.Value.Value)
			}
		}
		fmt.Printf("  %s = %s\n", prop.Name, val)
	}

	return nil
}

// sendRequest sends a raw CDP request and waits for the response
func (r *REPL) sendRequest(method string, params interface{}) (map[string]interface{}, error) {
	if r.rawConn == nil {
		return nil, errors.New("no active connection")
	}

	r.mu.Lock()
	r.msgID++
	id := r.msgID
	ch := make(chan map[string]interface{}, 1)
	r.responses[id] = ch
	r.mu.Unlock()

	defer func() {
		r.mu.Lock()
		delete(r.responses, id)
		r.mu.Unlock()
	}()

	req := map[string]interface{}{
		"id":     id,
		"method": method,
		"params": params,
	}

	if err := r.rawConn.WriteJSON(req); err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}

	select {
	case res := <-ch:
		if errObj, ok := res["error"]; ok {
			return nil, fmt.Errorf("CDP error: %v", errObj)
		}
		if result, ok := res["result"].(map[string]interface{}); ok {
			return result, nil
		}
		return map[string]interface{}{}, nil
	case <-time.After(10 * time.Second):
		return nil, errors.New("request timed out")
	}
}

// connectToTargetRaw connects to a target using raw WebSocket
func (r *REPL) connectToTargetRaw(ctx context.Context, target *DebugTarget) error {
	if target.WebSocketDebuggerURL == "" {
		return errors.New("target has no WebSocket URL")
	}

	// Dial WebSocket
	conn, _, err := websocket.DefaultDialer.Dial(target.WebSocketDebuggerURL, nil)
	if err != nil {
		return fmt.Errorf("failed to dial WebSocket: %w", err)
	}

	r.rawConn = conn

	// Start read loop
	go r.readLoop()

	// Enable domains
	if _, err := r.sendRequest("Runtime.enable", nil); err != nil {
		return fmt.Errorf("failed to enable Runtime: %w", err)
	}
	if _, err := r.sendRequest("Debugger.enable", nil); err != nil {
		return fmt.Errorf("failed to enable Debugger: %w", err)
	}

	return nil
}

// readLoop reads messages from the WebSocket
func (r *REPL) readLoop() {
	for {
		_, msg, err := r.rawConn.ReadMessage()
		if err != nil {
			// Don't log if expected close
			if r.running && r.verbose {
				fmt.Printf("Read error: %v\n", err)
			}
			r.running = false
			return
		}

		// Forward to DevTools frontend if connected
		r.mu.Lock()
		if r.devtoolsConn != nil {
			if err := r.devtoolsConn.WriteMessage(websocket.TextMessage, msg); err != nil {
				if r.verbose {
					fmt.Printf("DevTools write error: %v\n", err)
				}
				r.devtoolsConn.Close()
				r.devtoolsConn = nil
			}
		}
		r.mu.Unlock()

		var m map[string]interface{}
		if err := json.Unmarshal(msg, &m); err != nil {
			if r.verbose {
				fmt.Printf("JSON unmarshal error: %v\n", err)
			}
			continue
		}

		// Handle Response
		if idVal, ok := m["id"].(float64); ok {
			id := int64(idVal)
			r.mu.Lock()
			if ch, ok := r.responses[id]; ok {
				ch <- m
			}
			r.mu.Unlock()
			continue
		}

		// Handle Event
		if method, ok := m["method"].(string); ok {
			// Extract params properly
			var params json.RawMessage
			if p, ok := m["params"]; ok {
				// Re-marshal params to RawMessage? Efficient?
				// Better: unmarshal m with params as RawMessage?
				// For now simple way:
				b, _ := json.Marshal(p)
				params = b
			}
			r.handleEvent(method, params)
		}
	}
}

// handleEvent handles CDP events
func (r *REPL) handleEvent(method string, params interface{}) {
	switch method {
	case "Debugger.paused":
		// manually unmarshal params to event
		b, _ := json.Marshal(params)
		var ev debugger.EventPaused
		json.Unmarshal(b, &ev)

		r.pausedState = &ev
		fmt.Printf("\n[Debugger Paused] Reason: %s\n", ev.Reason)
		if len(ev.CallFrames) > 0 {
			cf := ev.CallFrames[0]
			fmt.Printf("  at %s (%s:%d:%d)\n", cf.FunctionName, cf.Location.ScriptID, cf.Location.LineNumber+1, cf.Location.ColumnNumber+1)
		}
		fmt.Print("ndp> ")

	case "Debugger.resumed":
		r.pausedState = nil
		fmt.Printf("\n[Debugger Resumed]\nndp> ")

	case "Runtime.consoleAPICalled":
		b, _ := json.Marshal(params)
		var ev runtime.EventConsoleAPICalled
		json.Unmarshal(b, &ev)

		args := make([]string, len(ev.Args))
		for i, arg := range ev.Args {
			val := string(arg.Value)
			if val == "" && arg.Description != "" {
				val = arg.Description
			}
			args[i] = val
		}
		fmt.Printf("\n[Console] %s\nndp> ", strings.Join(args, " "))

	case "Debugger.scriptParsed":
		b, _ := json.Marshal(params)
		var ev debugger.EventScriptParsed
		json.Unmarshal(b, &ev)

		r.mu.Lock()
		r.scriptURLs[string(ev.ScriptID)] = ev.URL
		r.mu.Unlock()

		if strings.HasPrefix(ev.URL, "file://") && r.watcher != nil {
			path := strings.TrimPrefix(ev.URL, "file://")
			if err := r.watcher.Add(path); err != nil {
				if r.verbose {
					fmt.Printf("Failed to watch %s: %v\n", path, err)
				}
			} else if r.verbose {
				fmt.Printf("Watching %s for changes\n", path)
			}
		}

		if ev.SourceMapURL != "" {
			go r.loadSourceMap(string(ev.ScriptID), ev.URL, ev.SourceMapURL)
		}

	case "Network.requestWillBeSent":
		if !r.networkEnabled {
			return
		}
		b, _ := json.Marshal(params)
		var ev network.EventRequestWillBeSent
		json.Unmarshal(b, &ev)
		fmt.Printf("[Network] -> %s %s\n", ev.Request.Method, ev.Request.URL)

	case "Network.responseReceived":
		if !r.networkEnabled {
			return
		}
		b, _ := json.Marshal(params)
		var ev network.EventResponseReceived
		json.Unmarshal(b, &ev)
		fmt.Printf("[Network] <- %s (%d) %s\n", ev.Response.MimeType, ev.Response.Status, ev.Response.URL)
	}
}

// loadSourceMap fetches and parses a source map
func (r *REPL) loadSourceMap(scriptID string, scriptURL string, sourceMapURL string) {
	if sourceMapURL == "" {
		return
	}

	// Resolve absolute URL
	var absoluteURL string
	if strings.HasPrefix(sourceMapURL, "data:") {
		// TODO: Handle data URIs
		return
	}

	if strings.Contains(sourceMapURL, "://") {
		absoluteURL = sourceMapURL
	} else {
		// Resolve relative to script URL
		u, err := url.Parse(scriptURL)
		if err != nil {
			if r.verbose {
				fmt.Printf("Failed to parse script URL %s: %v\n", scriptURL, err)
			}
			return
		}
		base := u.ResolveReference(&url.URL{Path: sourceMapURL})
		absoluteURL = base.String()
	}

	var content []byte
	var err error

	if strings.HasPrefix(absoluteURL, "file://") {
		path := strings.TrimPrefix(absoluteURL, "file://")
		content, err = os.ReadFile(path)
	} else {
		resp, httpErr := http.Get(absoluteURL)
		if httpErr != nil {
			err = httpErr
		} else {
			defer resp.Body.Close()
			content, err = io.ReadAll(resp.Body)
		}
	}

	if err != nil {
		if r.verbose {
			fmt.Printf("Failed to load source map %s: %v\n", absoluteURL, err)
		}
		return
	}

	consumer, err := sourcemap.Parse(absoluteURL, content)
	if err != nil {
		if r.verbose {
			fmt.Printf("Failed to parse source map %s: %v\n", absoluteURL, err)
		}
		return
	}

	r.mu.Lock()
	r.sourceMaps[scriptID] = consumer
	r.mu.Unlock()

	if r.verbose {
		fmt.Printf("Loaded source map for %s\n", scriptURL)
	}
}

// dumpSources outputs all known scripts in txtar format
func (r *REPL) dumpSources() {
	r.mu.Lock()
	scripts := make(map[string]string)
	for id, url := range r.scriptURLs {
		scripts[id] = url
	}
	r.mu.Unlock()

	var archive txtar.Archive

	for id, url := range scripts {
		if r.rawConn != nil {
			params := map[string]interface{}{"scriptId": id}
			res, err := r.sendRequest("Debugger.getScriptSource", params)
			if err != nil {
				if r.verbose {
					fmt.Printf("Failed to get source for %s: %v\n", url, err)
				}
				continue
			}
			if src, ok := res["scriptSource"].(string); ok {
				archive.Files = append(archive.Files, txtar.File{
					Name: url,
					Data: []byte(src),
				})
			}
		} else {
			// Chrome path (using chromedp)
			// TODO: Implement Chrome path if needed, requires keeping track of context or using separate action
			if r.verbose {
				fmt.Println("Sources dump not fully implemented for Chrome targets yet (requires script tracking)")
			}
		}
	}

	fmt.Print(string(txtar.Format(&archive)))
}

func (r *REPL) watchLoop() {
	if r.watcher == nil {
		return
	}
	defer r.watcher.Close()
	for {
		select {
		case event, ok := <-r.watcher.Events:
			if !ok {
				return
			}
			if event.Has(fsnotify.Write) {
				if r.verbose {
					fmt.Printf("File changed: %s\n", event.Name)
				}
				r.reloadScript(event.Name)
			}
		case err, ok := <-r.watcher.Errors:
			if !ok {
				return
			}
			if r.verbose {
				fmt.Printf("Watcher error: %v\n", err)
			}
		}
	}
}

func (r *REPL) reloadScript(path string) {
	// Simple heuristic for file URL construction
	// In a real app we might need exact matching of what node reported.
	// But usually we just check if any known script URL (which is file://) matches this path.

	content, err := os.ReadFile(path)
	if err != nil {
		if r.verbose {
			fmt.Printf("Failed to read changed file %s: %v\n", path, err)
		}
		return
	}

	r.mu.Lock()
	var ids []string
	for id, url := range r.scriptURLs {
		if strings.HasPrefix(url, "file://") {
			fPath := strings.TrimPrefix(url, "file://")
			if fPath == path {
				ids = append(ids, id)
			}
		}
	}
	r.mu.Unlock()

	for _, id := range ids {
		if r.verbose {
			fmt.Printf("Reloading script %s (ID: %s)\n", path, id)
		}

		if r.rawConn != nil {
			params := map[string]interface{}{
				"scriptId":     id,
				"scriptSource": string(content),
			}
			_, err := r.sendRequest("Debugger.setScriptSource", params)
			if err != nil {
				fmt.Printf("Failed to reload script %s: %v\n", path, err)
			} else {
				fmt.Printf("Hot Reloaded: %s\n", filepath.Base(path))
			}
		} else {
			// Chrome path would go here
		}
	}
}

func (r *REPL) openDevTools() {
	if r.session == nil || r.session.Target.WebSocketDebuggerURL == "" {
		fmt.Println("No active session or URL not available")
		return
	}

	// Start Proxy Server
	proxyAddr, err := r.startProxyServer()
	if err != nil {
		fmt.Printf("Failed to start proxy server: %v\n", err)
		return
	}

	// Construct DevTools URL pointing to Proxy
	// ws param should be host:port (no scheme)
	dtURL := fmt.Sprintf("devtools://devtools/bundled/inspector.html?experiments=true&v8only=true&ws=%s", proxyAddr)

	if r.verbose {
		fmt.Printf("Opening DevTools (via Proxy %s): %s\n", proxyAddr, dtURL)
	}

	// Launch Chrome (MacOS)
	cmd := exec.Command("open", "-a", "Google Chrome", "--args", "--new-window", dtURL)

	if err := cmd.Start(); err != nil {
		fmt.Printf("Failed to launch DevTools: %v\n", err)
		fmt.Printf("URL: %s\n", dtURL)
	} else {
		fmt.Println("DevTools launched (attached via REPL Proxy)")
	}
}

func (r *REPL) toggleNetwork(ctx context.Context, args []string) error {
	enable := !r.networkEnabled
	if len(args) > 0 {
		switch args[0] {
		case "on", "enable", "true":
			enable = true
		case "off", "disable", "false":
			enable = false
		}
	}

	r.networkEnabled = enable

	if r.verbose {
		fmt.Printf("Network logging set to %v\n", enable)
	}

	method := "Network.enable"
	if !enable {
		method = "Network.disable"
	}

	if r.rawConn != nil {
		if _, err := r.sendRequest(method, nil); err != nil {
			return fmt.Errorf(fmt.Sprintf("failed to %s", method)+": %w", err)
		}
	} else if r.session != nil {
		// Chrome path
		err := chromedp.Run(r.session.ChromeCtx,
			chromedp.ActionFunc(func(ctx context.Context) error {
				if enable {
					return network.Enable().Do(ctx)
				}
				return network.Disable().Do(ctx)
			}),
		)
		if err != nil {
			return err
		}
	}

	if enable {
		fmt.Println("Network inspection enabled")
	} else {
		fmt.Println("Network inspection disabled")
	}
	return nil
}

var proxyUpgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

func (r *REPL) startProxyServer() (string, error) {
	if r.proxyListener != nil {
		return r.proxyListener.Addr().String(), nil
	}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", err
	}
	r.proxyListener = ln

	go http.Serve(ln, http.HandlerFunc(r.handleProxyConnection))

	return ln.Addr().String(), nil
}

func (r *REPL) handleProxyConnection(w http.ResponseWriter, req *http.Request) {
	ws, err := proxyUpgrader.Upgrade(w, req, nil)
	if err != nil {
		if r.verbose {
			fmt.Printf("Proxy upgrade failed: %v\n", err)
		}
		return
	}

	r.mu.Lock()
	if r.devtoolsConn != nil {
		r.devtoolsConn.Close()
	}
	r.devtoolsConn = ws
	r.mu.Unlock()

	defer func() {
		r.mu.Lock()
		if r.devtoolsConn == ws {
			r.devtoolsConn = nil
		}
		r.mu.Unlock()
		ws.Close()
	}()

	if r.verbose {
		fmt.Println("DevTools connected to Proxy")
	}

	for {
		_, msg, err := ws.ReadMessage()
		if err != nil {
			if r.verbose {
				fmt.Printf("Proxy read error: %v\n", err)
			}
			break
		}

		// Intercept Debugger.setScriptSource
		// Inspect message
		var m map[string]interface{}
		// We ignore unmarshal errors here for speed/robustness unless we need to act
		if err := json.Unmarshal(msg, &m); err == nil {
			if method, ok := m["method"].(string); ok && method == "Debugger.setScriptSource" {
				// Intercepted!
				if r.verbose {
					fmt.Println("Intercepted Debugger.setScriptSource!")
				}
				go r.handleSetScriptSource(m)
			}
		}

		// Forward to Target
		r.mu.Lock()
		if r.rawConn != nil {
			if err := r.rawConn.WriteMessage(websocket.TextMessage, msg); err != nil {
				if r.verbose {
					fmt.Printf("Target write error: %v\n", err)
				}
			}
		}
		r.mu.Unlock()
	}
}

func (r *REPL) handleSetScriptSource(m map[string]interface{}) {
	params, ok := m["params"].(map[string]interface{})
	if !ok {
		return
	}

	scriptID, _ := params["scriptId"].(string)
	source, _ := params["scriptSource"].(string)

	if scriptID == "" || source == "" {
		return
	}

	r.mu.Lock()
	url, ok := r.scriptURLs[scriptID]
	r.mu.Unlock()

	if !ok {
		if r.verbose {
			fmt.Printf("Unknown scriptID %s, cannot save\n", scriptID)
		}
		return
	}

	if strings.HasPrefix(url, "file://") {
		path := strings.TrimPrefix(url, "file://")
		if r.verbose {
			fmt.Printf("Writing changes to %s...\n", path)
		}
		if err := os.WriteFile(path, []byte(source), 0644); err != nil {
			fmt.Printf("Failed to write file %s: %v\n", path, err)
		} else {
			fmt.Printf("Saved %s\n", path)
		}
	} else {
		if r.verbose {
			fmt.Printf("Skipping non-file URL: %s\n", url)
		}
	}
}
