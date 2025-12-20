// Package main implements a computer use agent that controls Chrome via natural language.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/chromedp/cdproto/input"
	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"
	"github.com/tmc/misc/chrome-to-har/internal/browser"
	"github.com/tmc/misc/chrome-to-har/internal/chromeprofiles"
	"github.com/tmc/misc/chrome-to-har/internal/recorder"
)

// BrowserComputer implements computer control using the chrome-to-har browser package.
type BrowserComputer struct {
	browser          *browser.Browser
	page             *browser.Page
	width            int
	height           int
	harRecorder      *recorder.Recorder
	harOutput        string
	useOSScreenshots bool
	verbose          bool
	windowID         string // Chrome/Brave window ID for screen-capture
	workDir          string // Work directory for screenshots and logs
	screenshotCount  int    // Counter for screenshot filenames
}

// NewBrowserComputer creates a new browser computer instance.
func NewBrowserComputer(ctx context.Context, profileMgr chromeprofiles.ProfileManager, harOutputPath string, useOSScreenshots, verbose bool, workDir string, opts ...browser.Option) (*BrowserComputer, error) {
	// Create browser with options
	b, err := browser.New(ctx, profileMgr, opts...)
	if err != nil {
		return nil, fmt.Errorf("creating browser: %w", err)
	}

	// Launch browser
	if err := b.Launch(ctx); err != nil {
		return nil, fmt.Errorf("launching browser: %w", err)
	}

	// Get the current page
	p := b.GetCurrentPage()
	if p == nil {
		return nil, fmt.Errorf("no page available")
	}

	// Create HAR recorder if output path is specified
	var harRec *recorder.Recorder
	if harOutputPath != "" {
		harRec, err = recorder.New(recorder.WithVerbose(false))
		if err != nil {
			return nil, fmt.Errorf("creating HAR recorder: %w", err)
		}

		// Enable network events and attach HAR recorder
		// CRITICAL: Must use browser context (not page context) and enable BEFORE navigation
		// This ensures cookies are properly loaded from the profile
		if err := chromedp.Run(b.Context(),
			network.Enable(),
			chromedp.ActionFunc(func(ctx context.Context) error {
				// Attach HAR recorder to capture network events
				chromedp.ListenTarget(ctx, harRec.HandleNetworkEvent(ctx))
				if verbose {
					fmt.Fprintf(os.Stderr, "HAR recorder attached to network events\n")
				}
				return nil
			}),
		); err != nil {
			return nil, fmt.Errorf("enabling network and attaching HAR recorder: %w", err)
		}
	} else {
		// Even without HAR recording, enable network events for proper cookie handling
		// CRITICAL: Must use browser context and enable BEFORE navigation
		if err := chromedp.Run(b.Context(), network.Enable()); err != nil {
			return nil, fmt.Errorf("enabling network events: %w", err)
		}
		if verbose {
			fmt.Fprintf(os.Stderr, "Network events enabled for cookie support\n")
		}
	}

	bc := &BrowserComputer{
		browser:          b,
		page:             p,
		width:            1280,
		height:           800,
		harRecorder:      harRec,
		harOutput:        harOutputPath,
		useOSScreenshots: useOSScreenshots,
		verbose:          verbose,
		workDir:          workDir,
		screenshotCount:  0,
	}

	// If using OS screenshots, try to detect the window ID
	// Window detection will be retried lazily during screenshot capture if it fails here
	if useOSScreenshots {
		if verbose {
			fmt.Fprintf(os.Stderr, "OS screenshots enabled, will detect window ID during screenshot capture\n")
		}
	}

	return bc, nil
}

// ScreenSize returns the viewport dimensions.
func (c *BrowserComputer) ScreenSize() (width, height int) {
	return c.width, c.height
}

// OpenWebBrowser navigates to the initial URL.
func (c *BrowserComputer) OpenWebBrowser(url string) (*EnvState, error) {
	if err := c.browser.Navigate(url); err != nil {
		return nil, fmt.Errorf("navigating to %s: %w", url, err)
	}

	return c.CurrentState()
}

// ClickAt clicks at normalized coordinates (0-1000 scale).
func (c *BrowserComputer) ClickAt(x, y int) (*EnvState, error) {
	c.addNote(fmt.Sprintf("🖱️  Click at coordinates (%d, %d)", x, y))

	// When using OS screenshots, we need OS-level clicks to reach DevTools UI
	if c.useOSScreenshots {
		return c.clickAtOS(x, y)
	}

	// Otherwise use CDP for viewport-relative clicks
	pixelX := float64(x*c.width) / 1000.0
	pixelY := float64(y*c.height) / 1000.0

	if c.verbose {
		fmt.Fprintf(os.Stderr, "🖱️  Click: normalized=(%d, %d) pixel=(%.1f, %.1f) viewport=(%dx%d)\n",
			x, y, pixelX, pixelY, c.width, c.height)
	}

	ctx := c.page.Context()

	// Debug: Highlight element at these coordinates with a border
	if c.verbose {
		var elementInfo string
		chromedp.Run(ctx, chromedp.Evaluate(fmt.Sprintf(`
			(() => {
				const elem = document.elementFromPoint(%f, %f);
				if (elem) {
					// Add a red border to highlight the element
					elem.style.border = '3px solid red';
					elem.style.outline = '3px solid yellow';

					const info = elem.tagName + (elem.id ? '#'+elem.id : '') +
					       (elem.className ? '.'+elem.className.split(' ').join('.') : '') +
					       ' text="' + (elem.innerText || elem.textContent || '').substring(0, 50) + '"';

					// Remove border after 2 seconds
					setTimeout(() => {
						elem.style.border = '';
						elem.style.outline = '';
					}, 2000);

					return info;
				}
				return 'null';
			})()
		`, pixelX, pixelY), &elementInfo))
		fmt.Fprintf(os.Stderr, "🎯 Element at click point: %s\n", elementInfo)

		// Take a screenshot with the highlighted element
		c.addScreenshot(fmt.Sprintf("Highlighting element at (%d, %d)", x, y))
		time.Sleep(300 * time.Millisecond) // Brief pause to see the highlight
	}

	if err := chromedp.Run(ctx,
		chromedp.MouseClickXY(pixelX, pixelY),
	); err != nil {
		return nil, fmt.Errorf("clicking at (%d, %d): %w", x, y, err)
	}

	time.Sleep(500 * time.Millisecond)
	c.addScreenshot(fmt.Sprintf("After click at (%d, %d)", x, y))
	return c.CurrentState()
}

// getBrowserProcessName returns the process name for AppleScript
func (c *BrowserComputer) getBrowserProcessName() string {
	// Try to detect from window ID detection logic
	// For now, try Brave first, then Chrome
	apps := []string{"Brave Browser", "Google Chrome"}
	for _, app := range apps {
		cmd := exec.Command("pgrep", "-x", app)
		if err := cmd.Run(); err == nil {
			return app
		}
	}
	return "Brave Browser" // Default fallback
}

// clickAtOS performs an OS-level click at normalized coordinates (0-1000 scale)
// This is needed when using OS screenshots to click on DevTools UI elements
func (c *BrowserComputer) clickAtOS(x, y int) (*EnvState, error) {
	// Get window bounds from list-app-windows to convert normalized coords to screen coords
	// This doesn't require Accessibility permissions like AppleScript does

	browserProcess := c.getBrowserProcessName()

	// Get window bounds using list-app-windows
	cmd := exec.Command("list-app-windows", "-json", "-app", browserProcess)
	output, err := cmd.Output()
	if err != nil {
		if c.verbose {
			fmt.Fprintf(os.Stderr, "Warning: Could not get window bounds: %v\n", err)
		}
		// Fall back to CDP click
		pixelX := float64(x*c.width) / 1000.0
		pixelY := float64(y*c.height) / 1000.0
		ctx := c.page.Context()
		if err := chromedp.Run(ctx, chromedp.MouseClickXY(pixelX, pixelY)); err != nil {
			return nil, fmt.Errorf("clicking at (%d, %d): %w", x, y, err)
		}
		time.Sleep(500 * time.Millisecond)
		c.addScreenshot(fmt.Sprintf("After click at (%d, %d)", x, y))
		return c.CurrentState()
	}

	// Parse JSON to get window bounds
	type WindowInfo struct {
		WindowID  int32  `json:"window_id"`
		OwnerName string `json:"owner_name"`
		X         int    `json:"x"`
		Y         int    `json:"y"`
		Width     int    `json:"width"`
		Height    int    `json:"height"`
	}

	var windows []WindowInfo
	if err := json.Unmarshal(output, &windows); err != nil || len(windows) == 0 {
		if c.verbose {
			fmt.Fprintf(os.Stderr, "Warning: Could not parse window bounds: %v\n", err)
		}
		// Fall back to CDP click
		pixelX := float64(x*c.width) / 1000.0
		pixelY := float64(y*c.height) / 1000.0
		ctx := c.page.Context()
		if err := chromedp.Run(ctx, chromedp.MouseClickXY(pixelX, pixelY)); err != nil {
			return nil, fmt.Errorf("clicking at (%d, %d): %w", x, y, err)
		}
		time.Sleep(500 * time.Millisecond)
		c.addScreenshot(fmt.Sprintf("After click at (%d, %d)", x, y))
		return c.CurrentState()
	}

	winX := windows[0].X
	winY := windows[0].Y
	winWidth := windows[0].Width
	winHeight := windows[0].Height

	// Convert normalized coordinates to screen coordinates
	screenX := winX + (x * winWidth / 1000)
	screenY := winY + (y * winHeight / 1000)

	if c.verbose {
		fmt.Fprintf(os.Stderr, "🖱️  OS Click: normalized=(%d, %d) window=(%d,%d %dx%d) screen=(%d, %d)\n",
			x, y, winX, winY, winWidth, winHeight, screenX, screenY)
	}

	// First, smoothly move the mouse to make the movement visible
	moveCmd := exec.Command("mouse-move", "-smooth", fmt.Sprintf("%d", screenX), fmt.Sprintf("%d", screenY))
	if output, err := moveCmd.CombinedOutput(); err != nil {
		if c.verbose {
			fmt.Fprintf(os.Stderr, "Move command output: %s\n", output)
			fmt.Fprintf(os.Stderr, "Move command error: %v\n", err)
		}
		// Continue to click even if move fails - it's not critical
	}

	// Then perform the click using mouse-click tool (CGEvent APIs)
	clickCmd := exec.Command("mouse-click", "-visual", fmt.Sprintf("%d", screenX), fmt.Sprintf("%d", screenY))
	if output, err := clickCmd.CombinedOutput(); err != nil {
		if c.verbose {
			fmt.Fprintf(os.Stderr, "Click command output: %s\n", output)
			fmt.Fprintf(os.Stderr, "Click command error: %v\n", err)
		}
		// Fall back to CDP click if OS click fails
		if c.verbose {
			fmt.Fprintf(os.Stderr, "Falling back to CDP click\n")
		}
		pixelX := float64(x*c.width) / 1000.0
		pixelY := float64(y*c.height) / 1000.0
		ctx := c.page.Context()
		if err := chromedp.Run(ctx, chromedp.MouseClickXY(pixelX, pixelY)); err != nil {
			return nil, fmt.Errorf("clicking at (%d, %d): %w", x, y, err)
		}
	}

	time.Sleep(500 * time.Millisecond)
	c.addScreenshot(fmt.Sprintf("After OS click at (%d, %d)", x, y))
	return c.CurrentState()
}

// HoverAt hovers at normalized coordinates (0-1000 scale).
func (c *BrowserComputer) HoverAt(x, y int) (*EnvState, error) {
	// Denormalize coordinates
	pixelX := float64(x*c.width) / 1000.0
	pixelY := float64(y*c.height) / 1000.0

	if c.verbose {
		fmt.Fprintf(os.Stderr, "👆 Hover: normalized=(%d, %d) pixel=(%.1f, %.1f) viewport=(%dx%d)\n",
			x, y, pixelX, pixelY, c.width, c.height)
	}

	ctx := c.page.Context()
	if err := chromedp.Run(ctx,
		chromedp.MouseEvent(input.MouseMoved, pixelX, pixelY),
	); err != nil {
		return nil, fmt.Errorf("hovering at (%d, %d): %w", x, y, err)
	}

	time.Sleep(200 * time.Millisecond)
	return c.CurrentState()
}

// TypeTextAt types text at normalized coordinates (0-1000 scale).
func (c *BrowserComputer) TypeTextAt(x, y int, text string, pressEnter, clearBefore bool) (*EnvState, error) {
	if c.verbose {
		fmt.Fprintf(os.Stderr, "⌨️  Type: text='%s' at=(%d,%d) pressEnter=%v clearBefore=%v\n",
			text, x, y, pressEnter, clearBefore)
	}
	c.addNote(fmt.Sprintf("⌨️  Type text at (%d, %d): '%s'", x, y, text))

	// First click at the position to focus
	if _, err := c.ClickAt(x, y); err != nil {
		return nil, fmt.Errorf("focusing element: %w", err)
	}

	ctx := c.page.Context()

	// Clear existing text if requested
	if clearBefore {
		// Select all and delete
		if err := chromedp.Run(ctx,
			chromedp.KeyEvent("\ue009a"), // Ctrl+A
			chromedp.KeyEvent("\b"),      // Backspace
		); err != nil {
			return nil, fmt.Errorf("clearing text: %w", err)
		}
	}

	// Type the text
	if err := chromedp.Run(ctx,
		chromedp.KeyEvent(text),
	); err != nil {
		return nil, fmt.Errorf("typing text: %w", err)
	}

	// Press Enter if requested
	if pressEnter {
		if err := chromedp.Run(ctx,
			chromedp.KeyEvent("\r"),
		); err != nil {
			return nil, fmt.Errorf("pressing enter: %w", err)
		}
	}

	time.Sleep(500 * time.Millisecond)
	return c.CurrentState()
}

// DragAndDrop performs a drag and drop operation.
func (c *BrowserComputer) DragAndDrop(x, y, destX, destY int) (*EnvState, error) {
	// Denormalize coordinates
	pixelX := float64(x*c.width) / 1000.0
	pixelY := float64(y*c.height) / 1000.0
	pixelDestX := float64(destX*c.width) / 1000.0
	pixelDestY := float64(destY*c.height) / 1000.0

	ctx := c.page.Context()
	if err := chromedp.Run(ctx,
		chromedp.MouseEvent(input.MousePressed, pixelX, pixelY, chromedp.Button("left")),
		chromedp.MouseEvent(input.MouseMoved, pixelDestX, pixelDestY),
		chromedp.MouseEvent(input.MouseReleased, pixelDestX, pixelDestY, chromedp.Button("left")),
	); err != nil {
		return nil, fmt.Errorf("drag and drop: %w", err)
	}

	time.Sleep(500 * time.Millisecond)
	return c.CurrentState()
}

// Navigate navigates to a URL.
func (c *BrowserComputer) Navigate(url string) (*EnvState, error) {
	c.addNote(fmt.Sprintf("🌐 Navigate to: %s", url))

	if err := c.browser.Navigate(url); err != nil {
		return nil, fmt.Errorf("navigating to %s: %w", url, err)
	}

	time.Sleep(1 * time.Second)

	c.addScreenshot(fmt.Sprintf("After navigating to %s", url))
	return c.CurrentState()
}

// GoBack navigates back in history.
func (c *BrowserComputer) GoBack() (*EnvState, error) {
	ctx := c.page.Context()
	if err := chromedp.Run(ctx, chromedp.NavigateBack()); err != nil {
		return nil, fmt.Errorf("going back: %w", err)
	}

	time.Sleep(1 * time.Second)
	return c.CurrentState()
}

// GoForward navigates forward in history.
func (c *BrowserComputer) GoForward() (*EnvState, error) {
	ctx := c.page.Context()
	if err := chromedp.Run(ctx, chromedp.NavigateForward()); err != nil {
		return nil, fmt.Errorf("going forward: %w", err)
	}

	time.Sleep(1 * time.Second)
	return c.CurrentState()
}

// Search navigates to Google.
func (c *BrowserComputer) Search() (*EnvState, error) {
	return c.Navigate("https://www.google.com")
}

// ScrollDocument scrolls the entire page.
// Uses keyboard events (PageDown/PageUp) for vertical scrolling like Google's Playwright implementation,
// which is more reliable than JavaScript scrolling as it works with any focused scrollable element.
func (c *BrowserComputer) ScrollDocument(direction string) (*EnvState, error) {
	switch direction {
	case "up":
		// Use PageUp key for vertical scroll up (like Google's Playwright implementation)
		if c.verbose {
			fmt.Fprintf(os.Stderr, "📜 Scroll up: sending PageUp key\n")
		}
		return c.KeyCombination([]string{"PageUp"})

	case "down":
		// Use PageDown key for vertical scroll down (like Google's Playwright implementation)
		if c.verbose {
			fmt.Fprintf(os.Stderr, "📜 Scroll down: sending PageDown key\n")
		}
		return c.KeyCombination([]string{"PageDown"})

	case "left", "right":
		// For horizontal scrolling, use JavaScript like Google's implementation
		// Scroll by 50% of viewport width
		ctx := c.page.Context()
		scrollAmount := c.width / 2
		deltaX := scrollAmount
		if direction == "left" {
			deltaX = -scrollAmount
		}

		script := fmt.Sprintf(`window.scrollBy(%d, 0);`, deltaX)
		if err := chromedp.Run(ctx, chromedp.Evaluate(script, nil)); err != nil {
			return nil, fmt.Errorf("scrolling %s: %w", direction, err)
		}
		if c.verbose {
			fmt.Fprintf(os.Stderr, "📜 Scroll %s: scrolled by %d pixels\n", direction, deltaX)
		}

		time.Sleep(300 * time.Millisecond)
		c.addScreenshot(fmt.Sprintf("After scroll %s", direction))
		return c.CurrentState()

	default:
		return nil, fmt.Errorf("invalid direction: %s", direction)
	}
}

// ScrollAt scrolls at specific coordinates.
func (c *BrowserComputer) ScrollAt(x, y int, direction string, magnitude float64) (*EnvState, error) {
	// Denormalize coordinates
	pixelX := x * c.width / 1000
	pixelY := y * c.height / 1000

	var deltaX, deltaY float64
	switch direction {
	case "up":
		deltaY = -magnitude
	case "down":
		deltaY = magnitude
	case "left":
		deltaX = -magnitude
	case "right":
		deltaX = magnitude
	}

	ctx := c.page.Context()
	script := fmt.Sprintf(`
		const el = document.elementFromPoint(%d, %d);
		if (el) el.scrollBy(%f, %f);
	`, pixelX, pixelY, deltaX, deltaY)

	if err := chromedp.Run(ctx, chromedp.Evaluate(script, nil)); err != nil {
		return nil, fmt.Errorf("scrolling at position: %w", err)
	}

	time.Sleep(300 * time.Millisecond)
	return c.CurrentState()
}

// KeyCombination presses a key combination.
func (c *BrowserComputer) KeyCombination(keys []string) (*EnvState, error) {
	ctx := c.page.Context()

	// Map common key names to CDP key codes
	for _, key := range keys {
		var keyEvent string
		switch key {
		case "Control", "Ctrl":
			keyEvent = "\ue009"
		case "Shift":
			keyEvent = "\ue008"
		case "Alt":
			keyEvent = "\ue00a"
		case "Meta", "Command", "Cmd":
			keyEvent = "\ue03d"
		case "Enter":
			keyEvent = "\r"
		case "Backspace":
			keyEvent = "\b"
		case "Tab":
			keyEvent = "\t"
		case "Escape", "Esc":
			keyEvent = "\ue00c"
		case "PageDown":
			keyEvent = "\ue00f"
		case "PageUp":
			keyEvent = "\ue00e"
		default:
			keyEvent = key
		}

		if err := chromedp.Run(ctx, chromedp.KeyEvent(keyEvent)); err != nil {
			return nil, fmt.Errorf("pressing key %s: %w", key, err)
		}
	}

	time.Sleep(200 * time.Millisecond)
	return c.CurrentState()
}

// Wait5Seconds waits for 5 seconds.
func (c *BrowserComputer) Wait5Seconds() (*EnvState, error) {
	time.Sleep(5 * time.Second)
	return c.CurrentState()
}

// CurrentState returns the current browser state.
func (c *BrowserComputer) CurrentState() (*EnvState, error) {
	// Get current URL
	url, err := c.browser.GetURL()
	if err != nil {
		return nil, fmt.Errorf("getting URL: %w", err)
	}

	// Take screenshot
	screenshot, err := c.takeScreenshot()
	if err != nil {
		return nil, fmt.Errorf("taking screenshot: %w", err)
	}

	return &EnvState{
		Screenshot: screenshot,
		URL:        url,
	}, nil
}

// Close closes the browser and saves HAR if configured.
func (c *BrowserComputer) Close() error {
	// Save HAR if recorder is configured
	if c.harRecorder != nil && c.harOutput != "" {
		if err := c.saveHAR(); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to save HAR: %v\n", err)
		}
	}
	return c.browser.Close()
}

// saveHAR saves the recorded HAR to the output file.
func (c *BrowserComputer) saveHAR() error {
	harData, err := c.harRecorder.HAR()
	if err != nil {
		return fmt.Errorf("getting HAR data: %w", err)
	}

	harJSON, err := json.MarshalIndent(harData, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling HAR: %w", err)
	}

	if err := os.WriteFile(c.harOutput, harJSON, 0644); err != nil {
		return fmt.Errorf("writing HAR file: %w", err)
	}

	fmt.Printf("\n📊 HAR file saved to: %s\n", c.harOutput)
	return nil
}

// addNote adds a note to the HAR recording if recorder is enabled.
func (c *BrowserComputer) addNote(description string) {
	if c.harRecorder != nil {
		ctx := c.page.Context()
		_ = c.harRecorder.AddNote(ctx, description)
	}
}

// addScreenshot adds a screenshot to the HAR recording if recorder is enabled.
func (c *BrowserComputer) addScreenshot(description string) {
	if c.harRecorder != nil {
		ctx := c.page.Context()
		_ = c.harRecorder.AddScreenshot(ctx, description)
	}
}

// detectWindowID detects the Chrome/Brave window ID using list-app-windows
func (c *BrowserComputer) detectWindowID(ctx context.Context) error {
	// Try both Brave and Chrome
	apps := []string{"Brave Browser", "Google Chrome"}

	type WindowInfo struct {
		WindowID  int32  `json:"window_id"`
		OwnerName string `json:"owner_name"`
		OwnerPID  int32  `json:"owner_pid"`
		Width     int    `json:"width"`
		Height    int    `json:"height"`
	}

	for _, app := range apps {
		cmd := exec.Command("list-app-windows", "-json", "-app", app)
		output, err := cmd.Output()
		if err != nil {
			if c.verbose {
				fmt.Fprintf(os.Stderr, "list-app-windows error for %s: %v\n", app, err)
			}
			continue // Try next browser
		}

		var windows []WindowInfo
		if err := json.Unmarshal(output, &windows); err != nil {
			continue // Try next browser
		}

		if len(windows) > 0 {
			// Find the largest window (likely the main browser window, not devtools or popups)
			// This helps avoid capturing small inspector windows or popup dialogs
			var bestWindow *WindowInfo
			maxArea := 0

			for i := range windows {
				area := windows[i].Width * windows[i].Height
				if area > maxArea {
					maxArea = area
					bestWindow = &windows[i]
				}
			}

			if bestWindow != nil {
				c.windowID = fmt.Sprintf("%d", bestWindow.WindowID)

				if c.verbose {
					fmt.Fprintf(os.Stderr, "Detected window ID: %s for %s (PID: %d, size: %dx%d)\n",
						c.windowID, app, bestWindow.OwnerPID, bestWindow.Width, bestWindow.Height)
				}

				return nil
			}
		}
	}

	return fmt.Errorf("no windows found for Chrome or Brave")
}

// takeScreenshot captures a PNG screenshot.
func (c *BrowserComputer) takeScreenshot() ([]byte, error) {
	// Use OS-level screenshot if enabled
	if c.useOSScreenshots {
		// If window ID not yet detected, try to detect it now
		if c.windowID == "" {
			ctx := context.Background()
			if err := c.detectWindowID(ctx); err != nil {
				if c.verbose {
					fmt.Fprintf(os.Stderr, "Could not detect window ID for OS screenshot: %v\n", err)
					fmt.Fprintf(os.Stderr, "Using CDP screenshot instead\n")
				}
				// Don't fall back permanently, keep trying on next screenshot
			}
		}

		if c.windowID != "" {
			return c.takeOSScreenshot()
		}
	}

	// Fall back to CDP screenshot
	ctx := c.page.Context()
	var buf []byte

	if err := chromedp.Run(ctx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			var err error
			buf, err = page.CaptureScreenshot().Do(ctx)
			return err
		}),
	); err != nil {
		return nil, err
	}

	// Save CDP screenshot to work directory
	if c.workDir != "" {
		c.screenshotCount++
		screenshotFile := filepath.Join(c.workDir, fmt.Sprintf("screenshot-%04d.png", c.screenshotCount))
		if err := os.WriteFile(screenshotFile, buf, 0644); err != nil {
			if c.verbose {
				fmt.Fprintf(os.Stderr, "Warning: failed to save screenshot to work dir: %v\n", err)
			}
		} else if c.verbose {
			fmt.Fprintf(os.Stderr, "Saved screenshot: %s\n", screenshotFile)
		}
	}

	return buf, nil
}

// takeOSScreenshot captures a screenshot using the screen-capture tool
func (c *BrowserComputer) takeOSScreenshot() ([]byte, error) {
	// Increment screenshot counter
	c.screenshotCount++

	// Save screenshot to work directory with sequential naming
	screenshotFile := filepath.Join(c.workDir, fmt.Sprintf("screenshot-%04d.png", c.screenshotCount))

	// Also create a temp file for immediate reading (in case work dir is not writable)
	tmpFile := filepath.Join(os.TempDir(), fmt.Sprintf("computer-use-agent-%d.png", time.Now().UnixNano()))

	// Use screen-capture tool (macgo-enabled wrapper for screencapture)
	cmd := exec.Command("screen-capture", "-window", c.windowID, tmpFile)

	// Clear MACGO_IN_BUNDLE so screen-capture can create its own independent bundle
	// This allows screen-capture to get its own TCC permissions separate from computer-use-agent
	cmd.Env = os.Environ()
	for i, env := range cmd.Env {
		if strings.HasPrefix(env, "MACGO_IN_BUNDLE=") {
			cmd.Env = append(cmd.Env[:i], cmd.Env[i+1:]...)
			break
		}
	}


	// Capture stdout and stderr to show in verbose mode
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		if c.verbose {
			if stdout.Len() > 0 {
				fmt.Fprintf(os.Stderr, "screen-capture stdout: %s\n", stdout.String())
			}
			if stderr.Len() > 0 {
				fmt.Fprintf(os.Stderr, "screen-capture stderr: %s\n", stderr.String())
			}
		}
		return nil, fmt.Errorf("screen-capture failed: %w (stderr: %s)", err, stderr.String())
	}

	if c.verbose && stdout.Len() > 0 {
		fmt.Fprintf(os.Stderr, "screen-capture stdout: %s\n", stdout.String())
	}

	// Give the file system a moment to sync (screen-capture might return before file is fully written)
	time.Sleep(200 * time.Millisecond)

	// Check if file exists before trying to read it
	if _, err := os.Stat(tmpFile); err != nil {
		if c.verbose {
			fmt.Fprintf(os.Stderr, "Screenshot file does not exist: %s\n", tmpFile)
			fmt.Fprintf(os.Stderr, "screen-capture may have failed silently\n")
			// Try to capture using screencapture directly as fallback
			fmt.Fprintf(os.Stderr, "Attempting fallback to native screencapture...\n")
		}
		// Try native screencapture as fallback
		fallbackCmd := exec.Command("screencapture", "-l", c.windowID, tmpFile)
		if fallbackErr := fallbackCmd.Run(); fallbackErr != nil {
			return nil, fmt.Errorf("failed to read screenshot %s after screen-capture completed (exit code 0), and fallback screencapture also failed: %w", tmpFile, fallbackErr)
		}
		// Give it another moment
		time.Sleep(100 * time.Millisecond)
	}

	// Read the screenshot file
	data, err := os.ReadFile(tmpFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read screenshot: %w", err)
	}

	// Copy screenshot to work directory for persistence
	if c.workDir != "" {
		if err := os.WriteFile(screenshotFile, data, 0644); err != nil {
			if c.verbose {
				fmt.Fprintf(os.Stderr, "Warning: failed to save screenshot to work dir: %v\n", err)
			}
			// Don't fail - we still have the screenshot data in memory
		} else if c.verbose {
			fmt.Fprintf(os.Stderr, "Saved screenshot: %s\n", screenshotFile)
		}
	}

	// Clean up temp file
	os.Remove(tmpFile)

	return data, nil
}

// EnvState represents the browser environment state.
type EnvState struct {
	Screenshot []byte `json:"screenshot"`
	URL        string `json:"url"`
}

// MarshalJSON customizes JSON encoding to base64-encode the screenshot.
func (e *EnvState) MarshalJSON() ([]byte, error) {
	type Alias EnvState
	return json.Marshal(&struct {
		Screenshot string `json:"screenshot"`
		*Alias
	}{
		Screenshot: encodeBase64(e.Screenshot),
		Alias:      (*Alias)(e),
	})
}

// Helper to encode bytes as base64.
func encodeBase64(data []byte) string {
	if len(data) == 0 {
		return ""
	}
	// Return data URI for PNG
	return "data:image/png;base64," + string(data)
}
