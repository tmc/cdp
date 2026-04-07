package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"errors"

	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/debugger"
	"github.com/chromedp/cdproto/dom"
	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/cdproto/runtime"
	"github.com/chromedp/cdproto/target"
	"github.com/chromedp/chromedp"
)

// ChromeTab represents a Chrome tab or target
type ChromeTab struct {
	ID          string `json:"id"`
	Type        string `json:"type"`
	Title       string `json:"title"`
	URL         string `json:"url"`
	Description string `json:"description,omitempty"`
}

// NetworkRequest represents a captured network request
type NetworkRequest struct {
	RequestID string                 `json:"request_id"`
	URL       string                 `json:"url"`
	Method    string                 `json:"method"`
	Headers   map[string]interface{} `json:"headers"`
	PostData  string                 `json:"post_data,omitempty"`
	Response  *NetworkResponse       `json:"response,omitempty"`
	Timestamp time.Time              `json:"timestamp"`
	Duration  float64                `json:"duration,omitempty"`
}

// NetworkResponse represents a network response
type NetworkResponse struct {
	Status     int                    `json:"status"`
	StatusText string                 `json:"status_text"`
	Headers    map[string]interface{} `json:"headers"`
	MimeType   string                 `json:"mime_type"`
	Body       string                 `json:"body,omitempty"`
	BodySize   int64                  `json:"body_size"`
}

// ChromeDebugger handles Chrome/Chromium debugging operations
type ChromeDebugger struct {
	manager          *SessionManager
	session          *Session
	verbose          bool
	networkRequests  map[string]*NetworkRequest
	recordingNetwork bool
}

// NewChromeDebugger creates a new Chrome debugger
func NewChromeDebugger(verbose bool) *ChromeDebugger {
	return &ChromeDebugger{
		manager:         NewSessionManager(verbose),
		verbose:         verbose,
		networkRequests: make(map[string]*NetworkRequest),
	}
}

// Attach attaches to a Chrome instance
func (cd *ChromeDebugger) Attach(ctx context.Context, port string) error {
	// Verify Chrome is available
	url := fmt.Sprintf("http://localhost:%s/json/version", port)
	resp, err := http.Get(url)
	if err != nil {
		return fmt.Errorf(fmt.Sprintf("cannot connect to Chrome on port %s", port)+": %w", err)
	}
	defer resp.Body.Close()

	var versionInfo map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&versionInfo); err != nil {
		return fmt.Errorf("failed to get Chrome version info: %w", err)
	}

	if cd.verbose {
		log.Printf("Chrome version: %v", versionInfo["Browser"])
	}

	// List available tabs
	tabs, err := cd.ListTabs(ctx, port)
	if err != nil {
		return fmt.Errorf("failed to list tabs: %w", err)
	}

	if len(tabs) == 0 {
		// Create a new tab
		newTabURL := fmt.Sprintf("http://localhost:%s/json/new", port)
		resp, err := http.Get(newTabURL)
		if err != nil {
			return fmt.Errorf("failed to create new tab: %w", err)
		}
		resp.Body.Close()

		// Re-list tabs
		tabs, err = cd.ListTabs(ctx, port)
		if err != nil {
			return fmt.Errorf("failed to list tabs after creation: %w", err)
		}
	}

	// Use the first page tab
	var targetTab *ChromeTab
	for _, tab := range tabs {
		if tab.Type == "page" {
			targetTab = &tab
			break
		}
	}

	if targetTab == nil && len(tabs) > 0 {
		targetTab = &tabs[0]
	}

	if targetTab == nil {
		return errors.New("no suitable Chrome tab found")
	}

	debugTarget := DebugTarget{
		ID:          targetTab.ID,
		Type:        SessionTypeChrome,
		Title:       targetTab.Title,
		URL:         targetTab.URL,
		Description: targetTab.Description,
		Port:        port,
		Connected:   true,
	}

	// Create session
	session, err := cd.manager.CreateSession(ctx, debugTarget)
	if err != nil {
		return fmt.Errorf("failed to create session: %w", err)
	}

	cd.session = session

	// Enable necessary domains
	if err := cd.enableDomains(ctx); err != nil {
		return fmt.Errorf("failed to enable Chrome domains: %w", err)
	}

	fmt.Printf("Attached to Chrome tab: %s\n", targetTab.Title)
	fmt.Printf("URL: %s\n", targetTab.URL)

	// Start event monitoring
	cd.startEventMonitoring()

	return nil
}

// enableDomains enables necessary Chrome DevTools domains
func (cd *ChromeDebugger) enableDomains(ctx context.Context) error {
	return chromedp.Run(cd.session.ChromeCtx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			// Enable Page domain
			if err := page.Enable().Do(ctx); err != nil {
				return err
			}

			// Enable Runtime domain
			if err := runtime.Enable().Do(ctx); err != nil {
				return err
			}

			// Enable Network domain
			if err := network.Enable().Do(ctx); err != nil {
				return err
			}

			// Enable DOM domain
			if err := dom.Enable().Do(ctx); err != nil {
				return err
			}

			// Enable Debugger domain for JavaScript debugging
			_, err := debugger.Enable().Do(ctx)
			if err != nil {
				return err
			}

			if cd.verbose {
				log.Println("Chrome DevTools domains enabled")
			}

			return nil
		}),
	)
}

// startEventMonitoring starts monitoring Chrome events
func (cd *ChromeDebugger) startEventMonitoring() {
	chromedp.ListenTarget(cd.session.ChromeCtx, func(ev interface{}) {
		switch ev := ev.(type) {
		case *runtime.EventConsoleAPICalled:
			cd.handleConsoleMessage(ev)

		case *runtime.EventExceptionThrown:
			cd.handleException(ev)

		case *page.EventJavascriptDialogOpening:
			cd.handleDialog(ev)

		case *page.EventFrameNavigated:
			if cd.verbose {
				log.Printf("Frame navigated: %s", ev.Frame.URL)
			}

		case *network.EventRequestWillBeSent:
			cd.handleNetworkRequest(ev)

		case *network.EventResponseReceived:
			cd.handleNetworkResponse(ev)

		case *network.EventLoadingFinished:
			cd.handleNetworkFinished(ev)

		case *debugger.EventPaused:
			cd.handleDebuggerPaused(ev)
		}
	})
}

// handleConsoleMessage handles console output from Chrome
func (cd *ChromeDebugger) handleConsoleMessage(ev *runtime.EventConsoleAPICalled) {
	var args []string
	for _, arg := range ev.Args {
		if arg.Value != nil {
			var val interface{}
			if err := json.Unmarshal(arg.Value, &val); err == nil {
				args = append(args, fmt.Sprintf("%v", val))
			}
		} else if arg.Description != "" {
			args = append(args, arg.Description)
		}
	}

	prefix := ""
	switch ev.Type {
	case runtime.APITypeError:
		prefix = "[CONSOLE ERROR]"
	case runtime.APITypeWarning:
		prefix = "[CONSOLE WARN]"
	case runtime.APITypeInfo:
		prefix = "[CONSOLE INFO]"
	case runtime.APITypeDebug:
		prefix = "[CONSOLE DEBUG]"
	default:
		prefix = "[CONSOLE]"
	}

	fmt.Printf("%s %s\n", prefix, strings.Join(args, " "))
}

// handleException handles JavaScript exceptions
func (cd *ChromeDebugger) handleException(ev *runtime.EventExceptionThrown) {
	details := ev.ExceptionDetails
	fmt.Printf("[EXCEPTION] %s\n", details.Text)

	if details.Exception != nil && details.Exception.Description != "" {
		fmt.Printf("  %s\n", details.Exception.Description)
	}

	if details.StackTrace != nil && len(details.StackTrace.CallFrames) > 0 {
		fmt.Println("  Stack trace:")
		for _, frame := range details.StackTrace.CallFrames {
			fmt.Printf("    at %s (%s:%d:%d)\n",
				frame.FunctionName, frame.URL, frame.LineNumber, frame.ColumnNumber)
		}
	}
}

// handleDialog handles JavaScript dialogs
func (cd *ChromeDebugger) handleDialog(ev *page.EventJavascriptDialogOpening) {
	fmt.Printf("[DIALOG] %s: %s\n", ev.Type, ev.Message)

	// Auto-accept dialogs in non-interactive mode
	chromedp.Run(cd.session.ChromeCtx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			return page.HandleJavaScriptDialog(true).Do(ctx)
		}),
	)
}

// handleDebuggerPaused handles debugger pause events
func (cd *ChromeDebugger) handleDebuggerPaused(ev *debugger.EventPaused) {
	fmt.Printf("[DEBUGGER PAUSED] Reason: %s\n", ev.Reason)

	if len(ev.CallFrames) > 0 {
		frame := ev.CallFrames[0]
		fmt.Printf("  Location: %s:%d:%d\n",
			frame.Location.ScriptID, frame.Location.LineNumber, frame.Location.ColumnNumber)
		if frame.FunctionName != "" {
			fmt.Printf("  Function: %s\n", frame.FunctionName)
		}
	}
}

// handleNetworkRequest handles network request events
func (cd *ChromeDebugger) handleNetworkRequest(ev *network.EventRequestWillBeSent) {
	if !cd.recordingNetwork {
		return
	}

	req := &NetworkRequest{
		RequestID: string(ev.RequestID),
		URL:       ev.Request.URL,
		Method:    ev.Request.Method,
		Headers:   ev.Request.Headers,
		Timestamp: time.Now(),
	}

	cd.networkRequests[req.RequestID] = req

	if cd.verbose {
		log.Printf("[NETWORK] %s %s", req.Method, req.URL)
	}
}

// handleNetworkResponse handles network response events
func (cd *ChromeDebugger) handleNetworkResponse(ev *network.EventResponseReceived) {
	if !cd.recordingNetwork {
		return
	}

	if req, exists := cd.networkRequests[string(ev.RequestID)]; exists {
		req.Response = &NetworkResponse{
			Status:     int(ev.Response.Status),
			StatusText: ev.Response.StatusText,
			Headers:    ev.Response.Headers,
			MimeType:   ev.Response.MimeType,
		}
	}
}

// handleNetworkFinished handles network loading finished events
func (cd *ChromeDebugger) handleNetworkFinished(ev *network.EventLoadingFinished) {
	if !cd.recordingNetwork {
		return
	}

	if req, exists := cd.networkRequests[string(ev.RequestID)]; exists {
		if ev.Timestamp != nil {
			req.Duration = float64(ev.Timestamp.Time().Unix()) - float64(req.Timestamp.Unix())
		}

		if cd.verbose {
			log.Printf("[NETWORK COMPLETE] %s %s - %dms",
				req.Method, req.URL, int(req.Duration*1000))
		}
	}
}

// ListTabs lists Chrome tabs and targets
func (cd *ChromeDebugger) ListTabs(ctx context.Context, port string) ([]ChromeTab, error) {
	url := fmt.Sprintf("http://localhost:%s/json/list", port)

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("failed to list Chrome tabs: %w", err)
	}
	defer resp.Body.Close()

	var rawTabs []map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&rawTabs); err != nil {
		return nil, fmt.Errorf("failed to parse tabs: %w", err)
	}

	var tabs []ChromeTab
	for _, rawTab := range rawTabs {
		tab := ChromeTab{
			ID:          getString(rawTab, "id"),
			Type:        getString(rawTab, "type"),
			Title:       getString(rawTab, "title"),
			URL:         getString(rawTab, "url"),
			Description: getString(rawTab, "description"),
		}

		tabs = append(tabs, tab)
	}

	return tabs, nil
}

// Navigate navigates to a URL
func (cd *ChromeDebugger) Navigate(ctx context.Context, url string, tabID string) error {
	if cd.session == nil {
		return errors.New("not attached to Chrome")
	}

	// If specific tab requested, switch to it
	if tabID != "" {
		err := chromedp.Run(cd.session.ChromeCtx,
			chromedp.ActionFunc(func(ctx context.Context) error {
				return target.ActivateTarget(target.ID(tabID)).Do(ctx)
			}),
		)
		if err != nil {
			return fmt.Errorf("failed to activate target: %w", err)
		}
	}

	// Navigate to URL
	err := chromedp.Run(cd.session.ChromeCtx,
		chromedp.Navigate(url),
	)

	if err != nil {
		return fmt.Errorf("failed to navigate: %w", err)
	}

	fmt.Printf("Navigated to: %s\n", url)

	return nil
}

// EvaluateJS evaluates JavaScript in the Chrome console
func (cd *ChromeDebugger) EvaluateJS(ctx context.Context, expression string, tabID string) (interface{}, error) {
	if cd.session == nil {
		return nil, errors.New("not attached to Chrome")
	}

	// If specific tab requested, switch to it
	if tabID != "" {
		err := chromedp.Run(cd.session.ChromeCtx,
			chromedp.ActionFunc(func(ctx context.Context) error {
				return target.ActivateTarget(target.ID(tabID)).Do(ctx)
			}),
		)
		if err != nil {
			return nil, fmt.Errorf("failed to activate target: %w", err)
		}
	}

	var result interface{}
	err := chromedp.Run(cd.session.ChromeCtx,
		chromedp.Evaluate(expression, &result),
	)

	if err != nil {
		return nil, fmt.Errorf("failed to evaluate JavaScript: %w", err)
	}

	return result, nil
}

// StartNetworkRecording starts recording network activity
func (cd *ChromeDebugger) StartNetworkRecording() {
	cd.recordingNetwork = true
	cd.networkRequests = make(map[string]*NetworkRequest)
	fmt.Println("Network recording started")
}

// StopNetworkRecording stops recording network activity
func (cd *ChromeDebugger) StopNetworkRecording() []NetworkRequest {
	cd.recordingNetwork = false

	var requests []NetworkRequest
	for _, req := range cd.networkRequests {
		requests = append(requests, *req)
	}

	fmt.Printf("Network recording stopped. Captured %d requests\n", len(requests))

	return requests
}

// TakeScreenshot takes a screenshot of the current page
func (cd *ChromeDebugger) TakeScreenshot(ctx context.Context, filename string) error {
	if cd.session == nil {
		return errors.New("not attached to Chrome")
	}

	var buf []byte
	err := chromedp.Run(cd.session.ChromeCtx,
		chromedp.CaptureScreenshot(&buf),
	)

	if err != nil {
		return fmt.Errorf("failed to capture screenshot: %w", err)
	}

	// Save to file
	if err := os.WriteFile(filename, buf, 0644); err != nil {
		return fmt.Errorf("failed to save screenshot: %w", err)
	}

	fmt.Printf("Screenshot saved to: %s\n", filename)

	return nil
}

// GetPageHTML gets the current page HTML
func (cd *ChromeDebugger) GetPageHTML(ctx context.Context) (string, error) {
	if cd.session == nil {
		return "", errors.New("not attached to Chrome")
	}

	var html string
	err := chromedp.Run(cd.session.ChromeCtx,
		chromedp.OuterHTML("html", &html),
	)

	if err != nil {
		return "", fmt.Errorf("failed to get page HTML: %w", err)
	}

	return html, nil
}

// InspectElement inspects a DOM element by selector
func (cd *ChromeDebugger) InspectElement(ctx context.Context, selector string) error {
	if cd.session == nil {
		return errors.New("not attached to Chrome")
	}

	var nodeID cdp.NodeID
	err := chromedp.Run(cd.session.ChromeCtx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			// Get document
			doc, err := dom.GetDocument().Do(ctx)
			if err != nil {
				return err
			}

			// Query selector
			nodeID, err = dom.QuerySelector(doc.NodeID, selector).Do(ctx)
			if err != nil {
				return err
			}

			if nodeID == 0 {
				return fmt.Errorf("element not found: %s", selector)
			}

			// Get node details
			node, err := dom.DescribeNode().WithNodeID(nodeID).Do(ctx)
			if err != nil {
				return err
			}

			fmt.Printf("Element: %s\n", node.NodeName)
			if len(node.Attributes) > 0 {
				fmt.Println("Attributes:")
				for i := 0; i < len(node.Attributes); i += 2 {
					fmt.Printf("  %s: %s\n", node.Attributes[i], node.Attributes[i+1])
				}
			}

			return nil
		}),
	)

	return err
}

// Close closes the Chrome debugger connection
func (cd *ChromeDebugger) Close() error {
	if cd.session != nil {
		return cd.manager.CloseSession(cd.session.ID)
	}
	return nil
}

// Helper function to safely get string from map
func getString(m map[string]interface{}, key string) string {
	if val, ok := m[key]; ok {
		if str, ok := val.(string); ok {
			return str
		}
	}
	return ""
}
