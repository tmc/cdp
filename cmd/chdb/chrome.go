package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"errors"

	"github.com/chromedp/cdproto/debugger"
	"github.com/chromedp/cdproto/dom"
	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/cdproto/page"
	cdpruntime "github.com/chromedp/cdproto/runtime"
	cdptarget "github.com/chromedp/cdproto/target"
	"github.com/chromedp/chromedp"
)

// ChromeTarget represents a Chrome tab or debug target
type ChromeTarget struct {
	ID           string `json:"id"`
	Type         string `json:"type"`
	Title        string `json:"title"`
	URL          string `json:"url"`
	Description  string `json:"description,omitempty"`
	DevtoolsURL  string `json:"devtoolsFrontendUrl,omitempty"`
	WebSocketURL string `json:"webSocketDebuggerUrl,omitempty"`
}

// ChromeDebugger provides comprehensive Chrome debugging capabilities
type ChromeDebugger struct {
	context       context.Context
	cancel        context.CancelFunc
	chromeCtx     context.Context
	chromeCancel  context.CancelFunc
	port          string
	verbose       bool
	connected     bool
	currentTarget *ChromeTarget
}

// NewChromeDebugger creates a new Chrome debugger instance
func NewChromeDebugger(port string, verbose bool) *ChromeDebugger {
	return &ChromeDebugger{
		port:    port,
		verbose: verbose,
	}
}

// ListTargets lists all available Chrome targets
func (cd *ChromeDebugger) ListTargets(ctx context.Context) ([]ChromeTarget, error) {
	url := fmt.Sprintf("http://localhost:%s/json/list", cd.port)

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to Chrome: %w", err)
	}
	defer resp.Body.Close()

	var targets []ChromeTarget
	if err := json.NewDecoder(resp.Body).Decode(&targets); err != nil {
		return nil, fmt.Errorf("failed to parse targets: %w", err)
	}

	return targets, nil
}

// getBrowserWSURL fetches the browser-level WebSocket URL from /json/version.
func (cd *ChromeDebugger) getBrowserWSURL() (string, error) {
	url := fmt.Sprintf("http://localhost:%s/json/version", cd.port)
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return "", fmt.Errorf("failed to get /json/version: %w", err)
	}
	defer resp.Body.Close()

	var info struct {
		WebSocketDebuggerURL string `json:"webSocketDebuggerUrl"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return "", fmt.Errorf("failed to parse /json/version: %w", err)
	}
	if info.WebSocketDebuggerURL == "" {
		return "", errors.New("no webSocketDebuggerUrl in /json/version")
	}
	return info.WebSocketDebuggerURL, nil
}

// Connect connects to a Chrome target
func (cd *ChromeDebugger) Connect(ctx context.Context, targetID string) error {
	targets, err := cd.ListTargets(ctx)
	if err != nil {
		return err
	}

	var target *ChromeTarget
	for _, t := range targets {
		if t.ID == targetID || targetID == "" {
			target = &t
			break
		}
	}

	if target == nil {
		return fmt.Errorf("target %s not found", targetID)
	}

	// Store the parent context
	cd.context = ctx

	// Connect via browser-level WebSocket and attach to the specific target.
	// This works with Electron apps that don't support Target.createTarget.
	browserWSURL, err := cd.getBrowserWSURL()
	if err != nil {
		return err
	}

	if cd.verbose {
		log.Printf("Connecting via browser WS=%s target ID=%s", browserWSURL, target.ID)
	}

	allocCtx, allocCancel := chromedp.NewRemoteAllocator(ctx, browserWSURL)
	cd.cancel = allocCancel

	var opts []chromedp.ContextOption
	if cd.verbose {
		opts = append(opts, chromedp.WithLogf(log.Printf))
	}
	opts = append(opts, chromedp.WithTargetID(cdptarget.ID(target.ID)))

	chromeCtx, chromeCancel := chromedp.NewContext(allocCtx, opts...)
	cd.chromeCtx = chromeCtx
	cd.chromeCancel = chromeCancel

	// Test connection
	if err := chromedp.Run(chromeCtx, chromedp.Evaluate("1+1", nil)); err != nil {
		cd.Close()
		return fmt.Errorf("failed to connect to target: %w", err)
	}

	cd.connected = true
	cd.currentTarget = target

	if cd.verbose {
		log.Printf("Connected to %s: %s", target.Type, target.Title)
	}

	return nil
}

// EnableDomains enables Chrome DevTools domains
func (cd *ChromeDebugger) EnableDomains(ctx context.Context, domains ...string) error {
	if !cd.connected {
		return errors.New("not connected to Chrome")
	}

	// Use the chrome context, not the passed context
	// The passed context might be short-lived while cd.chromeCtx persists
	return chromedp.Run(cd.chromeCtx,
		chromedp.ActionFunc(func(innerCtx context.Context) error {
			for _, domain := range domains {
				switch domain {
				case "Runtime":
					if err := cdpruntime.Enable().Do(innerCtx); err != nil {
						return fmt.Errorf(fmt.Sprintf("failed to enable %s", domain)+": %w", err)
					}
				case "Page":
					if err := page.Enable().Do(innerCtx); err != nil {
						return fmt.Errorf(fmt.Sprintf("failed to enable %s", domain)+": %w", err)
					}
				case "Network":
					if err := network.Enable().Do(innerCtx); err != nil {
						return fmt.Errorf(fmt.Sprintf("failed to enable %s", domain)+": %w", err)
					}
				case "DOM":
					if err := dom.Enable().Do(innerCtx); err != nil {
						return fmt.Errorf(fmt.Sprintf("failed to enable %s", domain)+": %w", err)
					}
				case "Debugger":
					_, err := debugger.Enable().Do(innerCtx)
					if err != nil {
						return fmt.Errorf(fmt.Sprintf("failed to enable %s", domain)+": %w", err)
					}
				}
			}

			if cd.verbose {
				log.Printf("Enabled domains: %v", domains)
			}

			return nil
		}),
	)
}

// Execute executes JavaScript in the Chrome context
func (cd *ChromeDebugger) Execute(ctx context.Context, expression string) (interface{}, error) {
	if !cd.connected {
		return nil, errors.New("not connected to Chrome")
	}

	var result interface{}
	// Use a timeout context for the evaluation
	evalCtx, evalCancel := context.WithTimeout(cd.chromeCtx, 10*time.Second)
	defer evalCancel()

	err := chromedp.Run(evalCtx,
		chromedp.Evaluate(expression, &result),
	)

	return result, err
}

// Navigate navigates to a URL
func (cd *ChromeDebugger) Navigate(ctx context.Context, url string) error {
	if !cd.connected {
		return errors.New("not connected to Chrome")
	}

	// Use a timeout context for navigation
	navCtx, navCancel := context.WithTimeout(cd.chromeCtx, 15*time.Second)
	defer navCancel()

	return chromedp.Run(navCtx,
		chromedp.Navigate(url),
	)
}

// TakeScreenshot takes a screenshot
func (cd *ChromeDebugger) TakeScreenshot(ctx context.Context, filename string) error {
	if !cd.connected {
		return errors.New("not connected to Chrome")
	}

	var buf []byte
	// Use a timeout context for the screenshot capture
	screenshotCtx, screenshotCancel := context.WithTimeout(cd.chromeCtx, 10*time.Second)
	defer screenshotCancel()

	err := chromedp.Run(screenshotCtx,
		chromedp.CaptureScreenshot(&buf),
	)

	if err != nil {
		return err
	}

	return os.WriteFile(filename, buf, 0644)
}

// InspectElement inspects a DOM element
func (cd *ChromeDebugger) InspectElement(ctx context.Context, selector string) error {
	if !cd.connected {
		return errors.New("not connected to Chrome")
	}

	expression := fmt.Sprintf(`
		(function() {
			const el = document.querySelector('%s');
			if (!el) return 'Element not found';
			return {
				tagName: el.tagName,
				id: el.id || '',
				className: el.className || '',
				textContent: (el.textContent || '').substring(0, 100),
				attributes: Array.from(el.attributes || []).map(a => a.name + '=' + a.value),
				style: el.style.cssText || '',
				position: el.getBoundingClientRect()
			};
		})()
	`, selector)

	result, err := cd.Execute(ctx, expression)
	if err != nil {
		return err
	}

	// Pretty print the result
	if resultBytes, err := json.MarshalIndent(result, "", "  "); err == nil {
		fmt.Println(string(resultBytes))
	} else {
		fmt.Printf("%v\n", result)
	}

	return nil
}

// MonitorNetwork starts monitoring network requests
func (cd *ChromeDebugger) MonitorNetwork(ctx context.Context, duration time.Duration) error {
	if !cd.connected {
		return errors.New("not connected to Chrome")
	}

	// Enable network domain
	if err := cd.EnableDomains(ctx, "Network"); err != nil {
		return err
	}

	fmt.Printf("Monitoring network requests")
	if duration > 0 {
		fmt.Printf(" for %s", duration)
	}
	fmt.Println("...")

	requestCount := 0

	// Listen for network events
	chromedp.ListenTarget(cd.chromeCtx, func(ev interface{}) {
		switch ev := ev.(type) {
		case *network.EventRequestWillBeSent:
			requestCount++
			fmt.Printf("[%d] %s %s\n", requestCount, ev.Request.Method, ev.Request.URL)

		case *network.EventResponseReceived:
			fmt.Printf("    → %d %s (%s)\n",
				ev.Response.Status,
				ev.Response.StatusText,
				ev.Response.MimeType)
		}
	})

	// Wait for duration or until interrupted
	if duration > 0 {
		select {
		case <-time.After(duration):
		case <-ctx.Done():
			return ctx.Err()
		}
	} else {
		<-ctx.Done()
	}

	fmt.Printf("\nMonitoring complete. Captured %d requests.\n", requestCount)

	return nil
}

// LaunchDevTools opens DevTools for the current target
func (cd *ChromeDebugger) LaunchDevTools(ctx context.Context) error {
	if cd.currentTarget == nil {
		return errors.New("no target connected")
	}

	// Modern Chrome DevTools access:
	// The chrome-devtools-frontend.appspot.com URLs often don't work anymore
	// The proper way is through chrome://inspect

	fmt.Println("=== Chrome DevTools Access ===")
	fmt.Printf("Target: %s\n", cd.currentTarget.Title)
	fmt.Printf("URL: %s\n", cd.currentTarget.URL)
	fmt.Printf("Target ID: %s\n", cd.currentTarget.ID)
	fmt.Println("\nTo open DevTools, use one of these methods:")
	fmt.Println("\nMethod 1: Chrome Inspector (Recommended)")
	fmt.Println("  1. Open Chrome or Brave")
	fmt.Println("  2. Navigate to: chrome://inspect")
	fmt.Println("  3. Your target should appear under 'Remote Target'")
	fmt.Println("  4. Click 'inspect' next to your target")

	fmt.Println("\nMethod 2: Direct URL")
	fmt.Printf("  Navigate to: http://localhost:%s\n", cd.port)
	fmt.Println("  This shows a list of debuggable pages")

	// Still try to open the URL if we have it, but warn it might not work
	if cd.currentTarget.DevtoolsURL != "" {
		devtoolsURL := cd.currentTarget.DevtoolsURL
		if !strings.HasPrefix(devtoolsURL, "http") {
			devtoolsURL = fmt.Sprintf("http://localhost:%s%s", cd.port, devtoolsURL)
		}
		fmt.Println("\nMethod 3: Legacy DevTools URL (may not work):")
		fmt.Printf("  %s\n", devtoolsURL)

		// Try to open it anyway
		var cmd *exec.Cmd
		switch runtime.GOOS {
		case "windows":
			cmd = exec.Command("cmd", "/c", "start", devtoolsURL)
		case "darwin":
			cmd = exec.Command("open", devtoolsURL)
		default:
			cmd = exec.Command("xdg-open", devtoolsURL)
		}

		return cmd.Start()
	}

	// If no DevTools URL, just open the local debug page
	localURL := fmt.Sprintf("http://localhost:%s", cd.port)
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("cmd", "/c", "start", localURL)
	case "darwin":
		cmd = exec.Command("open", localURL)
	default:
		cmd = exec.Command("xdg-open", localURL)
	}

	return cmd.Start()
}

// CreateDevToolsTarget creates a new DevTools target for DevTools-in-DevTools
func (cd *ChromeDebugger) CreateDevToolsTarget(ctx context.Context) (*ChromeTarget, error) {
	// Create a new target with DevTools URL
	newTabURL := fmt.Sprintf("http://localhost:%s/json/new", cd.port)

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(newTabURL)
	if err != nil {
		return nil, fmt.Errorf("failed to create new tab: %w", err)
	}
	defer resp.Body.Close()

	var newTarget ChromeTarget
	if err := json.NewDecoder(resp.Body).Decode(&newTarget); err != nil {
		return nil, fmt.Errorf("failed to parse new target: %w", err)
	}

	return &newTarget, nil
}

// Close closes the Chrome debugger connection
func (cd *ChromeDebugger) Close() error {
	if cd.chromeCancel != nil {
		cd.chromeCancel()
	}
	if cd.cancel != nil {
		cd.cancel()
	}
	cd.connected = false
	cd.currentTarget = nil

	return nil
}
