package main

import (
	"context"
	"fmt"
	"log"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"errors"

	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/debugger"
	"github.com/chromedp/cdproto/runtime"
	"github.com/chromedp/chromedp"
)

// BreakpointType represents the type of breakpoint
type BreakpointType string

const (
	BreakpointTypeLine     BreakpointType = "line"
	BreakpointTypeFunction BreakpointType = "function"
	BreakpointTypeEvent    BreakpointType = "event"
	BreakpointTypeLog      BreakpointType = "log"
)

// Breakpoint represents a breakpoint with its properties
type Breakpoint struct {
	ID           string         `json:"id"`
	Type         BreakpointType `json:"type"`
	Location     string         `json:"location"`    // file:line or function name
	Condition    string         `json:"condition"`   // Conditional expression
	HitCount     int            `json:"hit_count"`   // Number of times hit
	LogMessage   string         `json:"log_message"` // For log points
	Enabled      bool           `json:"enabled"`
	ScriptID     cdp.ScriptID   `json:"script_id,omitempty"`
	LineNumber   int            `json:"line_number"`
	ColumnNumber int            `json:"column_number"`
	Resolved     bool           `json:"resolved"`
}

// BreakpointManager manages breakpoints across debugging sessions
type BreakpointManager struct {
	breakpoints map[string]*Breakpoint
	session     *Session
	verbose     bool
	mu          sync.RWMutex
}

// NewBreakpointManager creates a new breakpoint manager
func NewBreakpointManager(verbose bool) *BreakpointManager {
	return &BreakpointManager{
		breakpoints: make(map[string]*Breakpoint),
		verbose:     verbose,
	}
}

// SetSession associates the manager with a debug session
func (bm *BreakpointManager) SetSession(session *Session) {
	bm.session = session
}

// SetBreakpoint sets a breakpoint at the specified location
func (bm *BreakpointManager) SetBreakpoint(ctx context.Context, location string, condition string) error {
	if bm.session == nil {
		return errors.New("no active debug session")
	}

	// Parse location (file:line or function name)
	bp, err := bm.parseLocation(location)
	if err != nil {
		return fmt.Errorf("invalid breakpoint location: %w", err)
	}

	bp.Condition = condition
	bp.Enabled = true

	// Set the breakpoint based on type
	switch bp.Type {
	case BreakpointTypeLine:
		err = bm.setLineBreakpoint(ctx, bp)
	case BreakpointTypeFunction:
		err = bm.setFunctionBreakpoint(ctx, bp)
	default:
		err = fmt.Errorf("unsupported breakpoint type: %s", bp.Type)
	}

	if err != nil {
		return err
	}

	// Store breakpoint
	bm.mu.Lock()
	bm.breakpoints[bp.ID] = bp
	bm.mu.Unlock()

	if bm.verbose {
		log.Printf("Breakpoint set: %s at %s", bp.ID, bp.Location)
		if condition != "" {
			log.Printf("  Condition: %s", condition)
		}
	}

	fmt.Printf("Breakpoint %s set at %s\n", bp.ID, location)

	return nil
}

// parseLocation parses a breakpoint location string
func (bm *BreakpointManager) parseLocation(location string) (*Breakpoint, error) {
	bp := &Breakpoint{
		Location: location,
	}

	// Check if it's a file:line format
	if strings.Contains(location, ":") {
		parts := strings.SplitN(location, ":", 2)
		if len(parts) == 2 {
			lineNum, err := strconv.Atoi(parts[1])
			if err == nil {
				bp.Type = BreakpointTypeLine
				bp.LineNumber = lineNum
				bp.ID = fmt.Sprintf("bp_%s_%d", filepath.Base(parts[0]), lineNum)
				return bp, nil
			}
		}
	}

	// Otherwise, treat it as a function name
	bp.Type = BreakpointTypeFunction
	bp.ID = fmt.Sprintf("bp_func_%s", location)

	return bp, nil
}

// setLineBreakpoint sets a breakpoint at a specific line
func (bm *BreakpointManager) setLineBreakpoint(ctx context.Context, bp *Breakpoint) error {
	return chromedp.Run(bm.session.ChromeCtx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			// First, get all parsed scripts to find the right one
			scripts, err := bm.getScripts(ctx)
			if err != nil {
				return err
			}

			// Find matching script
			var targetScript *debugger.EventScriptParsed
			locationParts := strings.Split(bp.Location, ":")
			fileName := locationParts[0]

			for _, script := range scripts {
				if strings.Contains(script.URL, fileName) ||
					filepath.Base(script.URL) == fileName {
					targetScript = script
					break
				}
			}

			if targetScript == nil {
				// Script not loaded yet, set pending breakpoint
				return bm.setPendingBreakpoint(ctx, bp)
			}

			// Set breakpoint by URL
			params := debugger.SetBreakpointByURL(int64(bp.LineNumber)).
				WithURL(targetScript.URL)

			if bp.Condition != "" {
				params = params.WithCondition(bp.Condition)
			}

			if bp.ColumnNumber > 0 {
				params = params.WithColumnNumber(int64(bp.ColumnNumber))
			}

			bpID, locations, err := params.Do(ctx)
			if err != nil {
				return err
			}

			bp.ID = string(bpID)
			bp.ScriptID = targetScript.ScriptID
			bp.Resolved = true

			if len(locations) > 0 {
				loc := locations[0]
				bp.LineNumber = int(loc.LineNumber)
				bp.ColumnNumber = int(loc.ColumnNumber)
			}

			return nil
		}),
	)
}

// setFunctionBreakpoint sets a breakpoint on a function
func (bm *BreakpointManager) setFunctionBreakpoint(ctx context.Context, bp *Breakpoint) error {
	return chromedp.Run(bm.session.ChromeCtx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			// Use Runtime.evaluate to find the function
			expression := fmt.Sprintf(`
				(function() {
					if (typeof %s === 'function') {
						return true;
					}
					return false;
				})()
			`, bp.Location)

			result, exception, err := runtime.Evaluate(expression).Do(ctx)
			if err != nil {
				return err
			}

			if exception != nil {
				return fmt.Errorf("function %s not found", bp.Location)
			}

			if result.Value != nil {
				// Function exists, set breakpoint
				// This would use debugger.setBreakpointOnFunctionCall in newer CDP versions
				// For now, we'll use a workaround
				return bm.setFunctionBreakpointWorkaround(ctx, bp)
			}

			return fmt.Errorf("function %s not found", bp.Location)
		}),
	)
}

// setFunctionBreakpointWorkaround sets a function breakpoint using evaluation
func (bm *BreakpointManager) setFunctionBreakpointWorkaround(ctx context.Context, bp *Breakpoint) error {
	// Inject a wrapper around the function that triggers debugger
	expression := fmt.Sprintf(`
		(function() {
			const original = %s;
			if (typeof original !== 'function') {
				throw new Error('Not a function: %s');
			}

			%s = function(...args) {
				debugger; // Breakpoint here
				return original.apply(this, args);
			};

			return 'Function breakpoint set';
		})()
	`, bp.Location, bp.Location, bp.Location)

	_, exception, err := runtime.Evaluate(expression).Do(ctx)
	if err != nil {
		return err
	}

	if exception != nil {
		return fmt.Errorf("failed to set function breakpoint: %v", exception)
	}

	bp.Resolved = true
	return nil
}

// setPendingBreakpoint sets a breakpoint that will be resolved when the script loads
func (bm *BreakpointManager) setPendingBreakpoint(ctx context.Context, bp *Breakpoint) error {
	// For now, we'll use setBreakpointByURL with a pattern
	locationParts := strings.Split(bp.Location, ":")
	fileName := locationParts[0]

	params := debugger.SetBreakpointByURL(int64(bp.LineNumber)).
		WithURLRegex(fmt.Sprintf(".*%s.*", fileName))

	if bp.Condition != "" {
		params = params.WithCondition(bp.Condition)
	}

	bpID, locations, err := params.Do(ctx)
	if err != nil {
		return err
	}

	bp.ID = string(bpID)
	bp.Resolved = len(locations) > 0 // Will be resolved when script loads

	return nil
}

// RemoveBreakpoint removes a breakpoint
func (bm *BreakpointManager) RemoveBreakpoint(ctx context.Context, breakpointID string) error {
	if bm.session == nil {
		return errors.New("no active debug session")
	}

	bm.mu.RLock()
	bp, exists := bm.breakpoints[breakpointID]
	bm.mu.RUnlock()

	if !exists {
		return fmt.Errorf("breakpoint %s not found", breakpointID)
	}

	// Remove from debugger
	err := chromedp.Run(bm.session.ChromeCtx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			return debugger.RemoveBreakpoint(debugger.BreakpointID(bp.ID)).Do(ctx)
		}),
	)

	if err != nil {
		return err
	}

	// Remove from manager
	bm.mu.Lock()
	delete(bm.breakpoints, breakpointID)
	bm.mu.Unlock()

	fmt.Printf("Breakpoint %s removed\n", breakpointID)

	return nil
}

// ListBreakpoints lists all breakpoints
func (bm *BreakpointManager) ListBreakpoints() []Breakpoint {
	bm.mu.RLock()
	defer bm.mu.RUnlock()

	var breakpoints []Breakpoint
	for _, bp := range bm.breakpoints {
		breakpoints = append(breakpoints, *bp)
	}

	return breakpoints
}

// getScripts retrieves all parsed scripts
func (bm *BreakpointManager) getScripts(ctx context.Context) ([]*debugger.EventScriptParsed, error) {
	// In a real implementation, we would listen to scriptParsed events
	// and maintain a list of scripts. For now, we'll use a workaround.

	// This is a simplified version - in production, you'd maintain
	// a list of scripts from debugger.EventScriptParsed events
	return []*debugger.EventScriptParsed{}, nil
}
