// Package browser provides session detection utilities for Brave and Chromium browsers.
package browser

import (
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"errors"
)

// SessionDetector detects running browser sessions and manages session isolation.
type SessionDetector struct {
	verbose bool
}

// NewSessionDetector creates a new session detector.
func NewSessionDetector(verbose bool) *SessionDetector {
	return &SessionDetector{
		verbose: verbose,
	}
}

// logf logs a message if verbose mode is enabled.
func (sd *SessionDetector) logf(format string, args ...interface{}) {
	if sd.verbose {
		log.Printf(format, args...)
	}
}

// BrowserSession represents a running browser session.
type BrowserSession struct {
	PID            int
	BrowserType    string // "brave" or "chrome"
	DebugPort      int
	ProfilePath    string
	IsDebugEnabled bool
}

// DetectBraveSession checks if Brave browser is already running.
// Returns true if a Brave process is detected.
func (sd *SessionDetector) DetectBraveSession(ctx context.Context) bool {
	if runtime.GOOS == "windows" {
		return sd.detectBraveProcessWindows()
	} else if runtime.GOOS == "darwin" {
		return sd.detectBraveProcessDarwin()
	} else if runtime.GOOS == "linux" {
		return sd.detectBraveProcessLinux()
	}
	return false
}

// detectBraveProcessDarwin checks for running Brave on macOS.
func (sd *SessionDetector) detectBraveProcessDarwin() bool {
	cmd := exec.Command("pgrep", "-f", "Brave Browser")
	output, err := cmd.Output()
	if err != nil || len(output) == 0 {
		sd.logf("No Brave process detected on macOS")
		return false
	}
	sd.logf("Brave process detected on macOS: %s", strings.TrimSpace(string(output)))
	return true
}

// detectBraveProcessLinux checks for running Brave on Linux.
func (sd *SessionDetector) detectBraveProcessLinux() bool {
	cmd := exec.Command("pgrep", "-f", "brave")
	output, err := cmd.Output()
	if err != nil || len(output) == 0 {
		sd.logf("No Brave process detected on Linux")
		return false
	}
	sd.logf("Brave process detected on Linux: %s", strings.TrimSpace(string(output)))
	return true
}

// detectBraveProcessWindows checks for running Brave on Windows.
func (sd *SessionDetector) detectBraveProcessWindows() bool {
	cmd := exec.Command("tasklist", "/FI", "IMAGENAME eq brave.exe")
	output, err := cmd.Output()
	if err != nil {
		sd.logf("Error checking for Brave on Windows: %v", err)
		return false
	}
	if strings.Contains(string(output), "brave.exe") {
		sd.logf("Brave process detected on Windows")
		return true
	}
	sd.logf("No Brave process detected on Windows")
	return false
}

// IsPortAvailable checks if a port is available for binding.
func (sd *SessionDetector) IsPortAvailable(port int) bool {
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		sd.logf("Port %d is not available: %v", port, err)
		return false
	}
	defer listener.Close()
	sd.logf("Port %d is available", port)
	return true
}

// VerifyDevToolsPort checks if DevTools is listening on the specified port.
func (sd *SessionDetector) VerifyDevToolsPort(ctx context.Context, port int, timeout time.Duration) (bool, error) {
	url := fmt.Sprintf("http://127.0.0.1:%d/json/version", port)

	// Create a client with timeout
	client := &http.Client{
		Timeout: timeout,
	}

	// Create a request with the provided context
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return false, fmt.Errorf("creating request for DevTools verification: %w", err)
	}

	// Perform the request
	resp, err := client.Do(req)
	if err != nil {
		sd.logf("DevTools port %d verification failed: %v", port, err)
		return false, nil // Port not responding, but not an error per se
	}
	defer resp.Body.Close()

	if resp.StatusCode == 200 {
		sd.logf("DevTools verified on port %d", port)
		return true, nil
	}

	sd.logf("DevTools port %d returned status %d", port, resp.StatusCode)
	return false, nil
}

// WaitForDevTools polls the DevTools endpoint until it's available or timeout occurs.
func (sd *SessionDetector) WaitForDevTools(ctx context.Context, port int, maxWait time.Duration) error {
	deadline := time.Now().Add(maxWait)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(maxWait):
			return errors.New("timeout waiting for DevTools to become available")
		case <-ticker.C:
			// Check if DevTools is responding
			checkCtx, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
			available, _ := sd.VerifyDevToolsPort(checkCtx, port, 500*time.Millisecond)
			cancel()

			if available {
				sd.logf("DevTools became available on port %d", port)
				return nil
			}

			if time.Now().After(deadline) {
				return fmt.Errorf("timeout waiting for DevTools on port %d", port)
			}

			sd.logf("Waiting for DevTools on port %d...", port)
		}
	}
}

// EnumerateTabsWithRetry attempts to enumerate tabs with retry logic.
// Returns the raw JSON response from /json/list endpoint.
func (sd *SessionDetector) EnumerateTabsWithRetry(ctx context.Context, port int, maxRetries int) (string, error) {
	url := fmt.Sprintf("http://127.0.0.1:%d/json/list", port)
	client := &http.Client{
		Timeout: 5 * time.Second,
	}

	var lastErr error
	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			sd.logf("Tab enumeration attempt %d/%d", attempt+1, maxRetries)
			// Wait before retrying
			select {
			case <-ctx.Done():
				return "", ctx.Err()
			case <-time.After(200 * time.Millisecond):
			}
		}

		req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
		if err != nil {
			lastErr = fmt.Errorf("creating request for tab enumeration: %w", err)
			continue
		}

		resp, err := client.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("tab enumeration request failed: %w", err)
			continue
		}

		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()

		if err != nil {
			lastErr = fmt.Errorf("reading tab enumeration response: %w", err)
			continue
		}

		result := string(body)
		// Check for empty response (session reuse problem)
		if result == "" || result == "[]" {
			sd.logf("Empty tab list returned (possible session reuse issue)")
			// Continue retrying
			continue
		}

		sd.logf("Successfully enumerated tabs: %s", result[:min(len(result), 100)])
		return result, nil
	}

	// All retries exhausted
	if lastErr != nil {
		return "", fmt.Errorf(fmt.Sprintf("failed to enumerate tabs after %d attempts", maxRetries)+": %w", lastErr)
	}
	return "", fmt.Errorf("failed to enumerate tabs after %d attempts", maxRetries)
}

// min returns the minimum of two integers.
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// NeedsBraveSessionIsolation determines if Brave session isolation is needed.
// Returns true if:
// 1. Running Brave browser
// 2. Using an existing profile
// 3. A Brave process is already running
func (sd *SessionDetector) NeedsBraveSessionIsolation(ctx context.Context, browserPath string, usingExistingProfile bool) bool {
	// Check if this is Brave
	if !strings.Contains(strings.ToLower(browserPath), "brave") {
		return false
	}

	// Only needed if using an existing profile
	if !usingExistingProfile {
		return false
	}

	// Check if Brave is already running
	return sd.DetectBraveSession(ctx)
}

// ImportantWarning returns a user-friendly warning message when session isolation is needed.
func (sd *SessionDetector) ImportantWarning() string {
	return `
╔════════════════════════════════════════════════════════════════════════════╗
║ WARNING: Brave Session Reuse Detected                                      ║
║                                                                            ║
║ An existing Brave instance was detected. Brave's session reuse behavior   ║
║ prevents CDP tab enumeration when launching with an existing profile.     ║
║                                                                            ║
║ Creating isolated session with unique profile directory...                ║
║                                                                            ║
║ This ensures:                                                              ║
║ • Browser can be launched with CDP support                                ║
║ • /json/list endpoint returns available tabs                              ║
║ • Profile data (cookies, history, bookmarks) is still accessible          ║
╚════════════════════════════════════════════════════════════════════════════╝
`
}
