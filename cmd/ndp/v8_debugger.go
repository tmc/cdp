package main

import (
	"fmt"
	"strconv"
	"strings"
)

// V8Debugger provides comprehensive debugging capabilities matching Chrome DevTools
type V8Debugger struct {
	client *V8InspectorClient
}

// NewV8Debugger creates a new V8 debugger instance
func NewV8Debugger(client *V8InspectorClient) *V8Debugger {
	return &V8Debugger{client: client}
}

// EnableDebugger enables the Debugger domain
func (d *V8Debugger) EnableDebugger() error {
	result, err := d.client.SendCommand("Debugger.enable", nil)
	if err != nil {
		return err
	}

	d.client.debuggerEnabled = true

	// Set up common debugger configuration
	d.client.SendCommand("Debugger.setSkipAllPauses", map[string]interface{}{"skip": false})
	d.client.SendCommand("Debugger.setBreakpointsActive", map[string]interface{}{"active": true})
	d.client.SendCommand("Debugger.setPauseOnExceptions", map[string]interface{}{"state": "none"})

	if d.client.verbose {
		fmt.Printf("Debugger enabled. Debugger ID: %v\n", result["debuggerId"])
	}

	return nil
}

// DisableDebugger disables the Debugger domain
func (d *V8Debugger) DisableDebugger() error {
	_, err := d.client.SendCommand("Debugger.disable", nil)
	if err != nil {
		return err
	}

	d.client.debuggerEnabled = false
	return nil
}

// SetBreakpointByLineNumber sets a breakpoint at a specific line
func (d *V8Debugger) SetBreakpointByLineNumber(lineNumber int, url string, condition string) (*V8Breakpoint, error) {
	params := map[string]interface{}{
		"lineNumber": lineNumber,
	}

	if url != "" {
		params["url"] = url
	}

	if condition != "" {
		params["condition"] = condition
	}

	result, err := d.client.SendCommand("Debugger.setBreakpointByUrl", params)
	if err != nil {
		return nil, err
	}

	breakpoint := &V8Breakpoint{
		ID:         result["breakpointId"].(string),
		LineNumber: lineNumber,
		URL:        url,
		Condition:  condition,
	}

	// Check if breakpoint was resolved
	if locations, ok := result["locations"].([]interface{}); ok && len(locations) > 0 {
		breakpoint.Resolved = true
		if loc, ok := locations[0].(map[string]interface{}); ok {
			if scriptId, ok := loc["scriptId"].(string); ok {
				if script, exists := d.client.scripts[scriptId]; exists {
					breakpoint.URL = script.URL
				}
			}
		}
	}

	d.client.breakpoints[breakpoint.ID] = breakpoint

	if d.client.verbose {
		fmt.Printf("Breakpoint set: %s at line %d\n", breakpoint.ID, lineNumber)
		if breakpoint.Resolved {
			fmt.Println("Breakpoint resolved and active")
		} else {
			fmt.Println("Breakpoint pending (will activate when script loads)")
		}
	}

	return breakpoint, nil
}

// SetBreakpointByLocation sets a breakpoint using file:line format
func (d *V8Debugger) SetBreakpointByLocation(location string, condition string) (*V8Breakpoint, error) {
	parts := strings.Split(location, ":")
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid location format, use file:line")
	}

	fileName := parts[0]
	lineNumber, err := strconv.Atoi(parts[1])
	if err != nil {
		return nil, fmt.Errorf("invalid line number: %v", err)
	}

	// Convert line number to 0-based (V8 uses 0-based line numbers)
	lineNumber = lineNumber - 1

	// Try to find exact URL match first
	var targetURL string
	for _, script := range d.client.scripts {
		if strings.Contains(script.URL, fileName) || strings.HasSuffix(script.URL, fileName) {
			targetURL = script.URL
			break
		}
	}

	// If no exact match, use URL regex
	if targetURL == "" {
		params := map[string]interface{}{
			"lineNumber": lineNumber,
			"urlRegex":   fmt.Sprintf(".*%s.*", fileName),
		}

		if condition != "" {
			params["condition"] = condition
		}

		result, err := d.client.SendCommand("Debugger.setBreakpointByUrl", params)
		if err != nil {
			return nil, err
		}

		breakpoint := &V8Breakpoint{
			ID:         result["breakpointId"].(string),
			Location:   location,
			LineNumber: lineNumber + 1, // Convert back to 1-based for display
			URLRegex:   params["urlRegex"].(string),
			Condition:  condition,
		}

		if locations, ok := result["locations"].([]interface{}); ok && len(locations) > 0 {
			breakpoint.Resolved = true
		}

		d.client.breakpoints[breakpoint.ID] = breakpoint
		return breakpoint, nil
	}

	return d.SetBreakpointByLineNumber(lineNumber, targetURL, condition)
}

// RemoveBreakpoint removes a breakpoint by ID
func (d *V8Debugger) RemoveBreakpoint(breakpointID string) error {
	params := map[string]interface{}{
		"breakpointId": breakpointID,
	}

	_, err := d.client.SendCommand("Debugger.removeBreakpoint", params)
	if err != nil {
		return err
	}

	delete(d.client.breakpoints, breakpointID)

	if d.client.verbose {
		fmt.Printf("Breakpoint removed: %s\n", breakpointID)
	}

	return nil
}

// ListBreakpoints returns all active breakpoints
func (d *V8Debugger) ListBreakpoints() []*V8Breakpoint {
	var breakpoints []*V8Breakpoint
	for _, bp := range d.client.breakpoints {
		breakpoints = append(breakpoints, bp)
	}
	return breakpoints
}

// Resume continues execution
func (d *V8Debugger) Resume() error {
	_, err := d.client.SendCommand("Debugger.resume", nil)
	if err != nil {
		return err
	}

	if d.client.verbose {
		fmt.Println("Execution resumed")
	}

	return nil
}

// StepInto steps into the next function call
func (d *V8Debugger) StepInto() error {
	_, err := d.client.SendCommand("Debugger.stepInto", nil)
	if err != nil {
		return err
	}

	if d.client.verbose {
		fmt.Println("Stepping into...")
	}

	return nil
}

// StepOver steps over the next line
func (d *V8Debugger) StepOver() error {
	_, err := d.client.SendCommand("Debugger.stepOver", nil)
	if err != nil {
		return err
	}

	if d.client.verbose {
		fmt.Println("Stepping over...")
	}

	return nil
}

// StepOut steps out of the current function
func (d *V8Debugger) StepOut() error {
	_, err := d.client.SendCommand("Debugger.stepOut", nil)
	if err != nil {
		return err
	}

	if d.client.verbose {
		fmt.Println("Stepping out...")
	}

	return nil
}

// Pause pauses execution
func (d *V8Debugger) Pause() error {
	_, err := d.client.SendCommand("Debugger.pause", nil)
	if err != nil {
		return err
	}

	if d.client.verbose {
		fmt.Println("Execution paused")
	}

	return nil
}

// GetCallStack returns the current call stack
func (d *V8Debugger) GetCallStack() []*V8CallFrame {
	return d.client.callFrames
}

// SetPauseOnExceptions configures pause behavior for exceptions
func (d *V8Debugger) SetPauseOnExceptions(state string) error {
	// state can be "none", "uncaught", or "all"
	params := map[string]interface{}{
		"state": state,
	}

	_, err := d.client.SendCommand("Debugger.setPauseOnExceptions", params)
	if err != nil {
		return err
	}

	if d.client.verbose {
		fmt.Printf("Pause on exceptions set to: %s\n", state)
	}

	return nil
}

// SetBreakpointsActive enables or disables all breakpoints
func (d *V8Debugger) SetBreakpointsActive(active bool) error {
	params := map[string]interface{}{
		"active": active,
	}

	_, err := d.client.SendCommand("Debugger.setBreakpointsActive", params)
	if err != nil {
		return err
	}

	if d.client.verbose {
		fmt.Printf("Breakpoints active: %t\n", active)
	}

	return nil
}

// GetScriptSource retrieves the source code for a script
func (d *V8Debugger) GetScriptSource(scriptID string) (string, error) {
	params := map[string]interface{}{
		"scriptId": scriptID,
	}

	result, err := d.client.SendCommand("Debugger.getScriptSource", params)
	if err != nil {
		return "", err
	}

	if source, ok := result["scriptSource"].(string); ok {
		return source, nil
	}

	return "", fmt.Errorf("no script source found for script ID: %s", scriptID)
}

// SearchInContent searches for text within script content
func (d *V8Debugger) SearchInContent(scriptID, query string, caseSensitive, isRegex bool) ([]map[string]interface{}, error) {
	params := map[string]interface{}{
		"scriptId":      scriptID,
		"query":         query,
		"caseSensitive": caseSensitive,
		"isRegex":       isRegex,
	}

	result, err := d.client.SendCommand("Debugger.searchInContent", params)
	if err != nil {
		return nil, err
	}

	if matches, ok := result["result"].([]interface{}); ok {
		var searchResults []map[string]interface{}
		for _, match := range matches {
			if matchMap, ok := match.(map[string]interface{}); ok {
				searchResults = append(searchResults, matchMap)
			}
		}
		return searchResults, nil
	}

	return nil, nil
}

// SetScriptSource updates the source code of a script (hot reload)
func (d *V8Debugger) SetScriptSource(scriptID, scriptSource string) error {
	params := map[string]interface{}{
		"scriptId":     scriptID,
		"scriptSource": scriptSource,
	}

	result, err := d.client.SendCommand("Debugger.setScriptSource", params)
	if err != nil {
		return err
	}

	if d.client.verbose {
		fmt.Printf("Script source updated for script ID: %s\n", scriptID)
		if callFrames, ok := result["callFrames"]; ok {
			fmt.Printf("Updated call frames: %v\n", callFrames)
		}
	}

	return nil
}

// EvaluateOnCallFrame evaluates an expression in the context of a call frame
func (d *V8Debugger) EvaluateOnCallFrame(callFrameID, expression string) (map[string]interface{}, error) {
	params := map[string]interface{}{
		"callFrameId": callFrameID,
		"expression":  expression,
	}

	result, err := d.client.SendCommand("Debugger.evaluateOnCallFrame", params)
	if err != nil {
		return nil, err
	}

	return result, nil
}

// SetVariableValue sets the value of a variable in a call frame scope
func (d *V8Debugger) SetVariableValue(scopeNumber int, variableName string, newValue map[string]interface{}, callFrameID string) error {
	params := map[string]interface{}{
		"scopeNumber":   scopeNumber,
		"variableName":  variableName,
		"newValue":      newValue,
		"callFrameId":   callFrameID,
	}

	_, err := d.client.SendCommand("Debugger.setVariableValue", params)
	if err != nil {
		return err
	}

	if d.client.verbose {
		fmt.Printf("Variable '%s' updated in scope %d\n", variableName, scopeNumber)
	}

	return nil
}