package browser

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/chromedp/cdproto/target"
	"github.com/chromedp/chromedp"
	"github.com/pkg/errors"
)

// RemoteDebuggingInfo represents information about Chrome's remote debugging endpoint
type RemoteDebuggingInfo struct {
	Browser              string `json:"Browser"`
	ProtocolVersion      string `json:"Protocol-Version"`
	UserAgent            string `json:"User-Agent"`
	V8Version            string `json:"V8-Version"`
	WebKitVersion        string `json:"WebKit-Version"`
	WebSocketDebuggerURL string `json:"webSocketDebuggerUrl"`
}

// ChromeTab represents a Chrome tab/page
type ChromeTab struct {
	Description          string `json:"description"`
	DevtoolsFrontendURL  string `json:"devtoolsFrontendUrl"`
	ID                   string `json:"id"`
	Title                string `json:"title"`
	Type                 string `json:"type"`
	URL                  string `json:"url"`
	WebSocketDebuggerURL string `json:"webSocketDebuggerUrl"`
}

// GetRemoteDebuggingInfo retrieves Chrome's remote debugging information
func GetRemoteDebuggingInfo(host string, port int) (*RemoteDebuggingInfo, error) {
	url := fmt.Sprintf("http://%s:%d/json/version", host, port)

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to connect to Chrome at %s:%d", host, port)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, errors.Wrap(err, "reading response body")
	}

	var info RemoteDebuggingInfo
	if err := json.Unmarshal(body, &info); err != nil {
		return nil, errors.Wrap(err, "parsing JSON response")
	}

	return &info, nil
}

// ListTabs returns a list of all open tabs in Chrome
func ListTabs(host string, port int) ([]ChromeTab, error) {
	url := fmt.Sprintf("http://%s:%d/json", host, port)

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to connect to Chrome at %s:%d", host, port)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, errors.Wrap(err, "reading response body")
	}

	var tabs []ChromeTab
	if err := json.Unmarshal(body, &tabs); err != nil {
		return nil, errors.Wrap(err, "parsing JSON response")
	}

	return tabs, nil
}

// ConnectToTab connects to an existing Chrome tab by its ID
func (b *Browser) ConnectToTab(ctx context.Context, host string, port int, tabID string) error {
	// Find the tab
	tabs, err := ListTabs(host, port)
	if err != nil {
		return errors.Wrap(err, "listing tabs")
	}

	var targetTab *ChromeTab
	for _, tab := range tabs {
		if tab.ID == tabID || tab.URL == tabID {
			targetTab = &tab
			break
		}
	}

	if targetTab == nil {
		return fmt.Errorf("tab not found: %s", tabID)
	}

	// Connect directly to the tab's WebSocket URL
	if targetTab.WebSocketDebuggerURL == "" {
		return fmt.Errorf("tab %s has no WebSocket URL (may already be attached)", tabID)
	}

	return b.ConnectToTabWebSocket(ctx, targetTab.WebSocketDebuggerURL)
}

// ConnectToExistingTab connects to an existing tab without creating a new one
func (b *Browser) ConnectToExistingTab(ctx context.Context, browserWSURL string, tabID string) error {
	// Create a remote allocator pointing to the browser
	allocCtx, allocCancel := chromedp.NewRemoteAllocator(ctx, browserWSURL)

	// Create the browser context with the specific target ID
	var browserCtx context.Context
	var browserCancel context.CancelFunc

	opts := []chromedp.ContextOption{
		chromedp.WithTargetID(target.ID(tabID)),
	}

	opts = append(opts, chromedp.WithErrorf(filteredErrorf))
	if b.opts.Verbose {
		opts = append(opts, chromedp.WithLogf(log.Printf))
	}

	browserCtx, browserCancel = chromedp.NewContext(allocCtx, opts...)

	// Store context and cancel functions
	b.ctx = browserCtx
	b.cancelFunc = func() {
		browserCancel()
		allocCancel()
	}

	// Mark that we're attached to an existing tab (don't close on cleanup)
	b.attachedToTab = true

	return nil
}

// ConnectToTabWebSocket connects to a specific tab via its WebSocket URL
// Uses WithTargetID to attach to the existing tab instead of creating a new one
func (b *Browser) ConnectToTabWebSocket(ctx context.Context, tabWSURL string) error {
	// Extract the target ID from the WebSocket URL
	// URL format: ws://localhost:9222/devtools/page/{targetID}
	parts := strings.Split(tabWSURL, "/")
	if len(parts) < 1 {
		return fmt.Errorf("invalid WebSocket URL: %s", tabWSURL)
	}
	tabID := parts[len(parts)-1]

	// Get the browser's WebSocket URL from /json/version
	// Parse host:port from the tab's WebSocket URL
	host := "localhost"
	port := 9222
	if strings.Contains(tabWSURL, "://") {
		urlParts := strings.Split(tabWSURL, "://")
		if len(urlParts) > 1 {
			hostPort := strings.Split(urlParts[1], "/")[0]
			colonIdx := strings.LastIndex(hostPort, ":")
			if colonIdx > 0 {
				host = hostPort[:colonIdx]
				fmt.Sscanf(hostPort[colonIdx+1:], "%d", &port)
			} else {
				host = hostPort
			}
		}
	}

	info, err := GetRemoteDebuggingInfo(host, port)
	if err != nil {
		return errors.Wrap(err, "getting remote debugging info")
	}

	browserWSURL := info.WebSocketDebuggerURL
	if browserWSURL == "" {
		return fmt.Errorf("no browser WebSocket URL available")
	}

	if b.opts.Verbose {
		log.Printf("Browser WebSocket URL: %s", browserWSURL)
		log.Printf("Attaching to target ID: %s", tabID)
	}

	// Create a remote allocator pointing to the browser
	allocCtx, allocCancel := chromedp.NewRemoteAllocator(ctx, browserWSURL)

	if b.opts.Verbose {
		log.Printf("Created remote allocator")
	}

	// Create a context directly with WithTargetID from the allocator
	// Don't initialize a parent context first - that would create a new tab
	var tabCtx context.Context
	var tabCancel context.CancelFunc

	if b.opts.Verbose {
		tabCtx, tabCancel = chromedp.NewContext(allocCtx,
			chromedp.WithTargetID(target.ID(tabID)),
			chromedp.WithLogf(log.Printf),
			chromedp.WithErrorf(filteredErrorf))
	} else {
		tabCtx, tabCancel = chromedp.NewContext(allocCtx,
			chromedp.WithTargetID(target.ID(tabID)),
			chromedp.WithErrorf(filteredErrorf))
	}

	if b.opts.Verbose {
		log.Printf("Created tab context, running init...")
	}

	// Initialize the tab context
	if err := chromedp.Run(tabCtx); err != nil {
		tabCancel()
		allocCancel()
		return errors.Wrap(err, "initializing tab connection")
	}

	if b.opts.Verbose {
		log.Printf("Connection to tab initialized successfully")
	}

	// Store context and cancel functions
	b.ctx = tabCtx
	b.cancelFunc = func() {
		tabCancel()
		allocCancel()
	}

	// Mark that we're attached to an existing tab
	b.attachedToTab = true

	return nil
}

// ConnectToWebSocket connects to a Chrome instance via WebSocket URL (creates new tab)
func (b *Browser) ConnectToWebSocket(ctx context.Context, wsURL string) error {
	// Create a new context with the WebSocket URL
	allocCtx, allocCancel := chromedp.NewRemoteAllocator(ctx, wsURL)

	// Create the browser context
	var browserCtx context.Context
	var browserCancel context.CancelFunc

	if b.opts.Verbose {
		browserCtx, browserCancel = chromedp.NewContext(
			allocCtx,
			chromedp.WithLogf(log.Printf),
			chromedp.WithErrorf(filteredErrorf),
		)
	} else {
		browserCtx, browserCancel = chromedp.NewContext(allocCtx,
			chromedp.WithErrorf(filteredErrorf))
	}

	// Store context and cancel functions
	b.ctx = browserCtx
	b.cancelFunc = func() {
		browserCancel()
		allocCancel()
	}

	return nil
}

// ConnectToRunningChrome connects to an already running Chrome instance
func (b *Browser) ConnectToRunningChrome(ctx context.Context, host string, port int) error {
	// Get debugging info
	info, err := GetRemoteDebuggingInfo(host, port)
	if err != nil {
		return err
	}

	if info.WebSocketDebuggerURL == "" {
		return errors.New("Chrome instance does not have remote debugging enabled")
	}

	// Connect via WebSocket
	return b.ConnectToWebSocket(ctx, info.WebSocketDebuggerURL)
}
