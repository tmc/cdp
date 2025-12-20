package executor

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"
	"github.com/tmc/misc/chrome-to-har/internal/cdpscript/ast"
	"github.com/tmc/misc/chrome-to-har/internal/recorder"
)

// executeSave handles save commands (save variables to files).
func (e *Executor) executeSave(cmd *ast.SaveCommand) error {
	// Get variable value
	value, ok := e.variables[cmd.Variable]
	if !ok {
		return fmt.Errorf("variable %s not found", cmd.Variable)
	}

	// Determine output path
	outputPath := cmd.Filename
	if e.outputDir != "" && !filepath.IsAbs(outputPath) {
		outputPath = filepath.Join(e.outputDir, outputPath)
	}

	// Ensure directory exists
	if err := os.MkdirAll(filepath.Dir(outputPath), 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	// Convert value to bytes based on type
	var data []byte
	var err error

	switch v := value.(type) {
	case string:
		data = []byte(v)
	case []byte:
		data = v
	default:
		// For complex types, marshal to JSON
		data, err = json.MarshalIndent(value, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to marshal value: %w", err)
		}
	}

	// Write to file
	if err := os.WriteFile(outputPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}

	if e.verbose {
		fmt.Printf("Saved %s to %s (%d bytes)\n", cmd.Variable, outputPath, len(data))
	}

	return nil
}

// executeAssertion handles assertion commands.
func (e *Executor) executeAssertion(cmd *ast.AssertionCommand) error {
	page := e.browser.GetCurrentPage()
	if page == nil {
		result := AssertionResult{
			Name:    fmt.Sprintf("%s assertion", cmd.Type),
			Type:    cmd.Type,
			Passed:  false,
			Message: "no active page",
		}
		e.assertions.AddResult(result)
		return fmt.Errorf("no active page")
	}

	switch cmd.Type {
	case "selector":
		return e.assertSelector(page, cmd)
	case "status":
		return e.assertStatus(cmd)
	case "no-errors":
		return e.assertNoErrors()
	case "url":
		return e.assertURL(page, cmd)
	default:
		result := AssertionResult{
			Name:    fmt.Sprintf("unknown assertion: %s", cmd.Type),
			Type:    cmd.Type,
			Passed:  false,
			Message: fmt.Sprintf("unknown assertion type: %s", cmd.Type),
		}
		e.assertions.AddResult(result)
		return fmt.Errorf("unknown assertion type: %s", cmd.Type)
	}
}

// assertSelector checks selector-based assertions.
func (e *Executor) assertSelector(page interface{}, cmd *ast.AssertionCommand) error {
	// Type assertion to get the Page interface
	type PageWithMethods interface {
		ElementExists(selector string) (bool, error)
		GetText(selector string) (string, error)
	}

	p, ok := page.(PageWithMethods)
	if !ok {
		return fmt.Errorf("page does not support required methods")
	}

	switch cmd.Condition {
	case "exists":
		exists, err := p.ElementExists(cmd.Selector)
		result := AssertionResult{
			Name:    fmt.Sprintf("element %s exists", cmd.Selector),
			Type:    "selector",
			Passed:  err == nil && exists,
			Details: map[string]interface{}{"selector": cmd.Selector, "condition": "exists"},
		}
		if err != nil {
			result.Message = fmt.Sprintf("failed to check element existence: %v", err)
			e.assertions.AddResult(result)
			return fmt.Errorf("failed to check element existence: %w", err)
		}
		if !exists {
			result.Message = fmt.Sprintf("element %s does not exist", cmd.Selector)
			e.assertions.AddResult(result)
			return fmt.Errorf("assertion failed: element %s does not exist", cmd.Selector)
		}
		result.Message = fmt.Sprintf("element %s exists", cmd.Selector)
		e.assertions.AddResult(result)
		if e.verbose {
			fmt.Printf("✓ Assertion passed: element %s exists\n", cmd.Selector)
		}
		return nil

	case "contains":
		text, err := p.GetText(cmd.Selector)
		result := AssertionResult{
			Name:    fmt.Sprintf("element %s contains '%s'", cmd.Selector, cmd.Value),
			Type:    "selector",
			Passed:  err == nil && strings.Contains(text, cmd.Value),
			Details: map[string]interface{}{"selector": cmd.Selector, "condition": "contains", "expected": cmd.Value, "actual": text},
		}
		if err != nil {
			result.Message = fmt.Sprintf("failed to get element text: %v", err)
			e.assertions.AddResult(result)
			return fmt.Errorf("failed to get element text: %w", err)
		}
		if !strings.Contains(text, cmd.Value) {
			result.Message = fmt.Sprintf("element %s text '%s' does not contain '%s'", cmd.Selector, text, cmd.Value)
			e.assertions.AddResult(result)
			return fmt.Errorf("assertion failed: element %s text '%s' does not contain '%s'", cmd.Selector, text, cmd.Value)
		}
		result.Message = fmt.Sprintf("element %s contains '%s'", cmd.Selector, cmd.Value)
		e.assertions.AddResult(result)
		if e.verbose {
			fmt.Printf("✓ Assertion passed: element %s contains '%s'\n", cmd.Selector, cmd.Value)
		}
		return nil

	case "text":
		text, err := p.GetText(cmd.Selector)
		result := AssertionResult{
			Name:    fmt.Sprintf("element %s text equals '%s'", cmd.Selector, cmd.Value),
			Type:    "selector",
			Passed:  err == nil && text == cmd.Value,
			Details: map[string]interface{}{"selector": cmd.Selector, "condition": "text", "expected": cmd.Value, "actual": text},
		}
		if err != nil {
			result.Message = fmt.Sprintf("failed to get element text: %v", err)
			e.assertions.AddResult(result)
			return fmt.Errorf("failed to get element text: %w", err)
		}
		if text != cmd.Value {
			result.Message = fmt.Sprintf("element %s text is '%s', expected '%s'", cmd.Selector, text, cmd.Value)
			e.assertions.AddResult(result)
			return fmt.Errorf("assertion failed: element %s text is '%s', expected '%s'", cmd.Selector, text, cmd.Value)
		}
		result.Message = fmt.Sprintf("element %s text equals '%s'", cmd.Selector, cmd.Value)
		e.assertions.AddResult(result)
		if e.verbose {
			fmt.Printf("✓ Assertion passed: element %s text equals '%s'\n", cmd.Selector, cmd.Value)
		}
		return nil

	default:
		result := AssertionResult{
			Name:    fmt.Sprintf("unknown selector condition: %s", cmd.Condition),
			Type:    "selector",
			Passed:  false,
			Message: fmt.Sprintf("unknown selector assertion condition: %s", cmd.Condition),
		}
		e.assertions.AddResult(result)
		return fmt.Errorf("unknown selector assertion condition: %s", cmd.Condition)
	}
}

// assertStatus checks HTTP status code assertions.
func (e *Executor) assertStatus(cmd *ast.AssertionCommand) error {
	// Get current URL to match against requests
	page := e.browser.GetCurrentPage()
	if page == nil {
		return fmt.Errorf("no active page")
	}

	currentURL, err := page.URL()
	if err != nil {
		return fmt.Errorf("failed to get current URL: %w", err)
	}

	// Find the last request matching this URL
	reqs := e.network.GetRequests()
	var lastReq *NetworkRequest

	for i := len(reqs) - 1; i >= 0; i-- {
		// Simple exact match check first, then contains?
		// e.network stores the full URL.
		if reqs[i].URL == currentURL {
			lastReq = &reqs[i]
			break
		}
	}
	// If not found by exact match, maybe try contains if URL has fragments?
	if lastReq == nil {
		// Fallback: search for URL without fragment
		cleanURL := strings.Split(currentURL, "#")[0]
		for i := len(reqs) - 1; i >= 0; i-- {
			if strings.HasPrefix(reqs[i].URL, cleanURL) {
				lastReq = &reqs[i]
				break
			}
		}
	}

	if lastReq == nil {
		// If we can't find the request, we can't assert status
		// This might happen if network events weren't enabled or captured
		return fmt.Errorf("cannot assert status: no network request found for %s", currentURL)
	}

	result := AssertionResult{
		Name:   fmt.Sprintf("status code is %d", cmd.Status),
		Type:   "status",
		Passed: lastReq.Status == cmd.Status,
		Details: map[string]interface{}{
			"expected": cmd.Status,
			"actual":   lastReq.Status,
			"url":      lastReq.URL,
		},
	}

	if !result.Passed {
		result.Message = fmt.Sprintf("status code is %d, expected %d", lastReq.Status, cmd.Status)
		e.assertions.AddResult(result)
		return fmt.Errorf("assertion failed: %s", result.Message)
	}

	result.Message = fmt.Sprintf("status code is %d", cmd.Status)
	e.assertions.AddResult(result)
	if e.verbose {
		fmt.Printf("✓ Assertion passed: status code is %d\n", cmd.Status)
	}
	return nil
}

// assertNoErrors checks for console errors.
func (e *Executor) assertNoErrors() error {
	errors := e.console.GetErrors()
	result := AssertionResult{
		Name:    "no console errors",
		Type:    "no-errors",
		Passed:  len(errors) == 0,
		Details: map[string]interface{}{"error_count": len(errors)},
	}

	if len(errors) > 0 {
		errorMsgs := make([]string, len(errors))
		for i, err := range errors {
			errorMsgs[i] = err.Message
		}
		result.Message = fmt.Sprintf("found %d console errors: %s", len(errors), strings.Join(errorMsgs, "; "))
		result.Details["errors"] = errorMsgs
		e.assertions.AddResult(result)
		return fmt.Errorf("assertion failed: %s", result.Message)
	}

	result.Message = "no console errors found"
	e.assertions.AddResult(result)
	if e.verbose {
		fmt.Println("✓ Assertion passed: no console errors")
	}
	return nil
}

// assertURL checks URL-based assertions.
func (e *Executor) assertURL(page interface{}, cmd *ast.AssertionCommand) error {
	type PageWithURL interface {
		URL() (string, error)
	}

	p, ok := page.(PageWithURL)
	if !ok {
		return fmt.Errorf("page does not support URL method")
	}

	url, err := p.URL()
	if err != nil {
		return fmt.Errorf("failed to get current URL: %w", err)
	}

	switch cmd.Condition {
	case "contains":
		if !strings.Contains(url, cmd.Value) {
			return fmt.Errorf("assertion failed: URL '%s' does not contain '%s'", url, cmd.Value)
		}
		if e.verbose {
			fmt.Printf("✓ Assertion passed: URL contains '%s'\n", cmd.Value)
		}
		return nil

	default:
		return fmt.Errorf("unknown URL assertion condition: %s", cmd.Condition)
	}
}

// executeNetwork handles network commands.
func (e *Executor) executeNetwork(cmd *ast.NetworkCommand) error {
	switch cmd.Type {
	case "capture":
		return e.networkCapture(cmd)
	case "mock":
		return e.networkMock(cmd)
	case "block":
		return e.networkBlock(cmd)
	case "throttle":
		return e.networkThrottle(cmd)
	default:
		return fmt.Errorf("unknown network command type: %s", cmd.Type)
	}
}

// networkCapture starts network capture to HAR.
func (e *Executor) networkCapture(cmd *ast.NetworkCommand) error {
	if e.browser == nil {
		return fmt.Errorf("browser not initialized")
	}

	// Initialize recorder if not already done
	if e.rec == nil {
		opts := []recorder.Option{}
		if e.verbose {
			opts = append(opts, recorder.WithVerbose(true))
		}

		rec, err := recorder.New(opts...)
		if err != nil {
			return fmt.Errorf("failed to create recorder: %w", err)
		}
		e.rec = rec

		// Enable network events
		if err := chromedp.Run(e.browser.Context(), network.Enable()); err != nil {
			return fmt.Errorf("failed to enable network events: %w", err)
		}

		// Attach listener
		chromedp.ListenTarget(e.browser.Context(), rec.HandleNetworkEvent(e.browser.Context()))
	}

	if e.verbose {
		fmt.Printf("Started network capture (target: %s)\n", cmd.Target)
	}
	return nil
}

// networkMock sets up API mocking.
func (e *Executor) networkMock(cmd *ast.NetworkCommand) error {
	// TODO: Integrate with network interception
	if e.verbose {
		fmt.Printf("Would mock API %s with %s\n", cmd.Resource, cmd.Target)
	}
	return nil
}

// networkBlock blocks network requests.
func (e *Executor) networkBlock(cmd *ast.NetworkCommand) error {
	// TODO: Integrate with blocking engine
	if e.verbose {
		fmt.Printf("Would block requests matching %s\n", cmd.Pattern)
	}
	return nil
}

// networkThrottle applies network throttling.
func (e *Executor) networkThrottle(cmd *ast.NetworkCommand) error {
	// TODO: Integrate with network throttling
	if e.verbose {
		fmt.Printf("Would apply network throttling: %s\n", cmd.Pattern)
	}
	return nil
}

// executeControlFlow handles control flow commands.
func (e *Executor) executeControlFlow(cmd *ast.ControlFlowCommand) error {
	switch cmd.Type {
	case "if":
		return e.controlFlowIf(cmd)
	case "for":
		return e.controlFlowFor(cmd)
	case "include":
		return e.controlFlowInclude(cmd)
	default:
		return fmt.Errorf("unknown control flow type: %s", cmd.Type)
	}
}

// controlFlowIf executes conditional blocks.
func (e *Executor) controlFlowIf(cmd *ast.ControlFlowCommand) error {
	// TODO: Implement condition evaluation
	if e.verbose {
		fmt.Printf("Would evaluate condition: %s\n", cmd.Condition)
	}
	// For now, always execute body
	for _, bodyCmd := range cmd.Body {
		if err := e.executeCommand(bodyCmd); err != nil {
			return err
		}
	}
	return nil
}

// controlFlowFor executes loops.
func (e *Executor) controlFlowFor(cmd *ast.ControlFlowCommand) error {
	// TODO: Implement loop execution
	if e.verbose {
		fmt.Printf("Would loop over %s in %v\n", cmd.Variable, cmd.List)
	}
	return nil
}

// controlFlowInclude includes another script.
func (e *Executor) controlFlowInclude(cmd *ast.ControlFlowCommand) error {
	// TODO: Load and execute included script
	if e.verbose {
		fmt.Printf("Would include script: %s\n", cmd.Filename)
	}
	return nil
}

// executeCompare handles visual comparison commands.
func (e *Executor) executeCompare(cmd *ast.CompareCommand) error {
	// TODO: Implement visual comparison using image diff
	if e.verbose {
		fmt.Printf("Would compare %s with %s\n", cmd.Current, cmd.Baseline)
	}
	return nil
}

// saveOutputFile is a helper to save output data to a file.
func (e *Executor) saveOutputFile(filename string, data []byte) error {
	// Determine output path
	outputPath := filename
	if e.outputDir != "" && !filepath.IsAbs(outputPath) {
		outputPath = filepath.Join(e.outputDir, outputPath)
	}

	// Ensure directory exists
	if err := os.MkdirAll(filepath.Dir(outputPath), 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	// Write to file
	if err := os.WriteFile(outputPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}

	if e.verbose {
		fmt.Printf("Saved output to %s (%d bytes)\n", outputPath, len(data))
	}

	return nil
}
