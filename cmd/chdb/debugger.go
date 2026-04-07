package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"errors"

	"github.com/chromedp/cdproto/debugger"
	"github.com/chromedp/cdproto/domdebugger"
	"github.com/chromedp/cdproto/runtime"
	"github.com/chromedp/chromedp"
)

// DebuggerController handles all debugging operations
type DebuggerController struct {
	debugger *ChromeDebugger
	verbose  bool

	// Breakpoint tracking
	breakpoints map[string]*Breakpoint

	// Execution state
	paused    bool
	callStack []debugger.CallFrame

	// Watch expressions
	watches []string
}

// Breakpoint represents a breakpoint in the debugger
type Breakpoint struct {
	ID           string `json:"id"`
	URL          string `json:"url,omitempty"`
	LineNumber   int    `json:"line"`
	ColumnNumber int    `json:"column,omitempty"`
	Condition    string `json:"condition,omitempty"`
	Type         string `json:"type"` // "line", "conditional", "logpoint", "dom", "xhr"
	Enabled      bool   `json:"enabled"`
}

// NewDebuggerController creates a new debugger controller
func NewDebuggerController(debugger *ChromeDebugger, verbose bool) *DebuggerController {
	return &DebuggerController{
		debugger:    debugger,
		verbose:     verbose,
		breakpoints: make(map[string]*Breakpoint),
		watches:     []string{},
	}
}

// SetBreakpoint sets a breakpoint at the specified location
func (dc *DebuggerController) SetBreakpoint(ctx context.Context, location string, condition string) (*Breakpoint, error) {
	if !dc.debugger.connected {
		return nil, errors.New("not connected to Chrome")
	}

	// Parse location (format: file:line or file:line:column or URL:line)
	parts := strings.Split(location, ":")
	if len(parts) < 2 {
		return nil, errors.New("invalid location format, use file:line or URL:line")
	}

	var url string
	var lineNumber int
	var columnNumber int

	// Determine if it's a file path or URL
	if strings.HasPrefix(parts[0], "http") || strings.HasPrefix(parts[0], "file:") {
		url = parts[0] + ":" + parts[1] // Reconstruct URL
		if len(parts) > 2 {
			fmt.Sscanf(parts[2], "%d", &lineNumber)
		}
		if len(parts) > 3 {
			fmt.Sscanf(parts[3], "%d", &columnNumber)
		}
	} else {
		// It's a file path
		url = parts[0]
		fmt.Sscanf(parts[1], "%d", &lineNumber)
		if len(parts) > 2 {
			fmt.Sscanf(parts[2], "%d", &columnNumber)
		}
	}

	// Set breakpoint using CDP
	var breakpointID debugger.BreakpointID
	err := chromedp.Run(dc.debugger.chromeCtx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			// First ensure debugger is enabled
			if _, err := debugger.Enable().Do(ctx); err != nil {
				return err
			}

			// Set breakpoint by URL
			if url != "" {
				params := debugger.SetBreakpointByURL(int64(lineNumber)).
					WithURL(url)

				if columnNumber > 0 {
					params = params.WithColumnNumber(int64(columnNumber))
				}

				if condition != "" {
					params = params.WithCondition(condition)
				}

				result, _, err := params.Do(ctx)
				if err != nil {
					return err
				}

				breakpointID = result
				return nil
			}

			return errors.New("unable to set breakpoint")
		}),
	)

	if err != nil {
		return nil, fmt.Errorf("failed to set breakpoint: %w", err)
	}

	// Store breakpoint
	bp := &Breakpoint{
		ID:           string(breakpointID),
		URL:          url,
		LineNumber:   lineNumber,
		ColumnNumber: columnNumber,
		Condition:    condition,
		Type:         "line",
		Enabled:      true,
	}

	if condition != "" {
		bp.Type = "conditional"
	}

	dc.breakpoints[string(breakpointID)] = bp

	return bp, nil
}

// SetXHRBreakpoint sets a breakpoint on XHR/Fetch URL
func (dc *DebuggerController) SetXHRBreakpoint(ctx context.Context, urlPattern string) error {
	if !dc.debugger.connected {
		return errors.New("not connected to Chrome")
	}

	err := chromedp.Run(dc.debugger.chromeCtx,
		domdebugger.SetXHRBreakpoint(urlPattern),
	)

	if err != nil {
		return fmt.Errorf("failed to set XHR breakpoint: %w", err)
	}

	dc.breakpoints["xhr:"+urlPattern] = &Breakpoint{
		ID:        "xhr:" + urlPattern,
		Condition: urlPattern,
		Type:      "xhr",
		Enabled:   true,
	}

	return nil
}

// SetDOMBreakpoint sets a breakpoint on DOM modification
func (dc *DebuggerController) SetDOMBreakpoint(ctx context.Context, nodeID int64, typeVal string) error {
	if !dc.debugger.connected {
		return errors.New("not connected to Chrome")
	}

	// We need cdproto.NodeID
	// The caller should have resolved selector to ID already, or we do it here?
	// Let's assume we pass in the ID for now, or use a helper.
	// Actually, easier to use domdebugger.SetDOMBreakpoint directly with Action.

	// Wait, domdebugger.SetDOMBreakpoint takes cdp.NodeID.
	// We'll need cdp import if we use strict types, or use int64 and cast.
	// Importing cdp package might be cleaner.

	// For now, let's keep it simple and assume standard integration.

	// To avoid import cycles or new imports, let's assume we can cast.
	// But cdp.NodeID is strict.
	// Let's rely on the caller or `dom` package.

	return errors.New("DOM breakpoints require resolving NodeID first (not yet implemented fully)")
}

// SetEventListenerBreakpoint sets a breakpoint on an event name
func (dc *DebuggerController) SetEventListenerBreakpoint(ctx context.Context, eventName string) error {
	if !dc.debugger.connected {
		return errors.New("not connected to Chrome")
	}

	err := chromedp.Run(dc.debugger.chromeCtx,
		domdebugger.SetEventListenerBreakpoint(eventName),
	)

	if err != nil {
		return fmt.Errorf("failed to set event listener breakpoint: %w", err)
	}

	dc.breakpoints["event:"+eventName] = &Breakpoint{
		ID:        "event:" + eventName,
		Condition: eventName,
		Type:      "event",
		Enabled:   true,
	}

	return nil
}

// RemoveBreakpoint removes a breakpoint
func (dc *DebuggerController) RemoveBreakpoint(ctx context.Context, breakpointID string) error {
	if !dc.debugger.connected {
		return errors.New("not connected to Chrome")
	}

	err := chromedp.Run(dc.debugger.chromeCtx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			return debugger.RemoveBreakpoint(debugger.BreakpointID(breakpointID)).Do(ctx)
		}),
	)

	if err != nil {
		return fmt.Errorf("failed to remove breakpoint: %w", err)
	}

	delete(dc.breakpoints, breakpointID)
	return nil
}

// ListBreakpoints returns all breakpoints
func (dc *DebuggerController) ListBreakpoints() []*Breakpoint {
	var result []*Breakpoint
	for _, bp := range dc.breakpoints {
		result = append(result, bp)
	}
	return result
}

// Pause pauses JavaScript execution
func (dc *DebuggerController) Pause(ctx context.Context) error {
	if !dc.debugger.connected {
		return errors.New("not connected to Chrome")
	}

	return chromedp.Run(dc.debugger.chromeCtx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			return debugger.Pause().Do(ctx)
		}),
	)
}

// Resume resumes JavaScript execution
func (dc *DebuggerController) Resume(ctx context.Context) error {
	if !dc.debugger.connected {
		return errors.New("not connected to Chrome")
	}

	return chromedp.Run(dc.debugger.chromeCtx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			return debugger.Resume().Do(ctx)
		}),
	)
}

// StepOver steps over the current line
func (dc *DebuggerController) StepOver(ctx context.Context) error {
	if !dc.debugger.connected {
		return errors.New("not connected to Chrome")
	}

	return chromedp.Run(dc.debugger.chromeCtx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			return debugger.StepOver().Do(ctx)
		}),
	)
}

// StepInto steps into the function call
func (dc *DebuggerController) StepInto(ctx context.Context) error {
	if !dc.debugger.connected {
		return errors.New("not connected to Chrome")
	}

	return chromedp.Run(dc.debugger.chromeCtx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			return debugger.StepInto().Do(ctx)
		}),
	)
}

// StepOut steps out of the current function
func (dc *DebuggerController) StepOut(ctx context.Context) error {
	if !dc.debugger.connected {
		return errors.New("not connected to Chrome")
	}

	return chromedp.Run(dc.debugger.chromeCtx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			return debugger.StepOut().Do(ctx)
		}),
	)
}

// GetCallStack returns the current call stack
func (dc *DebuggerController) GetCallStack(ctx context.Context) ([]debugger.CallFrame, error) {
	if !dc.debugger.connected {
		return nil, errors.New("not connected to Chrome")
	}

	// The call stack is typically received via the Paused event
	// For now, return the cached call stack
	return dc.callStack, nil
}

// EvaluateOnCallFrame evaluates an expression in a specific call frame
func (dc *DebuggerController) EvaluateOnCallFrame(ctx context.Context, frameID string, expression string) (interface{}, error) {
	if !dc.debugger.connected {
		return nil, errors.New("not connected to Chrome")
	}

	var result *runtime.RemoteObject
	err := chromedp.Run(dc.debugger.chromeCtx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			res, _, err := debugger.EvaluateOnCallFrame(debugger.CallFrameID(frameID), expression).Do(ctx)
			result = res
			return err
		}),
	)

	if err != nil {
		return nil, fmt.Errorf("failed to evaluate expression: %w", err)
	}

	if result.Value != nil {
		var value interface{}
		if err := json.Unmarshal(result.Value, &value); err == nil {
			return value, nil
		}
	}

	return result.Description, nil
}

// SetupEventListeners sets up debugger event listeners
func (dc *DebuggerController) SetupEventListeners() {
	chromedp.ListenTarget(dc.debugger.chromeCtx, func(ev interface{}) {
		switch e := ev.(type) {
		case *debugger.EventPaused:
			dc.handlePaused(e)
		case *debugger.EventResumed:
			dc.handleResumed()
		// Note: EventBreakpointResolved is not available in current CDP
		// We'll handle breakpoint resolution through other means
		case *debugger.EventScriptParsed:
			if dc.verbose {
				fmt.Printf("Script parsed: %s\n", e.URL)
			}
		}
	})
}

// handlePaused handles the paused event
func (dc *DebuggerController) handlePaused(ev *debugger.EventPaused) {
	dc.paused = true
	// Convert to local type
	var callStack []debugger.CallFrame
	for _, frame := range ev.CallFrames {
		if frame != nil {
			callStack = append(callStack, *frame)
		}
	}
	dc.callStack = callStack

	fmt.Printf("\n=== Debugger Paused ===\n")
	fmt.Printf("Reason: %s\n", ev.Reason)

	if len(ev.CallFrames) > 0 {
		frame := ev.CallFrames[0]
		fmt.Printf("Location: %d:%d\n",
			frame.Location.LineNumber,
			frame.Location.ColumnNumber)

		if frame.FunctionName != "" {
			fmt.Printf("Function: %s\n", frame.FunctionName)
		}
	}

	// Show watch expressions
	if len(dc.watches) > 0 {
		fmt.Println("\nWatch Expressions:")
		for _, expr := range dc.watches {
			if len(ev.CallFrames) > 0 {
				value, err := dc.EvaluateOnCallFrame(context.Background(),
					string(ev.CallFrames[0].CallFrameID), expr)
				if err != nil {
					fmt.Printf("  %s: <error: %v>\n", expr, err)
				} else {
					fmt.Printf("  %s: %v\n", expr, value)
				}
			}
		}
	}

	// Show call stack
	if len(ev.CallFrames) > 1 {
		fmt.Println("\nCall Stack:")
		for i, frame := range ev.CallFrames {
			fmt.Printf("  %d. %s (line %d)\n",
				i,
				frame.FunctionName,
				frame.Location.LineNumber)
		}
	}

	fmt.Println("\nCommands: (c)ontinue, (n)ext, (s)tep in, (o)ut, (p)rint <expr>")
}

// handleResumed handles the resumed event
func (dc *DebuggerController) handleResumed() {
	dc.paused = false
	dc.callStack = nil
	fmt.Println("=== Execution Resumed ===")
}

// Note: handleBreakpointResolved is not used as EventBreakpointResolved is not available
// in the current CDP implementation

// AddWatch adds a watch expression
func (dc *DebuggerController) AddWatch(expression string) {
	dc.watches = append(dc.watches, expression)
	fmt.Printf("Watch added: %s\n", expression)
}

// RemoveWatch removes a watch expression
func (dc *DebuggerController) RemoveWatch(expression string) {
	var newWatches []string
	for _, w := range dc.watches {
		if w != expression {
			newWatches = append(newWatches, w)
		}
	}
	dc.watches = newWatches
	fmt.Printf("Watch removed: %s\n", expression)
}

// ListWatches returns all watch expressions
func (dc *DebuggerController) ListWatches() []string {
	return dc.watches
}
