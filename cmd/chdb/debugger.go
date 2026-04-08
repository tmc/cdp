package main

import (
	"context"
	"fmt"
	"strings"

	"errors"

	"github.com/chromedp/cdproto/debugger"
	"github.com/chromedp/cdproto/domdebugger"
	"github.com/chromedp/chromedp"
)

// DebuggerController handles all debugging operations
type DebuggerController struct {
	debugger *ChromeDebugger
	verbose  bool

	// Breakpoint tracking
	breakpoints map[string]*Breakpoint

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
