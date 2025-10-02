// The CDP command-line tool for Chrome DevTools Protocol interaction.
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"regexp"
	goruntime "runtime"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/cdproto/runtime"
	"github.com/chromedp/cdproto/target"
	"github.com/chromedp/chromedp"
	"github.com/pkg/errors"
	"github.com/tmc/misc/chrome-to-har/internal/browser"
	"github.com/tmc/misc/chrome-to-har/internal/chromeprofiles"
	harrecorder "github.com/tmc/misc/chrome-to-har/internal/recorder"
)

// stringSlice implements flag.Value for multiple string values
type stringSlice []string

func (s *stringSlice) String() string {
	return strings.Join(*s, ",")
}

func (s *stringSlice) Set(value string) error {
	*s = append(*s, value)
	return nil
}

// Exit codes following Unix conventions
const (
	ExitSuccess         = 0 // Success
	ExitGeneralError    = 1 // General error
	ExitUsageError      = 2 // Command line usage error
	ExitBrowserError    = 3 // Browser launch/connection failed
	ExitNavigationError = 4 // Page navigation failed
	ExitTimeout         = 5 // Operation timed out
)

// Error types for machine-parseable error messages
const (
	ErrorTypeGeneral    = "general_error"
	ErrorTypeUsage      = "usage_error"
	ErrorTypeBrowser    = "browser_error"
	ErrorTypeNavigation = "navigation_error"
	ErrorTypeTimeout    = "timeout_error"
	ErrorTypeJavaScript = "javascript_error"
	ErrorTypeProfile    = "profile_error"
	ErrorTypeNetwork    = "network_error"
)

// CDPError represents a machine-parseable error with type and message
type CDPError struct {
	Type    string `json:"type"`
	Message string `json:"message"`
	Code    int    `json:"code"`
}

// Global format flag for errors (set during flag parsing)
var errorFormat = "text"

// exitWithError prints an error message and exits with the specified code
// Uses consistent format: machine-parseable with type information
func exitWithError(code int, errorType string, format string, args ...interface{}) {
	message := fmt.Sprintf(format, args...)

	if errorFormat == "json" {
		// Output structured JSON error
		err := CDPError{
			Type:    errorType,
			Message: message,
			Code:    code,
		}
		data, _ := json.MarshalIndent(err, "", "  ")
		fmt.Fprintf(os.Stderr, "%s\n", string(data))
	} else {
		// Output human-readable error to stderr with type prefix
		fmt.Fprintf(os.Stderr, "Error: [%s] %s\n", errorType, message)
	}
	os.Exit(code)
}

// exitWithCode exits with the specified code (for success or silent failures)
func exitWithCode(code int) {
	os.Exit(code)
}

var aliases = map[string]string{
	// Shortcuts for common operations
	"goto":       `Page.navigate {"url":"$1"}`,
	"reload":     `Page.reload {}`,
	"title":      `Runtime.evaluate {"expression":"document.title"}`,
	"url":        `Runtime.evaluate {"expression":"window.location.href"}`,
	"html":       `Runtime.evaluate {"expression":"document.documentElement.outerHTML"}`,
	"cookies":    `Network.getAllCookies {}`,
	"screenshot": `Page.captureScreenshot {}`,
	"pdf":        `Page.printToPDF {}`,

	// Debugging
	"pause":  `Debugger.pause {}`,
	"resume": `Debugger.resume {}`,
	"step":   `Debugger.stepInto {}`,
	"next":   `Debugger.stepOver {}`,
	"out":    `Debugger.stepOut {}`,

	// DOM interaction
	"click": `Runtime.evaluate {"expression":"document.querySelector('$1').click()"}`,
	"focus": `Runtime.evaluate {"expression":"document.querySelector('$1').focus()"}`,
	"type":  `Input.insertText {"text":"$1"}`,

	// Device emulation
	"mobile":  `Emulation.setDeviceMetricsOverride {"width":375,"height":812,"deviceScaleFactor":3,"mobile":true}`,
	"desktop": `Emulation.clearDeviceMetricsOverride {}`,

	// Performance & coverage
	"metrics":        `Performance.getMetrics {}`,
	"coverage_start": `Profiler.startPreciseCoverage {"callCount":true,"detailed":true}`,
	"coverage_take":  `Profiler.takePreciseCoverage {}`,
	"coverage_stop":  `Profiler.stopPreciseCoverage {}`,

	// Enhanced aliases for Playwright-like commands
	"wait":     `@wait $1`, // Custom command prefix @
	"waitfor":  `@waitfor $1`,
	"text":     `@text $1`,
	"hover":    `@hover $1`,
	"select":   `@select $1 $2`,
	"check":    `@check $1`,
	"uncheck":  `@uncheck $1`,
	"press":    `@press $1`,
	"fill":     `@fill $1 $2`,
	"clear":    `@clear $1`,
	"visible":  `@visible $1`,
	"hidden":   `@hidden $1`,
	"enabled":  `@enabled $1`,
	"disabled": `@disabled $1`,
	"count":    `@count $1`,
	"attr":     `@attr $1 $2`,
	"css":      `@css $1 $2`,

	// Network and security
	"headers":      `Network.getResponseHeaders {"requestId":"$1"}`,
	"block":        `Network.setBlockedURLs {"urls":["$1"]}`,
	"throttle":     `Network.emulateNetworkConditions {"offline":false,"downloadThroughput":$1,"uploadThroughput":$2,"latency":$3}`,
	"offline":      `Network.emulateNetworkConditions {"offline":true}`,
	"online":       `Network.emulateNetworkConditions {"offline":false}`,
	"clearcache":   `Network.clearBrowserCache {}`,
	"clearcookies": `Network.clearBrowserCookies {}`,
	"setcookie":    `Network.setCookie {"name":"$1","value":"$2","domain":"$3"}`,
	"deletecookie": `Network.deleteCookies {"name":"$1"}`,

	// Console and logging
	"console":       `Runtime.enable {}`,
	"log":           `Runtime.evaluate {"expression":"console.log('$1')"}`,
	"error":         `Runtime.evaluate {"expression":"console.error('$1')"}`,
	"warn":          `Runtime.evaluate {"expression":"console.warn('$1')"}`,
	"clear_console": `Runtime.evaluate {"expression":"console.clear()"}`,

	// Storage
	"localstorage":   `Runtime.evaluate {"expression":"JSON.stringify(localStorage)"}`,
	"sessionstorage": `Runtime.evaluate {"expression":"JSON.stringify(sessionStorage)"}`,
	"setlocal":       `Runtime.evaluate {"expression":"localStorage.setItem('$1', '$2')"}`,
	"setsession":     `Runtime.evaluate {"expression":"sessionStorage.setItem('$1', '$2')"}`,
	"clearlocal":     `Runtime.evaluate {"expression":"localStorage.clear()"}`,
	"clearsession":   `Runtime.evaluate {"expression":"sessionStorage.clear()"}`,

	// Page manipulation
	"scrollto":     `Runtime.evaluate {"expression":"window.scrollTo($1, $2)"}`,
	"scrollby":     `Runtime.evaluate {"expression":"window.scrollBy($1, $2)"}`,
	"scrolltop":    `Runtime.evaluate {"expression":"window.scrollTo(0, 0)"}`,
	"scrollbottom": `Runtime.evaluate {"expression":"window.scrollTo(0, document.body.scrollHeight)"}`,
	"zoomin":       `Emulation.setPageScaleFactor {"pageScaleFactor":$1}`,
	"zoomreset":    `Emulation.setPageScaleFactor {"pageScaleFactor":1}`,
	"darkmode":     `Emulation.setEmulatedMedia {"features":[{"name":"prefers-color-scheme","value":"dark"}]}`,
	"lightmode":    `Emulation.setEmulatedMedia {"features":[{"name":"prefers-color-scheme","value":"light"}]}`,

	// Viewport and display
	"viewport":   `Emulation.setDeviceMetricsOverride {"width":$1,"height":$2,"deviceScaleFactor":1,"mobile":false}`,
	"fullscreen": `Emulation.setDeviceMetricsOverride {"width":1920,"height":1080,"deviceScaleFactor":1,"mobile":false}`,
	"tablet":     `Emulation.setDeviceMetricsOverride {"width":768,"height":1024,"deviceScaleFactor":2,"mobile":true}`,

	// Advanced debugging
	"heap":         `HeapProfiler.takeHeapSnapshot {}`,
	"startcpu":     `Profiler.start {}`,
	"stopcpu":      `Profiler.stop {}`,
	"memory":       `Runtime.evaluate {"expression":"performance.memory"}`,
	"timing":       `Runtime.evaluate {"expression":"JSON.stringify(performance.timing)"}`,
	"paint":        `Runtime.evaluate {"expression":"JSON.stringify(performance.getEntriesByType('paint'))"}`,
	"route":        `@route $1 $2`,
	"waitrequest":  `@waitrequest $1`,
	"waitresponse": `@waitresponse $1`,
}

// BrowserCandidate represents a potential browser installation
type BrowserCandidate struct {
	Name      string
	Path      string
	Version   string
	IsRunning bool
	ProcessID int
	DebugPort int
}

// ChromeTab represents a Chrome tab
type ChromeTab struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	URL   string `json:"url"`
	Type  string `json:"type"`
}

// HAREntry represents a single HAR entry
type HAREntry struct {
	StartedDateTime string                 `json:"startedDateTime"`
	Request         map[string]interface{} `json:"request"`
	Response        map[string]interface{} `json:"response"`
	Time            float64                `json:"time"`
}

// HARLog represents the HAR log structure
type HARLog struct {
	Version string        `json:"version"`
	Creator interface{}   `json:"creator"`
	Pages   []interface{} `json:"pages"`
	Entries []HAREntry    `json:"entries"`
}

// HAR represents the top-level HAR structure
type HAR struct {
	Log HARLog `json:"log"`
}

// NetworkRecorder records network events for HAR generation
type NetworkRecorder struct {
	entries []HAREntry
	mu      sync.RWMutex
}

// AddEntry adds a new HAR entry to the recorder
func (nr *NetworkRecorder) AddEntry(entry HAREntry) {
	nr.mu.Lock()
	defer nr.mu.Unlock()
	nr.entries = append(nr.entries, entry)
}

// GetEntries returns all recorded HAR entries
func (nr *NetworkRecorder) GetEntries() []HAREntry {
	nr.mu.RLock()
	defer nr.mu.RUnlock()
	return append([]HAREntry(nil), nr.entries...)
}

// SaveHAR saves the recorded entries to a HAR file
func (nr *NetworkRecorder) SaveHAR(filename string) error {
	entries := nr.GetEntries()
	har := HAR{
		Log: HARLog{
			Version: "1.2",
			Creator: map[string]interface{}{
				"name":    "CDP-Enhanced",
				"version": "1.0",
			},
			Pages:   []interface{}{},
			Entries: entries,
		},
	}

	data, err := json.MarshalIndent(har, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(filename, data, 0644)
}

// checkRunningChrome checks if Chrome is running on a specific port
func checkRunningChrome(port int) bool {
	resp, err := http.Get(fmt.Sprintf("http://localhost:%d/json/version", port))
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == 200
}

// getChromeTabs gets list of available tabs from Chrome
func getChromeTabs(port int) ([]ChromeTab, error) {
	resp, err := http.Get(fmt.Sprintf("http://localhost:%d/json/list", port))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var tabs []ChromeTab
	if err := json.NewDecoder(resp.Body).Decode(&tabs); err != nil {
		return nil, err
	}

	return tabs, nil
}

// discoverBrowsers finds all available browser installations and running processes
func discoverBrowsers(verbose bool) ([]BrowserCandidate, error) {
	var candidates []BrowserCandidate

	// Check for running browsers first
	runningBrowsers, err := findRunningBrowsers(verbose)
	if err != nil && verbose {
		log.Printf("Warning: failed to find running browsers: %v", err)
	}
	candidates = append(candidates, runningBrowsers...)

	// Check for installed browsers
	installedBrowsers, err := findInstalledBrowsers(verbose)
	if err != nil && verbose {
		log.Printf("Warning: failed to find installed browsers: %v", err)
	}
	candidates = append(candidates, installedBrowsers...)

	return candidates, nil
}

// isMainBrowserExecutable checks if the path is the main browser executable (not a helper process)
func isMainBrowserExecutable(path string) bool {
	// Skip helper processes - these are support processes, not the main browser
	if strings.Contains(path, "/Helpers/") {
		return false
	}
	if strings.Contains(path, "/Frameworks/") {
		return false
	}
	if strings.Contains(path, " Helper") {
		return false
	}
	// Main browser executables end with the browser name in MacOS
	// e.g., "/Applications/Brave Browser.app/Contents/MacOS/Brave Browser"
	//       "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome"
	return true
}

// findRunningBrowsers detects currently running browser processes
func findRunningBrowsers(verbose bool) ([]BrowserCandidate, error) {
	var candidates []BrowserCandidate

	// Use ps to find running browser processes
	cmd := exec.Command("ps", "aux")
	output, err := cmd.Output()
	if err != nil {
		return candidates, err
	}

	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		if strings.Contains(line, "Chrome") || strings.Contains(line, "Chromium") ||
			strings.Contains(line, "Brave") || strings.Contains(line, "Edge") {

			// Parse the process line to extract useful information
			fields := strings.Fields(line)
			if len(fields) < 11 {
				continue
			}

			_ = filepath.Base(fields[10]) // processName not used
			var browserName, browserPath string
			var debugPort int

			// Extract browser info
			if strings.Contains(line, "Google Chrome Canary") {
				browserName = "Chrome Canary"
				browserPath = extractExecutablePath(line, "Google Chrome Canary")
			} else if strings.Contains(line, "Google Chrome") {
				browserName = "Chrome"
				browserPath = extractExecutablePath(line, "Google Chrome")
			} else if strings.Contains(line, "Chromium") {
				browserName = "Chromium"
				browserPath = extractExecutablePath(line, "Chromium")
			} else if strings.Contains(line, "Brave") {
				browserName = "Brave"
				browserPath = extractExecutablePath(line, "Brave")
			}

			// Skip if we couldn't extract a path or if it's a helper process
			if browserPath == "" || !isMainBrowserExecutable(browserPath) {
				continue
			}

			// Extract debug port if present
			if strings.Contains(line, "--remote-debugging-port=") {
				portStr := extractFlag(line, "--remote-debugging-port=")
				if portStr != "" {
					fmt.Sscanf(portStr, "%d", &debugPort)
				}
			}

			if browserName != "" && browserPath != "" {
				candidate := BrowserCandidate{
					Name:      browserName,
					Path:      browserPath,
					IsRunning: true,
					DebugPort: debugPort,
				}

				// Avoid duplicates
				found := false
				for _, existing := range candidates {
					if existing.Path == candidate.Path && existing.DebugPort == candidate.DebugPort {
						found = true
						break
					}
				}

				if !found {
					candidates = append(candidates, candidate)
					if verbose {
						log.Printf("Found running browser: %s at %s (debug port: %d)",
							browserName, browserPath, debugPort)
					}
				}
			}
		}
	}

	return candidates, nil
}

// findInstalledBrowsers looks for browser installations on the system
func findInstalledBrowsers(verbose bool) ([]BrowserCandidate, error) {
	var candidates []BrowserCandidate

	switch goruntime.GOOS {
	case "darwin":
		return findMacOSBrowsers(verbose)
	case "linux":
		return findLinuxBrowsers(verbose)
	case "windows":
		return findWindowsBrowsers(verbose)
	default:
		return candidates, fmt.Errorf("unsupported operating system: %s", goruntime.GOOS)
	}
}

// findMacOSBrowsers finds browser installations on macOS
func findMacOSBrowsers(verbose bool) ([]BrowserCandidate, error) {
	var candidates []BrowserCandidate

	// macOS browser paths in order of preference
	browserPaths := []struct {
		name string
		path string
	}{
		{"Brave", "/Applications/Brave Browser.app/Contents/MacOS/Brave Browser"},
		{"Chrome Canary", "/Applications/Google Chrome Canary.app/Contents/MacOS/Google Chrome Canary"},
		{"Chrome", "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome"},
		{"Chrome Beta", "/Applications/Google Chrome Beta.app/Contents/MacOS/Google Chrome Beta"},
		{"Chrome Dev", "/Applications/Google Chrome Dev.app/Contents/MacOS/Google Chrome Dev"},
		{"Chromium", "/Applications/Chromium.app/Contents/MacOS/Chromium"},
		{"Edge", "/Applications/Microsoft Edge.app/Contents/MacOS/Microsoft Edge"},
		{"Edge Beta", "/Applications/Microsoft Edge Beta.app/Contents/MacOS/Microsoft Edge Beta"},
		{"Edge Dev", "/Applications/Microsoft Edge Dev.app/Contents/MacOS/Microsoft Edge Dev"},
		{"Vivaldi", "/Applications/Vivaldi.app/Contents/MacOS/Vivaldi"},
		{"Opera", "/Applications/Opera.app/Contents/MacOS/Opera"},
		{"Chrome for Testing", "/Users/" + os.Getenv("USER") + "/.cache/puppeteer/chrome/*/chrome-mac*/Google Chrome for Testing.app/Contents/MacOS/Google Chrome for Testing"},
	}

	for _, browser := range browserPaths {
		// Handle glob patterns for Chrome for Testing
		if strings.Contains(browser.path, "*") {
			matches, err := filepath.Glob(browser.path)
			if err == nil {
				for _, match := range matches {
					if _, err := os.Stat(match); err == nil {
						version := extractVersionFromPath(match)
						candidates = append(candidates, BrowserCandidate{
							Name:    browser.name,
							Path:    match,
							Version: version,
						})
						if verbose {
							log.Printf("Found browser: %s at %s (version: %s)", browser.name, match, version)
						}
					}
				}
			}
		} else {
			if _, err := os.Stat(browser.path); err == nil {
				version := getBrowserVersion(browser.path)
				candidates = append(candidates, BrowserCandidate{
					Name:    browser.name,
					Path:    browser.path,
					Version: version,
				})
				if verbose {
					log.Printf("Found browser: %s at %s (version: %s)", browser.name, browser.path, version)
				}
			}
		}
	}

	return candidates, nil
}

// findLinuxBrowsers finds browser installations on Linux
func findLinuxBrowsers(verbose bool) ([]BrowserCandidate, error) {
	var candidates []BrowserCandidate

	// Common Linux browser commands
	browserCommands := []struct {
		name    string
		command string
	}{
		{"Brave", "brave-browser"},
		{"Chrome", "google-chrome"},
		{"Chrome Beta", "google-chrome-beta"},
		{"Chrome Dev", "google-chrome-unstable"},
		{"Chromium", "chromium"},
		{"Chromium Browser", "chromium-browser"},
		{"Edge", "microsoft-edge"},
		{"Edge Beta", "microsoft-edge-beta"},
		{"Edge Dev", "microsoft-edge-dev"},
		{"Vivaldi", "vivaldi"},
		{"Opera", "opera"},
	}

	for _, browser := range browserCommands {
		if path, err := exec.LookPath(browser.command); err == nil {
			version := getBrowserVersion(path)
			candidates = append(candidates, BrowserCandidate{
				Name:    browser.name,
				Path:    path,
				Version: version,
			})
			if verbose {
				log.Printf("Found browser: %s at %s (version: %s)", browser.name, path, version)
			}
		}
	}

	return candidates, nil
}

// findWindowsBrowsers finds browser installations on Windows
func findWindowsBrowsers(verbose bool) ([]BrowserCandidate, error) {
	var candidates []BrowserCandidate

	// Common Windows browser paths
	programFiles := os.Getenv("PROGRAMFILES")
	programFilesX86 := os.Getenv("PROGRAMFILES(X86)")
	localAppData := os.Getenv("LOCALAPPDATA")

	browserPaths := []struct {
		name string
		path string
	}{
		{"Chrome", filepath.Join(programFiles, "Google", "Chrome", "Application", "chrome.exe")},
		{"Chrome", filepath.Join(programFilesX86, "Google", "Chrome", "Application", "chrome.exe")},
		{"Chrome", filepath.Join(localAppData, "Google", "Chrome", "Application", "chrome.exe")},
		{"Edge", filepath.Join(programFiles, "Microsoft", "Edge", "Application", "msedge.exe")},
		{"Edge", filepath.Join(programFilesX86, "Microsoft", "Edge", "Application", "msedge.exe")},
		{"Brave", filepath.Join(localAppData, "BraveSoftware", "Brave-Browser", "Application", "brave.exe")},
		{"Vivaldi", filepath.Join(localAppData, "Vivaldi", "Application", "vivaldi.exe")},
		{"Opera", filepath.Join(localAppData, "Programs", "Opera", "opera.exe")},
	}

	for _, browser := range browserPaths {
		if _, err := os.Stat(browser.path); err == nil {
			version := getBrowserVersion(browser.path)
			candidates = append(candidates, BrowserCandidate{
				Name:    browser.name,
				Path:    browser.path,
				Version: version,
			})
			if verbose {
				log.Printf("Found browser: %s at %s (version: %s)", browser.name, browser.path, version)
			}
		}
	}

	return candidates, nil
}

// extractExecutablePath extracts the full executable path from a process line
func extractExecutablePath(processLine, browserName string) string {
	// Look for .app/Contents/MacOS/ pattern which is standard for macOS apps
	if strings.Contains(processLine, ".app/Contents/MacOS/") {
		start := strings.Index(processLine, "/Applications/")
		if start == -1 {
			// Try other common locations
			start = strings.Index(processLine, "/Users/")
		}
		if start != -1 {
			// Find the end of the executable path (before any flags like --)
			remainder := processLine[start:]
			// Look for MacOS/BrowserName pattern and extract until space after browser name
			macosIdx := strings.Index(remainder, "/MacOS/")
			if macosIdx != -1 {
				// Find end of path: look for common flag patterns or end of line
				afterMacOS := remainder[macosIdx+7:] // Skip "/MacOS/"
				// Find first occurrence of " -" (space followed by dash) which indicates a flag
				endIdx := strings.Index(afterMacOS, " -")
				if endIdx == -1 {
					// No flags, take whole remaining string
					return remainder
				}
				// Return path up to but not including the flag
				return remainder[:macosIdx+7+endIdx]
			}
			// Fallback: find first space
			end := strings.Index(remainder, " ")
			if end == -1 {
				return remainder
			}
			return remainder[:end]
		}
	}
	return ""
}

// extractFlag extracts a flag value from a command line
func extractFlag(commandLine, flag string) string {
	index := strings.Index(commandLine, flag)
	if index == -1 {
		return ""
	}

	start := index + len(flag)
	end := strings.Index(commandLine[start:], " ")
	if end == -1 {
		return commandLine[start:]
	}

	return commandLine[start : start+end]
}

// extractVersionFromPath extracts version information from a path
func extractVersionFromPath(path string) string {
	// Extract version from paths like "chrome/mac_arm-131.0.6778.204"
	parts := strings.Split(path, "/")
	for _, part := range parts {
		if strings.Contains(part, ".") && len(part) > 5 {
			// Looks like a version number
			return part
		}
	}
	return "unknown"
}

// splitAndTrim splits a string by separator and trims whitespace
func splitAndTrim(s, sep string) []string {
	if s == "" {
		return nil
	}
	parts := make([]string, 0)
	for _, p := range strings.Split(s, sep) {
		if p = strings.TrimSpace(p); p != "" {
			parts = append(parts, p)
		}
	}
	return parts
}

// getBrowserVersion attempts to get the version of a browser executable
func getBrowserVersion(browserPath string) string {
	cmd := exec.Command(browserPath, "--version")
	output, err := cmd.Output()
	if err != nil {
		return "unknown"
	}

	version := strings.TrimSpace(string(output))
	// Extract just the version number
	parts := strings.Fields(version)
	if len(parts) > 0 {
		return parts[len(parts)-1]
	}

	return "unknown"
}

// selectBestBrowser chooses the best browser from available candidates
func selectBestBrowser(candidates []BrowserCandidate, verbose bool) *BrowserCandidate {
	if len(candidates) == 0 {
		return nil
	}

	// Preference order:
	// 1. Running browsers with debug ports
	// 2. Chrome Canary (latest features)
	// 3. Chrome stable
	// 4. Other Chromium-based browsers

	// First, prefer running browsers with debug ports
	for _, candidate := range candidates {
		if candidate.IsRunning && candidate.DebugPort > 0 {
			if verbose {
				log.Printf("Selected running browser: %s (debug port: %d)", candidate.Name, candidate.DebugPort)
			}
			return &candidate
		}
	}

	// Then prefer running browsers (but only if they have CDP support)
	// Skip running browsers without debug ports - they can't be controlled via CDP
	for _, candidate := range candidates {
		if candidate.IsRunning && candidate.DebugPort > 0 {
			if verbose {
				log.Printf("Selected running browser: %s (debug port: %d)", candidate.Name, candidate.DebugPort)
			}
			return &candidate
		}
	}

	// Browser preference order (non-running)
	preferenceOrder := []string{
		"Brave",
		"Chrome Canary",
		"Chrome Beta",
		"Chrome Dev",
		"Chrome",
		"Chromium",
		"Edge",
		"Vivaldi",
		"Opera",
	}

	for _, preferred := range preferenceOrder {
		for _, candidate := range candidates {
			if candidate.Name == preferred {
				if verbose {
					log.Printf("Selected browser: %s at %s", candidate.Name, candidate.Path)
				}
				return &candidate
			}
		}
	}

	// Fallback to first available
	if verbose {
		log.Printf("Selected fallback browser: %s at %s", candidates[0].Name, candidates[0].Path)
	}
	return &candidates[0]
}

// setCustomHeaders enables network interception and sets custom HTTP headers
// This must be called before any navigation
func setCustomHeaders(ctx context.Context, headers map[string]interface{}) error {
	if len(headers) == 0 {
		return nil // No headers to set
	}

	// Enable network events
	if err := chromedp.Run(ctx, network.Enable()); err != nil {
		return err
	}

	// Convert interface{} map to network.Headers (which is map[string]string)
	headersMap := make(network.Headers)
	for k, v := range headers {
		headersMap[k] = fmt.Sprintf("%v", v)
	}

	// Set extra HTTP headers
	if err := chromedp.Run(ctx, network.SetExtraHTTPHeaders(headersMap)); err != nil {
		return err
	}

	return nil
}

// parseHeaders converts header strings to a map for network.SetExtraHTTPHeaders
// Each header string should be in format "Name: value"
func parseHeaders(headersList stringSlice) map[string]interface{} {
	headers := make(map[string]interface{})
	for _, headerStr := range headersList {
		parts := strings.SplitN(headerStr, ":", 2)
		if len(parts) == 2 {
			name := strings.TrimSpace(parts[0])
			value := strings.TrimSpace(parts[1])
			if name != "" {
				headers[name] = value
			}
		}
	}
	return headers
}

// applyExtraHeaders applies custom HTTP headers to a browser context
func applyExtraHeaders(ctx context.Context, extraHeaders map[string]interface{}, verbose bool) error {
	if len(extraHeaders) == 0 {
		return nil
	}

	if verbose {
		log.Printf("Applying %d extra HTTP headers", len(extraHeaders))
	}

	return chromedp.Run(ctx, network.SetExtraHTTPHeaders(extraHeaders))
}

// High-level commands that use the Page API
var enhancedCommands = map[string]func(*browser.Page, []string) error{
	"wait": func(p *browser.Page, args []string) error {
		if len(args) < 1 {
			return errors.New("selector required")
		}
		return p.WaitForSelector(args[0])
	},
	"waitfor": func(p *browser.Page, args []string) error {
		if len(args) < 1 {
			return errors.New("milliseconds required")
		}
		var ms int
		fmt.Sscanf(args[0], "%d", &ms)
		time.Sleep(time.Duration(ms) * time.Millisecond)
		return nil
	},
	"text": func(p *browser.Page, args []string) error {
		if len(args) < 1 {
			return errors.New("selector required")
		}
		text, err := p.GetText(args[0])
		if err != nil {
			return err
		}
		fmt.Println("Text:", text)
		return nil
	},
	"hover": func(p *browser.Page, args []string) error {
		if len(args) < 1 {
			return errors.New("selector required")
		}
		return p.Hover(args[0])
	},
	"fill": func(p *browser.Page, args []string) error {
		if len(args) < 2 {
			return errors.New("selector and text required")
		}
		return p.Type(args[0], args[1])
	},
	"clear": func(p *browser.Page, args []string) error {
		if len(args) < 1 {
			return errors.New("selector required")
		}
		el, err := p.QuerySelector(args[0])
		if err != nil {
			return err
		}
		if el == nil {
			return errors.New("element not found")
		}
		return el.Clear()
	},
	"press": func(p *browser.Page, args []string) error {
		if len(args) < 1 {
			return errors.New("key required")
		}
		return p.Press(args[0])
	},
	"select": func(p *browser.Page, args []string) error {
		if len(args) < 2 {
			return errors.New("selector and value required")
		}
		return p.SelectOption(args[0], args[1])
	},
	"visible": func(p *browser.Page, args []string) error {
		if len(args) < 1 {
			return errors.New("selector required")
		}
		visible, err := p.ElementVisible(args[0])
		if err != nil {
			return err
		}
		fmt.Printf("Visible: %v\n", visible)
		return nil
	},
	"count": func(p *browser.Page, args []string) error {
		if len(args) < 1 {
			return errors.New("selector required")
		}
		elements, err := p.QuerySelectorAll(args[0])
		if err != nil {
			return err
		}
		fmt.Printf("Count: %d\n", len(elements))
		return nil
	},
	"attr": func(p *browser.Page, args []string) error {
		if len(args) < 2 {
			return errors.New("selector and attribute name required")
		}
		value, err := p.GetAttribute(args[0], args[1])
		if err != nil {
			return err
		}
		fmt.Printf("Attribute %s: %s\n", args[1], value)
		return nil
	},
}

func main() {
	// Handle 'cdp script' subcommand before flag parsing
	if len(os.Args) > 1 && os.Args[1] == "script" {
		cmd := newScriptCmd()
		if err := cmd.run(os.Args[2:]); err != nil {
			exitWithError(ExitGeneralError, ErrorTypeGeneral, "%v", err)
		}
		return
	}

	var (
		url          string
		headless     bool
		debugPort    int
		timeout      int
		verbose      bool
		remoteHost   string
		remotePort   int
		remoteTab    string
		listTabs     bool
		listBrowsers bool
		chromePath   string
		autoDiscover bool

		// New features
		jsScripts   stringSlice // Support multiple --js flags
		tabID       string
		harFile     string
		harMode     string // HAR capture mode: simple, enhanced (default: enhanced)
		harlStream  bool   // Stream HAR entries as NDJSON
		harlFile    string // File to stream NDJSON to
		interactive bool
		background  bool
		command     string
		enhanced    bool

		// Profile management features
		useProfile      string
		cookieDomains   string
		listProfiles    bool
		connectExisting bool

		// URL monitoring features
		waitForURLChange  bool
		monitorURLPattern string

		// CSS selector extraction features
		extractSelector string
		extractMode     string

		// HTTP headers flag
		headers stringSlice

		// Window control features
		shell          bool
		windowPosition string
		windowSize     string
		newWindow      bool
		proxy          string
		chromeFlags    string
	)

	flag.StringVar(&url, "url", "about:blank", "URL to navigate to on start")
	flag.BoolVar(&headless, "headless", false, "Run Chrome in headless mode")
	flag.IntVar(&debugPort, "debug-port", 9222, "Connect to Chrome on specific port (0 for auto)")
	flag.IntVar(&timeout, "timeout", 60, "Timeout in seconds (0 for no timeout)")
	flag.BoolVar(&verbose, "verbose", false, "Enable verbose logging")
	flag.StringVar(&remoteHost, "remote-host", "", "Connect to remote Chrome at this host")
	flag.IntVar(&remotePort, "remote-port", 9222, "Remote Chrome debugging port")
	flag.StringVar(&remoteTab, "remote-tab", "", "Connect to specific tab ID or URL")
	flag.BoolVar(&listTabs, "list-tabs", false, "List available tabs on remote Chrome")
	flag.BoolVar(&listBrowsers, "list-browsers", false, "List all discovered browsers and exit")
	flag.StringVar(&chromePath, "chrome-path", "", "Path to specific Chrome executable")
	flag.BoolVar(&autoDiscover, "auto-discover", true, "Automatically discover and prefer running browsers")

	// New flags
	flag.Var(&jsScripts, "js", "JavaScript code to execute in console (can be used multiple times)")
	flag.StringVar(&tabID, "tab", "", "Target specific tab ID")
	flag.StringVar(&proxy, "proxy", "", "Proxy server URL")
	flag.StringVar(&chromeFlags, "chrome-flags", "", "Additional Chrome flags (space-separated)")
	flag.StringVar(&harFile, "har", "", "Save HAR file to this path")
	flag.StringVar(&harMode, "har-mode", "enhanced", "HAR capture mode: enhanced (complete headers/bodies/POST data) or simple (fast, basic)")
	flag.BoolVar(&harlStream, "harl", false, "Stream HAR entries as NDJSON")
	flag.StringVar(&harlFile, "harl-file", "output.har.jsonl", "File to stream NDJSON to (use '-' for stdout)")
	flag.BoolVar(&interactive, "interactive", false, "Keep browser open for interaction")
	flag.BoolVar(&background, "background", false, "Launch browser in background without focusing window")
	flag.StringVar(&command, "command", "", "Execute a single CDP command")
	flag.BoolVar(&enhanced, "enhanced", false, "Use enhanced command mode with better commands")

	// Window control flags
	flag.BoolVar(&shell, "shell", false, "Start in interactive shell mode (auto if no --url or --js)")
	flag.StringVar(&windowPosition, "window-position", "", "Set window position as 'x,y' (e.g., '100,100')")
	flag.StringVar(&windowSize, "window-size", "", "Set window size as 'width,height' (e.g., '1920,1080')")
	flag.BoolVar(&newWindow, "new-window", false, "Force open in new window (vs reusing existing)")

	// Profile management flags
	var profileDir string
	var outputFormat string
	flag.StringVar(&useProfile, "use-profile", "", "Use Chrome profile with cookies and session data")
	flag.StringVar(&profileDir, "profile-dir", "", "Custom profile directory (overrides default locations)")
	flag.StringVar(&cookieDomains, "cookie-domains", "", "Comma-separated list of domains to include cookies from")
	flag.BoolVar(&listProfiles, "list-profiles", false, "List available Chrome profiles and exit")
	flag.BoolVar(&connectExisting, "connect-existing", false, "Prefer connecting to existing Chrome sessions")

	// Output formatting flags
	flag.StringVar(&outputFormat, "format", "text", "Output format: text, json, or tsv (also controls error format)")

	// URL monitoring flags
	flag.BoolVar(&waitForURLChange, "wait-for-url-change", false, "Wait for URL to change and output the new URL")
	flag.StringVar(&monitorURLPattern, "monitor-url-pattern", "", "Monitor for URLs matching this pattern (regex)")

	// CSS selector extraction flags
	flag.StringVar(&extractSelector, "extract", "", "Extract content using CSS selector (e.g., 'p', 'h1', '.class', '#id')")
	flag.StringVar(&extractMode, "extract-mode", "text", "Extraction mode: text, html, attr:name, count (default: text)")

	// Custom HTTP headers flags
	flag.Var(&headers, "H", "Custom HTTP header (can be used multiple times, e.g., -H 'Authorization: Bearer token')")
	flag.Var(&headers, "headers", "Custom HTTP header (long form, can be used multiple times)")

	flag.Parse()

	// Validate har-mode flag
	if harMode != "simple" && harMode != "enhanced" {
		exitWithError(ExitUsageError, ErrorTypeUsage, "Invalid --har-mode value: %s (must be 'simple' or 'enhanced')", harMode)
	}

	// Parse custom headers from flag values
	customHeaders := parseHeaders(headers)

	// Set global error format based on output format
	// Supports: text, json, or tsv (json format is machine-parseable)
	if outputFormat == "json" {
		errorFormat = "json"
	} else {
		errorFormat = "text"
	}

	// Handle stdin for --js flag (read JavaScript code from stdin)
	if len(jsScripts) > 0 && jsScripts[len(jsScripts)-1] == "-" {
		scanner := bufio.NewScanner(os.Stdin)
		var scriptLines []string
		for scanner.Scan() {
			scriptLines = append(scriptLines, scanner.Text())
		}
		if err := scanner.Err(); err != nil {
			exitWithError(ExitGeneralError, ErrorTypeGeneral, "Failed to read JavaScript from stdin: %v", err)
		}
		script := strings.Join(scriptLines, "\n")
		if script == "" {
			exitWithError(ExitUsageError, ErrorTypeUsage, "No JavaScript code provided via stdin")
		}
		// Replace the "-" placeholder with the actual script
		jsScripts[len(jsScripts)-1] = script
	}

	// Handle profile listing
	if listProfiles {
		pm, err := chromeprofiles.NewProfileManager(
			chromeprofiles.WithVerbose(verbose),
		)
		if err != nil {
			exitWithError(ExitGeneralError, ErrorTypeProfile, "Failed to create profile manager: %v", err)
		}

		profiles, err := pm.ListProfiles()
		if err != nil {
			exitWithError(ExitGeneralError, ErrorTypeProfile, "Failed to list profiles: %v", err)
		}

		// Handle different output formats
		if outputFormat == "json" {
			jsonData, err := json.MarshalIndent(profiles, "", "  ")
			if err != nil {
				exitWithError(ExitGeneralError, ErrorTypeGeneral, "Failed to marshal profiles to JSON: %v", err)
			}
			fmt.Println(string(jsonData))
		} else if outputFormat == "tsv" {
			// TSV format: just one profile name per line
			for _, profile := range profiles {
				fmt.Println(profile)
			}
		} else {
			// Default text format
			fmt.Println("Available Chrome profiles:")
			fmt.Println("==========================")
			for i, profile := range profiles {
				fmt.Printf("[%d] %s\n", i+1, profile)
			}

			if len(profiles) == 0 {
				fmt.Println("No Chrome profiles found.")
				fmt.Println("Suggestion: Create a Chrome profile first by opening Chrome and going to Settings > People")
			} else {
				fmt.Printf("\nUse with: cdp -use-profile \"%s\" -js \"document.title\"\n", profiles[0])
			}
		}
		return
	}

	// Handle enhanced command mode
	if enhanced || command != "" {
		handleEnhancedMode(command, enhanced, verbose, chromePath)
		return
	}

	// Handle browser discovery and listing
	if listBrowsers {
		candidates, err := discoverBrowsers(verbose)
		if err != nil {
			exitWithError(ExitBrowserError, ErrorTypeBrowser, "Failed to discover browsers: %v", err)
		}

		// Handle different output formats
		if outputFormat == "json" {
			jsonData, err := json.MarshalIndent(candidates, "", "  ")
			if err != nil {
				exitWithError(ExitGeneralError, ErrorTypeGeneral, "Failed to marshal browsers to JSON: %v", err)
			}
			fmt.Println(string(jsonData))
		} else {
			// Default text format: one line per browser, tab-separated fields (name, path, version, status)
			// No decorative elements - pipe-friendly for Unix tools
			for _, candidate := range candidates {
				status := "Installed"
				if candidate.IsRunning {
					status = "Running"
					if candidate.DebugPort > 0 {
						status += fmt.Sprintf(" (port %d)", candidate.DebugPort)
					}
				}
				fmt.Printf("%s\t%s\t%s\t%s\n", candidate.Name, candidate.Path, candidate.Version, status)
			}
		}
		return
	}

	// Check for existing Chrome instances with debug ports first
	// Skip this if headless mode is explicitly requested (want a new instance)
	if remoteHost == "" && !headless && (connectExisting || useProfile == "") {
		debugPorts := []int{9222, 9223, 9224, 9225}
		for _, port := range debugPorts {
			if checkRunningChrome(port) {
				remoteHost = "localhost"
				remotePort = port
				if verbose {
					log.Printf("Found running Chrome on port %d, connecting...", port)
				}
				break
			}
		}
	}

	// Auto-discover browser if not explicitly specified and no running Chrome found
	var selectedBrowser *BrowserCandidate
	if autoDiscover && chromePath == "" && remoteHost == "" {
		candidates, err := discoverBrowsers(verbose)
		if err != nil && verbose {
			log.Printf("Warning: browser discovery failed: %v", err)
		}

		if len(candidates) > 0 {
			selectedBrowser = selectBestBrowser(candidates, verbose)

			// If we found a running browser with debug port, connect to it instead
			if selectedBrowser.IsRunning && selectedBrowser.DebugPort > 0 {
				remoteHost = "localhost"
				remotePort = selectedBrowser.DebugPort
				if verbose {
					log.Printf("Auto-connecting to running browser: %s (port %d)",
						selectedBrowser.Name, selectedBrowser.DebugPort)
				}
			} else if selectedBrowser.Path != "" {
				chromePath = selectedBrowser.Path
				if verbose {
					log.Printf("Auto-selected browser: %s at %s",
						selectedBrowser.Name, selectedBrowser.Path)
				}
			}
		}
	}

	// Handle --list-tabs separately
	if listTabs {
		// Check for running Chrome instances first
		debugPorts := []int{9222, 9223, 9224, 9225}
		for _, port := range debugPorts {
			if checkRunningChrome(port) {
				tabs, err := getChromeTabs(port)
				if err != nil {
					log.Printf("Failed to get tabs on port %d: %v", port, err)
					continue
				}

				fmt.Printf("Available tabs on port %d:\n", port)
				for i, tab := range tabs {
					fmt.Printf("[%d] %s - %s\n", i, tab.Title, tab.URL)
					fmt.Printf("    ID: %s\n", tab.ID)
				}
				return
			}
		}

		// Fallback to remote host if specified
		if remoteHost != "" {
			tabs, err := browser.ListTabs(remoteHost, remotePort)
			if err != nil {
				exitWithError(ExitBrowserError, ErrorTypeBrowser, "Failed to list tabs: %v", err)
			}

			fmt.Printf("Available tabs on %s:%d:\n\n", remoteHost, remotePort)
			for i, tab := range tabs {
				fmt.Printf("[%d] %s\n", i, tab.Title)
				fmt.Printf("    URL: %s\n", tab.URL)
				fmt.Printf("    Type: %s\n", tab.Type)
				fmt.Printf("    ID: %s\n\n", tab.ID)
			}
			return
		}

		exitWithError(ExitBrowserError, ErrorTypeBrowser, "No running Chrome found with debug port enabled")
	}

	// Set up context with timeout
	// For shell mode, use background context (no timeout) for long-running sessions
	// For non-interactive mode, use timeout context
	var ctx context.Context
	var cancel context.CancelFunc

	// Determine if we are in a mode that requires long-running session
	isImplicitShell := len(jsScripts) == 0 && harFile == "" && !harlStream && command == "" && !listTabs && !listBrowsers && !listProfiles

	if shell || isImplicitShell || timeout == 0 {
		// Shell mode, or explicit no timeout - no global timeout
		ctx, cancel = context.WithCancel(context.Background())
	} else {
		// Non-interactive mode - use timeout
		ctx, cancel = context.WithTimeout(context.Background(), time.Duration(timeout)*time.Second)
	}
	defer cancel()

	// Handle Ctrl+C
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigChan
		log.Println("Signal received, shutting down...")
		cancel()
	}()

	var browserCtx context.Context
	var browserCancel context.CancelFunc
	var enhancedBrowser *browser.Browser
	var enhancedPage *browser.Page
	var enhancedRecorder *harrecorder.Recorder
	var recorder *NetworkRecorder

	// Validate extract flag compatibility
	if extractSelector != "" && len(jsScripts) > 0 {
		exitWithError(ExitUsageError, ErrorTypeUsage, "Cannot use both --extract and --js flags together")
	}

	// Use enhanced browser API when connecting to remote Chrome
	if remoteHost != "" {
		// Handle direct tab connection for specific operations
		if len(jsScripts) > 0 || tabID != "" || harFile != "" || extractSelector != "" {
			// Get available tabs
			_, err := getChromeTabs(remotePort)
			if err != nil {
				exitWithError(ExitBrowserError, ErrorTypeBrowser, "Failed to get tabs: %v", err)
			}

			// Find target tab
			var targetTabID string
			if tabID != "" {
				targetTabID = tabID
			}

			// Connect to specific tab
			var remoteURL string
			if targetTabID != "" {
				remoteURL = fmt.Sprintf("ws://localhost:%d/devtools/page/%s", remotePort, targetTabID)
			} else {
				remoteURL = fmt.Sprintf("ws://localhost:%d", remotePort)
			}

			allocCtx, allocCancel := chromedp.NewRemoteAllocator(ctx, remoteURL)
			defer allocCancel()

			// Use the existing target instead of creating a new one
			var opts []chromedp.ContextOption
			if verbose {
				opts = append(opts, chromedp.WithLogf(log.Printf))
			}

			if targetTabID != "" {
				// Connect to browser first, then attach to existing target
				allocCtx, allocCancel = chromedp.NewRemoteAllocator(ctx, fmt.Sprintf("ws://localhost:%d", remotePort))
				opts = append(opts, chromedp.WithTargetID(target.ID(targetTabID)))
				// Use existing target without managing its lifecycle
				opts = append(opts, chromedp.WithBrowserOption(
					chromedp.WithBrowserDebugf(func(s string, i ...interface{}) {
						if verbose {
							log.Printf("Browser debug: "+s, i...)
						}
					}),
				))
			}

			browserCtx, browserCancel = chromedp.NewContext(allocCtx, opts...)

			// Set up HAR recording if requested
			var harlWriter *os.File
			if harlStream {
				if harlFile == "-" {
					harlWriter = os.Stdout
				} else {
					f, err := os.OpenFile(harlFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
					if err != nil {
						exitWithError(ExitGeneralError, ErrorTypeGeneral, "Failed to open HARL output file: %v", err)
					}
					harlWriter = f
					defer harlWriter.Close()
				}
			}

			if harFile != "" || harlStream {
				if harMode == "enhanced" {
					// Use enhanced recorder with full capture
					var err error
					enhancedRecorder, err = harrecorder.New(
						harrecorder.WithVerbose(verbose),
						harrecorder.WithStreaming(harlStream),
					)
					if err != nil {
						exitWithError(ExitGeneralError, ErrorTypeGeneral, "Failed to create enhanced recorder: %v", err)
					}
				} else {
					recorder = &NetworkRecorder{}
				}

				// Enable network monitoring
				if err := chromedp.Run(browserCtx, network.Enable()); err != nil {
					exitWithError(ExitGeneralError, ErrorTypeNetwork, "Failed to enable network monitoring: %v", err)
				}

				// Set up network event listeners based on mode
				if harMode == "enhanced" {
					chromedp.ListenTarget(browserCtx, enhancedRecorder.HandleNetworkEvent(browserCtx))
					if harFile != "" {
						fmt.Printf("Recording network traffic to: %s (enhanced mode)\n", harFile)
					}
				} else {
					chromedp.ListenTarget(browserCtx, func(ev interface{}) {
						switch ev := ev.(type) {
						case *network.EventResponseReceived:
							if verbose {
								log.Printf("Response received: %s", ev.Response.URL)
							}

							// Create basic HAR entry
							entry := HAREntry{
								StartedDateTime: time.Now().Format(time.RFC3339),
								Request: map[string]interface{}{
									"method":  "GET", // Simplified
									"url":     ev.Response.URL,
									"headers": []interface{}{},
								},
								Response: map[string]interface{}{
									"status":     ev.Response.Status,
									"statusText": ev.Response.StatusText,
									"headers":    []interface{}{},
									"content": map[string]interface{}{
										"size":     0,
										"mimeType": ev.Response.MimeType,
									},
								},
								Time: 0, // Simplified
							}

							recorder.AddEntry(entry)

							// Stream entry as NDJSON if --harl is enabled
							if harlStream {
								if jsonBytes, err := json.Marshal(entry); err == nil {
									fmt.Fprintln(harlWriter, string(jsonBytes))
								}
							}
						}
					})

					if harFile != "" {
						fmt.Printf("Recording network traffic to: %s\n", harFile)
					}
				}

				if harlStream {
					if verbose {
						if harlFile == "-" {
							log.Println("Streaming HAR entries as NDJSON to stdout")
						} else {
							log.Printf("Streaming HAR entries as NDJSON to %s", harlFile)
						}
					}
				}
			}

			// Handle CSS selector extraction if provided
			if extractSelector != "" {
				// Set custom headers before navigation
				if err := setCustomHeaders(browserCtx, customHeaders); err != nil {
					exitWithError(ExitGeneralError, ErrorTypeNetwork, "Failed to set custom headers: %v", err)
				}

				extractionCode := buildExtractionScript(extractSelector, extractMode)
				var result *runtime.RemoteObject
				if err := chromedp.Run(browserCtx,
					chromedp.Navigate(url),
					chromedp.Evaluate(extractionCode, &result),
				); err != nil {
					exitWithError(ExitGeneralError, ErrorTypeGeneral, "Failed to extract content: %v", err)
				}

				if result != nil && result.Value != nil {
					resultStr := string(result.Value)
					if outputFormat == "json" || (strings.HasPrefix(resultStr, "[") || strings.HasPrefix(resultStr, "{")) {
						var jsonData interface{}
						if err := json.Unmarshal([]byte(resultStr), &jsonData); err == nil {
							prettyJSON, _ := json.MarshalIndent(jsonData, "", "  ")
							fmt.Println(string(prettyJSON))
						} else {
							fmt.Println(resultStr)
						}
					} else {
						fmt.Println(resultStr)
					}
				}
				return
			}

			// Execute JavaScript if provided
			if len(jsScripts) > 0 {
				var results []interface{}
				// Prepare results slice with space for all scripts
				results = make([]interface{}, len(jsScripts))

				// Execute scripts sequentially in the same browser context
				for scriptIdx, jsCode := range jsScripts {
					var result *runtime.RemoteObject
					if err := chromedp.Run(browserCtx, chromedp.Evaluate(jsCode, &result)); err != nil {
						// Provide improved error messages with context, including script index
						errorMsg := fmt.Sprintf("script %d: ", scriptIdx+1)
						if strings.Contains(err.Error(), "SyntaxError") {
							if strings.Contains(jsCode, ":contains(") {
								exitWithError(ExitGeneralError, ErrorTypeJavaScript, "%sJavaScript Syntax Error: :contains() is not valid CSS. Use Array.from(document.querySelectorAll('button')).filter(btn => btn.textContent.includes('text')) instead.\nOriginal error: %v", errorMsg, err)
							} else {
								exitWithError(ExitGeneralError, ErrorTypeJavaScript, "%sJavaScript Syntax Error in code '%s': %v", errorMsg, jsCode, err)
							}
						} else if strings.Contains(err.Error(), "not a valid selector") {
							exitWithError(ExitGeneralError, ErrorTypeJavaScript, "%sInvalid CSS Selector: Use standard CSS selectors or JavaScript filtering methods.\nOriginal error: %v", errorMsg, err)
						} else {
							exitWithError(ExitGeneralError, ErrorTypeJavaScript, "%sFailed to execute JavaScript '%s': %v", errorMsg, jsCode, err)
						}
					}

					// Extract result value
					var resultValue interface{}
					if result != nil && result.Value != nil {
						// Try to parse JSON values for proper type preservation
						var val interface{}
						if err := json.Unmarshal(result.Value, &val); err == nil {
							resultValue = val
						} else {
							resultValue = string(result.Value)
						}
					} else {
						resultValue = nil
					}
					results[scriptIdx] = resultValue
				}

				// Output results based on format
				if outputFormat == "json" {
					jsonData, err := json.MarshalIndent(results, "", "  ")
					if err != nil {
						exitWithError(ExitGeneralError, ErrorTypeGeneral, "Failed to marshal script results to JSON: %v", err)
					}
					fmt.Println(string(jsonData))
				} else {
					// Default text format: one result per line
					fmt.Printf("✓ Executed %d JavaScript script(s) in Chrome on port %d\n", len(jsScripts), remotePort)
					if targetTabID != "" {
						fmt.Printf("Target tab ID: %s\n", targetTabID)
					}
					for scriptIdx, result := range results {
						if scriptIdx > 0 {
							fmt.Println()
						}
						fmt.Printf("Script %d result: %v\n", scriptIdx+1, result)
					}
				}

				// Handle URL monitoring if requested
				if waitForURLChange || monitorURLPattern != "" {
					if err := monitorURLChanges(browserCtx, monitorURLPattern, verbose); err != nil {
						log.Printf("URL monitoring failed: %v", err)
					}
				}

				// Handle URL monitoring if requested
				if waitForURLChange || monitorURLPattern != "" {
					if err := monitorURLChanges(browserCtx, monitorURLPattern, verbose); err != nil {
						log.Printf("URL monitoring failed: %v", err)
					}
				}

				// Save HAR file if recording and exit
				if recorder != nil && harFile != "" {
					if err := recorder.SaveHAR(harFile); err != nil {
						log.Printf("Failed to save HAR file: %v", err)
					} else {
						fmt.Printf("HAR file saved to: %s\n", harFile)
						fmt.Printf("Recorded %d network requests\n", len(recorder.GetEntries()))
					}
				}

				// Clean up gently without affecting the target
				if targetTabID != "" {
					// Don't cancel browser context for existing tabs
					return
				}

				return
			}

			// Navigate to URL if specified
			if url != "about:blank" {
				// Set custom headers before navigation
				if err := setCustomHeaders(browserCtx, customHeaders); err != nil && verbose {
					log.Printf("Warning: Failed to set custom headers: %v", err)
				}

				if err := chromedp.Run(browserCtx, chromedp.Navigate(url)); err != nil {
					if verbose {
						log.Printf("Failed to navigate to %s: %v", url, err)
					}
				}
			}

			// If HAR capture without JS, wait for user interaction
			if harFile != "" || harlStream {
				if verbose {
					fmt.Printf("Connected to Chrome on port %d\n", remotePort)
					if targetTabID != "" {
						fmt.Printf("Target tab ID: %s\n", targetTabID)
					}
					fmt.Println("Press Ctrl+C to stop recording...")
				}

				// Wait for signal or timeout
				select {
				case <-ctx.Done():
					if verbose {
						log.Println("Context cancelled...")
					}
				case <-sigChan:
					if verbose {
						log.Println("Signal received...")
					}
				}

				// Save HAR file if specified
				if harFile != "" && recorder != nil {
					if err := recorder.SaveHAR(harFile); err != nil {
						log.Printf("Failed to save HAR file: %v", err)
					} else {
						fmt.Printf("HAR file saved to: %s\n", harFile)
						fmt.Printf("Recorded %d network requests\n", len(recorder.GetEntries()))
					}
				}

				return
			}
		} else {
			// Use enhanced browser API for interactive mode
			// Create profile manager
			pm, err := chromeprofiles.NewProfileManager(
				chromeprofiles.WithVerbose(verbose),
			)
			if err != nil {
				exitWithError(ExitGeneralError, ErrorTypeGeneral, "%v", err)
			}

			// Set up browser options
			browserOpts := []browser.Option{
				browser.WithHeadless(headless),
				browser.WithVerbose(verbose),
				browser.WithTimeout(timeout),
			}

			if proxy != "" {
				browserOpts = append(browserOpts, browser.WithProxy(proxy))
			}
			if chromeFlags != "" {
				browserOpts = append(browserOpts, browser.WithChromeFlags(strings.Split(chromeFlags, " ")))
			}

			if remoteHost != "" {
				browserOpts = append(browserOpts, browser.WithRemoteChrome(remoteHost, remotePort))
				if remoteTab != "" {
					browserOpts = append(browserOpts, browser.WithRemoteTab(remoteTab))
				}
			}

			if debugPort > 0 {
				browserOpts = append(browserOpts, browser.WithDebugPort(debugPort))
			}

			// Create browser
			enhancedBrowser, err = browser.New(ctx, pm, browserOpts...)
			if err != nil {
				exitWithError(ExitBrowserError, ErrorTypeBrowser, "Failed to create browser: %v", err)
			}
			defer enhancedBrowser.Close()

			// Launch browser
			if err := enhancedBrowser.Launch(ctx); err != nil {
				exitWithError(ExitBrowserError, ErrorTypeBrowser, "Failed to launch browser: %v", err)
			}

			// Get or create page
			if remoteTab != "" {
				// When we connect to a specific remote tab, the browser context
				// is already connected to that tab. Get a page wrapper for it.
				enhancedPage = enhancedBrowser.GetCurrentPage()
			} else {
				pages, err := enhancedBrowser.Pages()
				if err != nil || len(pages) == 0 {
					enhancedPage, err = enhancedBrowser.NewPage()
					if err != nil {
						exitWithError(ExitGeneralError, ErrorTypeGeneral, "Failed to create page: %v", err)
					}
				} else {
					enhancedPage = pages[0]
				}
			}

			// Navigate to initial URL
			if url != "about:blank" && enhancedPage != nil {
				if err := enhancedPage.Navigate(url); err != nil {
					log.Printf("Warning: Failed to navigate to %s: %v", url, err)
				}
			}

			browserCtx = enhancedPage.Context()
			browserCancel = func() {} // browser.Close() will handle cleanup

			if remoteHost != "" {
				fmt.Printf("Connected to remote Chrome at %s:%d\n", remoteHost, remotePort)
				if remoteTab != "" {
					fmt.Printf("Connected to tab: %s\n", remoteTab)
				}
			}

			if verbose {
				fmt.Println("Using enhanced browser API for remote Chrome connection")
			}
		}
	} else {
		// Local Chrome instance with optional profile support
		var profileManager chromeprofiles.ProfileManager
		var err error

		// Set up profile management if requested
		if useProfile != "" {
			profileManager, err = chromeprofiles.NewProfileManager(
				chromeprofiles.WithVerbose(verbose),
			)
			if err != nil {
				exitWithError(ExitGeneralError, ErrorTypeProfile, "Failed to create profile manager: %v", err)
			}

			if err := profileManager.SetupWorkdir(); err != nil {
				exitWithError(ExitGeneralError, ErrorTypeProfile, "Failed to setup profile working directory: %v", err)
			}
			defer profileManager.Cleanup()

			// Parse cookie domains
			var cookieDomainsSlice []string
			if cookieDomains != "" {
				cookieDomainsSlice = splitAndTrim(cookieDomains, ",")
			}

			// Check for Brave session isolation needs
			sessionDetector := browser.NewSessionDetector(verbose)
			needsIsolation := sessionDetector.NeedsBraveSessionIsolation(ctx, chromePath, true)

			if needsIsolation {
				// Display warning about Brave session reuse
				fmt.Println(sessionDetector.ImportantWarning())

				// Use Brave session isolation instead of standard profile copy
				if err := profileManager.BraveSessionIsolation(useProfile, cookieDomainsSlice); err != nil {
					exitWithError(ExitGeneralError, ErrorTypeProfile, "Failed to create Brave isolated profile '%s': %v", useProfile, err)
				}

				// Wait for DevTools to be available if debug port is specified
				if debugPort > 0 {
					waitCtx, waitCancel := context.WithTimeout(ctx, 10*time.Second)
					if err := sessionDetector.WaitForDevTools(waitCtx, debugPort, 5*time.Second); err != nil {
						if verbose {
							log.Printf("Warning: DevTools verification timed out (non-fatal): %v", err)
						}
					}
					waitCancel()
				}

				if verbose {
					log.Printf("Brave session isolation created for profile '%s'", useProfile)
				}
			} else {
				// Standard profile copy
				if err := profileManager.CopyProfile(useProfile, cookieDomainsSlice); err != nil {
					exitWithError(ExitGeneralError, ErrorTypeProfile, "Failed to copy profile '%s': %v", useProfile, err)
				}
			}

			if verbose {
				if len(cookieDomainsSlice) > 0 {
					log.Printf("Using profile '%s' with cookies filtered for domains: %v", useProfile, cookieDomainsSlice)
				} else {
					log.Printf("Using profile '%s' with all cookies", useProfile)
				}
			}
		}

		opts := []chromedp.ExecAllocatorOption{
			chromedp.NoFirstRun,
			chromedp.NoDefaultBrowserCheck,

			// Add stability flags
			chromedp.Flag("disable-background-networking", true),
			chromedp.Flag("disable-background-timer-throttling", true),
			chromedp.Flag("disable-popup-blocking", true),
			chromedp.Flag("disable-sync", true),
		}

		// Add background launch flags to prevent window focusing
		if background || interactive {
			opts = append(opts,
				chromedp.Flag("no-startup-window", true),
				chromedp.Flag("silent-launch", true),
			)
			if verbose {
				log.Println("Launching browser in background mode")
			}
		}

		// Add profile directory if using a profile
		if profileManager != nil {
			opts = append(opts, chromedp.UserDataDir(profileManager.WorkDir()))
			if verbose {
				log.Printf("Using profile data from: %s", profileManager.WorkDir())
			}
		}

		if headless {
			opts = append(opts, chromedp.Headless)
			if verbose {
				log.Println("Running Chrome in headless mode")
			}
		}

		if debugPort > 0 {
			opts = append(opts, chromedp.Flag("remote-debugging-port", fmt.Sprintf("%d", debugPort)))
		}

		// Add Chrome path if specified or discovered
		if chromePath != "" {
			opts = append(opts, chromedp.ExecPath(chromePath))
			if verbose {
				log.Printf("Using Chrome at: %s", chromePath)
			}
		}

		// Add window positioning if specified
		if windowPosition != "" {
			opts = append(opts, chromedp.Flag("window-position", windowPosition))
			if verbose {
				log.Printf("Setting window position: %s", windowPosition)
			}
		}

		// Add window size if specified
		if windowSize != "" {
			opts = append(opts, chromedp.Flag("window-size", windowSize))
			if verbose {
				log.Printf("Setting window size: %s", windowSize)
			}
		}

		// Force new window if specified
		if newWindow {
			opts = append(opts, chromedp.Flag("new-window", true))
			if verbose {
				log.Println("Forcing new window")
			}
		}

		// Create Chrome allocator
		allocCtx, allocCancel := chromedp.NewExecAllocator(ctx, opts...)
		defer allocCancel()

		// Create Chrome browser context
		if verbose {
			browserCtx, browserCancel = chromedp.NewContext(allocCtx, chromedp.WithLogf(log.Printf))
		} else {
			browserCtx, browserCancel = chromedp.NewContext(allocCtx)
		}

		// Initialize browser context when using profiles to ensure profile loads before navigation
		if profileManager != nil {
			if verbose {
				log.Println("Initializing browser with profile...")
			}
			// Ensure browser is fully started and profile is loaded before any navigation
			if err := chromedp.Run(browserCtx); err != nil {
				exitWithError(ExitBrowserError, ErrorTypeBrowser, "Failed to initialize browser with profile: %v", err)
			}
			if verbose {
				log.Println("Browser profile initialized successfully")
			}
		}

		// Set up HAR recording if requested (for new Chrome instances)
		var harlWriter *os.File
		if harlStream {
			if harlFile == "-" {
				harlWriter = os.Stdout
			} else {
				f, err := os.OpenFile(harlFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
				if err != nil {
					exitWithError(ExitGeneralError, ErrorTypeGeneral, "Failed to open HARL output file: %v", err)
				}
				harlWriter = f
				defer harlWriter.Close()
			}
		}

		if harFile != "" || harlStream {
			if harMode == "enhanced" {
				// Use enhanced recorder with full capture
				var err error
				enhancedRecorder, err = harrecorder.New(
					harrecorder.WithVerbose(verbose),
					harrecorder.WithStreaming(harlStream),
				)
				if err != nil {
					exitWithError(ExitGeneralError, ErrorTypeGeneral, "Failed to create enhanced recorder: %v", err)
				}
			} else {
				recorder = &NetworkRecorder{}
			}

			// Enable network events BEFORE any navigation
			// This is critical: network.Enable() and listener attachment must happen before
			// any navigation to ensure we capture the initial page request and all network events
			if verbose {
				log.Printf("Enabling network recording and attaching event listeners...")
			}

			if err := chromedp.Run(browserCtx,
				network.Enable(),
				chromedp.ActionFunc(func(ctx context.Context) error {
					// Attach listeners synchronously within the same action
					if harMode == "enhanced" {
						chromedp.ListenTarget(ctx, enhancedRecorder.HandleNetworkEvent(ctx))
					} else {
						chromedp.ListenTarget(ctx, func(ev interface{}) {
							switch ev := ev.(type) {
							case *network.EventResponseReceived:
								if verbose {
									log.Printf("Response received: %s", ev.Response.URL)
								}

								// Create basic HAR entry
								entry := HAREntry{
									StartedDateTime: time.Now().Format(time.RFC3339),
									Request: map[string]interface{}{
										"method":  "GET", // Simplified
										"url":     ev.Response.URL,
										"headers": []interface{}{},
									},
									Response: map[string]interface{}{
										"status":     ev.Response.Status,
										"statusText": ev.Response.StatusText,
										"headers":    []interface{}{},
										"content": map[string]interface{}{
											"size":     0,
											"mimeType": ev.Response.MimeType,
										},
									},
									Time: 0, // Simplified
								}

								recorder.AddEntry(entry)

								// Stream entry as NDJSON if --harl is enabled
								if harlStream {
									if jsonBytes, err := json.Marshal(entry); err == nil {
										fmt.Fprintln(harlWriter, string(jsonBytes))
									}
								}
							}
						})
					}
					if verbose {
						log.Printf("Network event listeners attached successfully")
					}
					return nil
				}),
			); err != nil {
				exitWithError(ExitGeneralError, ErrorTypeNetwork, "Failed to enable network monitoring: %v", err)
			}

			if harFile != "" {
				if harMode == "enhanced" {
					fmt.Printf("Recording network traffic to: %s (enhanced mode)\n", harFile)
				} else {
					fmt.Printf("Recording network traffic to: %s\n", harFile)
				}
			}
			if harlStream {
				if verbose {
					if harlFile == "-" {
						log.Println("Streaming HAR entries as NDJSON to stdout")
					} else {
						log.Printf("Streaming HAR entries as NDJSON to %s", harlFile)
					}
				}
			}
		}

		// Handle CSS selector extraction if provided (for new Chrome instances)
		if extractSelector != "" {
			// Ensure browser is fully started before executing extraction
			if err := chromedp.Run(browserCtx); err != nil {
				exitWithError(ExitBrowserError, ErrorTypeBrowser, "Failed to start browser context: %v", err)
			}

			extractionCode := buildExtractionScript(extractSelector, extractMode)
			var result *runtime.RemoteObject
			if err := chromedp.Run(browserCtx,
				chromedp.Navigate(url),
				chromedp.Evaluate(extractionCode, &result),
			); err != nil {
				exitWithError(ExitGeneralError, ErrorTypeGeneral, "Failed to extract content: %v", err)
			}

			if result != nil && result.Value != nil {
				resultStr := string(result.Value)
				if outputFormat == "json" || (strings.HasPrefix(resultStr, "[") || strings.HasPrefix(resultStr, "{")) {
					var jsonData interface{}
					if err := json.Unmarshal([]byte(resultStr), &jsonData); err == nil {
						prettyJSON, _ := json.MarshalIndent(jsonData, "", "  ")
						fmt.Println(string(prettyJSON))
					} else {
						fmt.Println(resultStr)
					}
				} else {
					fmt.Println(resultStr)
				}
			}
			return
		}

		// Execute JavaScript if provided (for new Chrome instances)
		if len(jsScripts) > 0 {
			// Ensure browser is fully started before executing JavaScript
			if err := chromedp.Run(browserCtx); err != nil {
				exitWithError(ExitBrowserError, ErrorTypeBrowser, "Failed to start browser context: %v", err)
			}

			var results []interface{}
			// Prepare results slice with space for all scripts
			results = make([]interface{}, len(jsScripts))

			// Build tasks for all scripts to execute in same context
			tasks := make([]chromedp.Action, len(jsScripts)+1)
			scriptResults := make([]*runtime.RemoteObject, len(jsScripts))

			// First action: navigate to URL
			tasks[0] = chromedp.Navigate(url)

			// Build evaluate actions for all scripts
			for i, jsCode := range jsScripts {
				// Capture current index to avoid closure issues
				idx := i
				code := jsCode
				tasks[i+1] = chromedp.Evaluate(code, &scriptResults[idx])
			}

			if err := chromedp.Run(browserCtx, tasks...); err != nil {
				// Provide improved error messages with context
				for scriptIdx, jsCode := range jsScripts {
					errorMsg := fmt.Sprintf("script %d: ", scriptIdx+1)
					if strings.Contains(err.Error(), "SyntaxError") {
						if strings.Contains(jsCode, ":contains(") {
							exitWithError(ExitGeneralError, ErrorTypeJavaScript, "%sJavaScript Syntax Error: :contains() is not valid CSS. Use Array.from(document.querySelectorAll('button')).filter(btn => btn.textContent.includes('text')) instead.\nOriginal error: %v", errorMsg, err)
						} else {
							exitWithError(ExitGeneralError, ErrorTypeJavaScript, "%sJavaScript Syntax Error in code '%s': %v", errorMsg, jsCode, err)
						}
					} else if strings.Contains(err.Error(), "not a valid selector") {
						exitWithError(ExitGeneralError, ErrorTypeJavaScript, "%sInvalid CSS Selector: Use standard CSS selectors or JavaScript filtering methods.\nOriginal error: %v", errorMsg, err)
					}
				}
				// If we couldn't identify which script failed, report the first one
				exitWithError(ExitGeneralError, ErrorTypeJavaScript, "Failed to execute JavaScript scripts: %v", err)
			}

			// Extract result values from all scripts
			for i, result := range scriptResults {
				var resultValue interface{}
				if result != nil && result.Value != nil {
					// Try to parse JSON values for proper type preservation
					var val interface{}
					if err := json.Unmarshal(result.Value, &val); err == nil {
						resultValue = val
					} else {
						resultValue = string(result.Value)
					}
				} else {
					resultValue = nil
				}
				results[i] = resultValue
			}

			// Output results based on format
			if outputFormat == "json" {
				jsonData, err := json.MarshalIndent(results, "", "  ")
				if err != nil {
					exitWithError(ExitGeneralError, ErrorTypeGeneral, "Failed to marshal script results to JSON: %v", err)
				}
				fmt.Println(string(jsonData))
			} else {
				// Default text format: one result per line
				fmt.Printf("✓ Executed %d JavaScript script(s) in new Chrome instance\n", len(jsScripts))
				for scriptIdx, result := range results {
					if scriptIdx > 0 {
						fmt.Println()
					}
					fmt.Printf("Script %d result: %v\n", scriptIdx+1, result)
				}
			}

			// Save HAR file if recording and exit
			if harFile != "" {
				if harMode == "enhanced" && enhancedRecorder != nil {
					if err := enhancedRecorder.WriteHAR(harFile); err != nil {
						log.Printf("Failed to save HAR file: %v", err)
					} else {
						fmt.Printf("HAR file saved to: %s\n", harFile)
						// Get entry count from HAR
						har, _ := enhancedRecorder.HAR()
						if har != nil {
							fmt.Printf("Recorded %d network requests\n", len(har.Log.Entries))
						}
					}
				} else if recorder != nil {
					if err := recorder.SaveHAR(harFile); err != nil {
						log.Printf("Failed to save HAR file: %v", err)
					} else {
						fmt.Printf("HAR file saved to: %s\n", harFile)
						fmt.Printf("Recorded %d network requests\n", len(recorder.GetEntries()))
					}
				}
			}

			return
		}

		// If HAR capture without JS and NOT in shell mode, navigate and wait for user interaction
		if (harFile != "" || harlStream) && !shell {
			// CRITICAL: Navigate to URL with network monitoring active
			// This ensures we capture all network events including the initial page request
			if verbose {
				log.Printf("Network monitoring is active, proceeding with navigation...")
			}

			if err := chromedp.Run(browserCtx, chromedp.Navigate(url)); err != nil {
				exitWithError(ExitNavigationError, ErrorTypeNavigation, "Failed to navigate to %s: %v", url, err)
			}

			fmt.Printf("Chrome launched and connected to %s\n", url)
			fmt.Println("Press Ctrl+C to stop recording and save HAR file...")

			// Wait for signal or timeout
			select {
			case <-ctx.Done():
				if verbose {
					log.Println("Context cancelled...")
				}
			case <-sigChan:
				if verbose {
					log.Println("Signal received...")
				}
			}

			// Save HAR file
			if harMode == "enhanced" && enhancedRecorder != nil {
				if err := enhancedRecorder.WriteHAR(harFile); err != nil {
					log.Printf("Failed to save HAR file: %v", err)
				} else {
					fmt.Printf("HAR file saved to: %s\n", harFile)
					har, _ := enhancedRecorder.HAR()
					if har != nil {
						fmt.Printf("Recorded %d network requests\n", len(har.Log.Entries))
					}
				}
			} else if recorder != nil {
				if err := recorder.SaveHAR(harFile); err != nil {
					log.Printf("Failed to save HAR file: %v", err)
				} else {
					fmt.Printf("HAR file saved to: %s\n", harFile)
					fmt.Printf("Recorded %d network requests\n", len(recorder.GetEntries()))
				}
			}

			return
		}

		// Start and connect to browser
		if err := chromedp.Run(browserCtx, chromedp.Navigate(url)); err != nil {
			exitWithError(ExitBrowserError, ErrorTypeBrowser, "Error launching Chrome: %v", err)
		}
	}
	defer browserCancel()

	// Determine if we should run interactive shell mode
	// Enter shell if: --shell flag explicitly set, OR (no JS scripts AND no HAR file)
	// Note: --shell allows combining interactive mode with HAR recording
	shouldRunShell := shell || (len(jsScripts) == 0 && harFile == "" && !harlStream)

	if !shouldRunShell {
		// Direct execution mode already handled above
		return
	}

	// Interactive loop
	fmt.Println("Connected to Chrome. Type commands or 'help' for assistance.")
	fmt.Println("Examples: 'goto https://example.com', 'title', 'screenshot'")
	fmt.Println("Type 'exit' or press Ctrl+C to quit")

	scanner := bufio.NewScanner(os.Stdin)

	// Create a channel to signal when scanner input is ready
	inputChan := make(chan string)
	go func() {
		for scanner.Scan() {
			inputChan <- scanner.Text()
		}
		close(inputChan)
	}()

	for {
		fmt.Print("cdp> ")

		// Wait for either input or context cancellation
		select {
		case <-ctx.Done():
			// Context was canceled (Ctrl+C)
			fmt.Println()
			goto cleanup
		case text, ok := <-inputChan:
			if !ok {
				// Scanner closed (EOF)
				goto cleanup
			}

			line := strings.TrimSpace(text)
			if line == "" {
				continue
			}

			if line == "exit" || line == "quit" {
				goto cleanup
			}

			if line == "help" {
				printHelp()
				continue
			}

			if line == "help aliases" {
				printAliases()
				continue
			}

			// Handle special commands
			parts := strings.SplitN(line, " ", 2)
			cmd := parts[0]

			// Handle sources command
			if cmd == "sources" {
				if err := handleSourcesCommand(browserCtx, parts); err != nil {
					fmt.Printf("Error: %v\n", err)
				}
				continue
			}

			// Handle annotation commands if enhanced recorder is active
			if enhancedRecorder != nil {
				switch cmd {
				case "note":
					if len(parts) < 2 {
						fmt.Println("Usage: note <description>")
						continue
					}
					if verbose {
						fmt.Printf("\033[90m[verbose] Adding note annotation...\033[0m\n")
					}
					if err := enhancedRecorder.AddNote(browserCtx, parts[1]); err != nil {
						fmt.Printf("Error adding note: %v\n", err)
						if verbose {
							fmt.Printf("\033[90m[verbose] Note addition failed: %v\033[0m\n", err)
						}
					} else {
						fmt.Printf("✓ Note added: %s\n", parts[1])
						if verbose {
							fmt.Printf("\033[90m[verbose] Note annotation completed successfully\033[0m\n")
						}
					}
					continue
				case "screenshot":
					description := "Screenshot"
					if len(parts) >= 2 {
						description = parts[1]
					}
					if verbose {
						fmt.Printf("\033[90m[verbose] Capturing screenshot annotation...\033[0m\n")
					}
					if err := enhancedRecorder.AddScreenshot(browserCtx, description); err != nil {
						fmt.Printf("Error capturing screenshot: %v\n", err)
						if verbose {
							fmt.Printf("\033[90m[verbose] Screenshot capture failed: %v\033[0m\n", err)
						}
					} else {
						fmt.Printf("✓ Screenshot captured: %s\n", description)
						if verbose {
							fmt.Printf("\033[90m[verbose] Screenshot annotation completed successfully\033[0m\n")
						}
					}
					continue
				case "dom":
					description := "DOM Snapshot"
					if len(parts) >= 2 {
						description = parts[1]
					}
					if verbose {
						fmt.Printf("\033[90m[verbose] Capturing DOM snapshot annotation...\033[0m\n")
					}
					if err := enhancedRecorder.AddDOMSnapshot(browserCtx, description); err != nil {
						fmt.Printf("Error capturing DOM: %v\n", err)
						if verbose {
							fmt.Printf("\033[90m[verbose] DOM capture failed: %v\033[0m\n", err)
						}
					} else {
						fmt.Printf("✓ DOM snapshot captured: %s\n", description)
						if verbose {
							fmt.Printf("\033[90m[verbose] DOM snapshot annotation completed successfully\033[0m\n")
						}
					}
					continue
				}
			}

			// Process command or alias
			var cmdToRun string

			if alias, ok := aliases[cmd]; ok {
				// It's an alias
				cmdToRun = alias

				// Check if it has parameters
				if strings.Contains(alias, "$1") && len(parts) > 1 {
					cmdToRun = strings.ReplaceAll(cmdToRun, "$1", parts[1])
				}

				fmt.Printf("Alias: %s\n", cmdToRun)
			} else {
				// Raw CDP command
				cmdToRun = line
			}

			// Execute command with enhanced page if available
			if enhancedPage != nil && strings.HasPrefix(cmdToRun, "@") {
				// Execute enhanced command
				if err := executeEnhancedCommand(enhancedPage, strings.TrimPrefix(cmdToRun, "@")); err != nil {
					fmt.Printf("Error: %v\n", err)
				}
			} else {
				// Execute standard CDP command
				if err := executeCommand(browserCtx, cmdToRun); err != nil {
					fmt.Printf("Error: %v\n", err)
				}
			}
		}
	}

cleanup:

	if err := scanner.Err(); err != nil {
		exitWithError(ExitGeneralError, ErrorTypeGeneral, "Error reading input: %v", err)
	}

	// Save HAR file if recording was enabled
	if harFile != "" {
		if harMode == "enhanced" && enhancedRecorder != nil {
			if err := enhancedRecorder.WriteHAR(harFile); err != nil {
				log.Printf("Failed to save HAR file: %v", err)
			} else {
				fmt.Printf("HAR file saved to: %s\n", harFile)
				har, _ := enhancedRecorder.HAR()
				if har != nil {
					fmt.Printf("Recorded %d network requests\n", len(har.Log.Entries))
				}
			}
		} else if recorder != nil {
			if err := recorder.SaveHAR(harFile); err != nil {
				log.Printf("Failed to save HAR file: %v", err)
			} else {
				fmt.Printf("HAR file saved to: %s\n", harFile)
				fmt.Printf("Recorded %d network requests\n", len(recorder.GetEntries()))
			}
		}
	}

	fmt.Println("Exiting...")
}

func executeCommand(ctx context.Context, command string) error {
	// Parse Domain.method {params}
	parts := strings.SplitN(command, " ", 2)
	if len(parts) == 0 {
		return errors.New("empty command")
	}

	method := parts[0]
	if !strings.Contains(method, ".") {
		return errors.New("invalid command format: expected 'Domain.method'")
	}

	// Parse parameters
	var params json.RawMessage
	if len(parts) > 1 {
		paramStr := strings.TrimSpace(parts[1])
		if paramStr == "" || paramStr == "{}" {
			params = json.RawMessage("{}")
		} else {
			// Validate JSON
			var temp map[string]interface{}
			if err := json.Unmarshal([]byte(paramStr), &temp); err != nil {
				return errors.Wrap(err, "invalid JSON parameters")
			}
			params = json.RawMessage(paramStr)
		}
	} else {
		params = json.RawMessage("{}")
	}

	// Special case for Runtime.evaluate since it's very common
	if method == "Runtime.evaluate" {
		var evalParams runtime.EvaluateParams
		if err := json.Unmarshal(params, &evalParams); err != nil {
			return errors.Wrap(err, "parsing Runtime.evaluate parameters")
		}

		var result interface{}
		if err := chromedp.Run(ctx, chromedp.Evaluate(evalParams.Expression, &result)); err != nil {
			return err
		}

		fmt.Println("Result:", result)
		return nil
	}

	// Special case for navigation which is very common
	if method == "Page.navigate" {
		var navParams struct {
			URL string `json:"url"`
		}
		if err := json.Unmarshal(params, &navParams); err != nil {
			return errors.Wrap(err, "parsing Page.navigate parameters")
		}

		if err := chromedp.Run(ctx, chromedp.Navigate(navParams.URL)); err != nil {
			return err
		}

		fmt.Println("Navigated to:", navParams.URL)
		return nil
	}

	// Special case for screenshots which are very common
	if method == "Page.captureScreenshot" {
		var buf []byte
		if err := chromedp.Run(ctx, chromedp.CaptureScreenshot(&buf)); err != nil {
			return err
		}

		// Save screenshot to file
		filename := fmt.Sprintf("screenshot-%d.png", time.Now().Unix())
		if err := os.WriteFile(filename, buf, 0644); err != nil {
			return errors.Wrap(err, "saving screenshot")
		}

		fmt.Println("Screenshot saved to:", filename)
		return nil
	}

	// For other commands, we provide a simplified implementation
	// which doesn't support all CDP methods but covers the basics
	fmt.Printf("Executing: %s with params %s\n", method, string(params))
	fmt.Println("(This is a simplified implementation that doesn't support all CDP methods)")

	// Execute appropriate CDP action if we know how to handle it
	if strings.HasPrefix(method, "Runtime.") {
		return executeCDPRuntime(ctx, method, params)
	} else if strings.HasPrefix(method, "Page.") {
		return executeCDPPage(ctx, method, params)
	} else if strings.HasPrefix(method, "Network.") {
		return executeCDPNetwork(ctx, method, params)
	} else if strings.HasPrefix(method, "DOM.") {
		return executeCDPDOM(ctx, method, params)
	}

	return errors.Errorf("unsupported CDP method: %s", method)
}

func executeCDPRuntime(ctx context.Context, method string, params json.RawMessage) error {
	// Only handle a few common Runtime methods as examples
	switch method {
	case "Runtime.evaluate":
		// Handled specially above
		return nil

	default:
		return errors.Errorf("unsupported Runtime method: %s", method)
	}
}

func executeCDPPage(ctx context.Context, method string, params json.RawMessage) error {
	// Only handle a few common Page methods as examples
	switch method {
	case "Page.navigate":
		// Handled specially above
		return nil

	case "Page.reload":
		return chromedp.Run(ctx, chromedp.Reload())

	case "Page.captureScreenshot":
		// Handled specially above
		return nil

	default:
		return errors.Errorf("unsupported Page method: %s", method)
	}
}

func executeCDPNetwork(ctx context.Context, method string, params json.RawMessage) error {
	// Only handle a few common Network methods as examples
	switch method {
	case "Network.getAllCookies":
		// Simple implementation that just gets cookies via JavaScript
		var cookies interface{}
		if err := chromedp.Run(ctx, chromedp.Evaluate("document.cookie", &cookies)); err != nil {
			return err
		}

		fmt.Println("Cookies:", cookies)
		return nil

	default:
		return errors.Errorf("unsupported Network method: %s", method)
	}
}

func executeCDPDOM(ctx context.Context, method string, params json.RawMessage) error {
	// Only handle a few common DOM methods as examples
	switch method {
	case "DOM.getDocument":
		// Simplified implementation
		var html string
		if err := chromedp.Run(ctx, chromedp.OuterHTML("html", &html)); err != nil {
			return err
		}

		fmt.Printf("HTML length: %d bytes\n", len(html))
		fmt.Println("(HTML content not shown - too large)")
		return nil

	default:
		return errors.Errorf("unsupported DOM method: %s", method)
	}
}

func handleSourcesCommand(ctx context.Context, parts []string) error {
	// Parse flags
	saveDir := ""
	filterType := ""
	getURL := ""

	if len(parts) > 1 {
		args := strings.Fields(parts[1])
		for i := 0; i < len(args); i++ {
			switch args[i] {
			case "--save":
				if i+1 < len(args) {
					saveDir = args[i+1]
					i++
				}
			case "--type":
				if i+1 < len(args) {
					filterType = args[i+1]
					i++
				}
			case "--get":
				if i+1 < len(args) {
					getURL = args[i+1]
					i++
				}
			}
		}
	}

	// Use JavaScript to get all scripts and stylesheets from the DOM
	var sourcesJS string
	err := chromedp.Run(ctx, chromedp.Evaluate(`
		(() => {
			const sources = [];

			// Get all script tags
			document.querySelectorAll('script[src]').forEach(script => {
				sources.push({
					type: 'JavaScript',
					url: script.src,
					inline: false,
					content: null
				});
			});

			// Get inline scripts (with content)
			document.querySelectorAll('script:not([src])').forEach((script, i) => {
				if (script.textContent.trim()) {
					sources.push({
						type: 'JavaScript',
						url: 'inline-script-' + i,
						inline: true,
						size: script.textContent.length,
						content: script.textContent
					});
				}
			});

			// Get all stylesheets
			document.querySelectorAll('link[rel="stylesheet"]').forEach(link => {
				sources.push({
					type: 'CSS',
					url: link.href,
					inline: false,
					content: null
				});
			});

			// Get inline styles (with content)
			document.querySelectorAll('style').forEach((style, i) => {
				if (style.textContent.trim()) {
					sources.push({
						type: 'CSS',
						url: 'inline-style-' + i,
						inline: true,
						size: style.textContent.length,
						content: style.textContent
					});
				}
			});

			return JSON.stringify(sources, null, 2);
		})()
	`, &sourcesJS))

	if err != nil {
		return fmt.Errorf("getting sources: %w", err)
	}

	var sources []struct {
		Type    string  `json:"type"`
		URL     string  `json:"url"`
		Inline  bool    `json:"inline"`
		Size    int     `json:"size"`
		Content *string `json:"content"` // Pointer to handle null
	}

	if err := json.Unmarshal([]byte(sourcesJS), &sources); err != nil {
		return fmt.Errorf("parsing sources: %w", err)
	}

	// Apply type filter if specified
	if filterType != "" {
		filtered := sources[:0]
		for _, src := range sources {
			match := false
			switch strings.ToLower(filterType) {
			case "js", "javascript":
				match = src.Type == "JavaScript"
			case "css":
				match = src.Type == "CSS"
			case "inline":
				match = src.Inline
			case "external":
				match = !src.Inline
			}
			if match {
				filtered = append(filtered, src)
			}
		}
		sources = filtered
	}

	// Handle --get flag for specific URL
	if getURL != "" {
		for _, src := range sources {
			if src.URL == getURL || strings.HasSuffix(src.URL, getURL) {
				if src.Inline && src.Content != nil {
					fmt.Printf("=== %s ===\n%s\n", src.URL, *src.Content)
				} else {
					fmt.Printf("Source URL: %s\n", src.URL)
					fmt.Println("Note: External sources need --save to fetch content")
				}
				return nil
			}
		}
		return fmt.Errorf("source not found: %s", getURL)
	}

	// Display sources
	fmt.Printf("\nFound %d sources", len(sources))
	if filterType != "" {
		fmt.Printf(" (filtered by: %s)", filterType)
	}
	fmt.Println(":")
	fmt.Println(strings.Repeat("─", 80))

	jsCount, cssCount := 0, 0
	for _, src := range sources {
		prefix := "  "
		if src.Type == "JavaScript" {
			prefix = "  [JS]  "
			jsCount++
		} else if src.Type == "CSS" {
			prefix = "  [CSS] "
			cssCount++
		}

		if src.Inline {
			fmt.Printf("%s%s (%d bytes, inline)\n", prefix, src.URL, src.Size)
		} else {
			fmt.Printf("%s%s\n", prefix, src.URL)
		}
	}

	fmt.Println(strings.Repeat("─", 80))
	fmt.Printf("Total: %d JavaScript, %d CSS\n", jsCount, cssCount)

	// Handle --save flag
	if saveDir != "" {
		fmt.Printf("\nSaving sources to %s...\n", saveDir)

		// Create directory
		if err := os.MkdirAll(saveDir, 0755); err != nil {
			return fmt.Errorf("creating directory: %w", err)
		}

		saved := 0
		for i, src := range sources {
			var filename string
			var content string

			if src.Inline {
				// Save inline content directly
				if src.Content == nil {
					continue
				}
				content = *src.Content
				ext := ".js"
				if src.Type == "CSS" {
					ext = ".css"
				}
				filename = fmt.Sprintf("%s/%s%s", saveDir, src.URL, ext)
			} else {
				// For external sources, we'd need to fetch them
				// For now, just save the URL reference
				fmt.Printf("  [%d/%d] Skipping external source: %s (use curl or wget to fetch)\n", i+1, len(sources), src.URL)
				continue
			}

			// Write file
			if err := os.WriteFile(filename, []byte(content), 0644); err != nil {
				fmt.Printf("  Error saving %s: %v\n", filename, err)
				continue
			}

			fmt.Printf("  ✓ Saved %s (%d bytes)\n", filename, len(content))
			saved++
		}

		fmt.Printf("\nSaved %d inline sources to %s\n", saved, saveDir)
		if saved < len(sources) {
			fmt.Println("\nNote: External sources were not fetched. To save them:")
			fmt.Println("  1. List external URLs: sources --type external")
			fmt.Println("  2. Use wget/curl to download them")
		}
	} else {
		fmt.Println("\nOptions:")
		fmt.Println("  sources --save /tmp/sources    - Save all inline sources")
		fmt.Println("  sources --type js              - Filter by type (js, css, inline, external)")
		fmt.Println("  sources --get <url>            - Display specific source content")
	}

	return nil
}

func printHelp() {
	fmt.Println("\nCDP - Chrome DevTools Protocol CLI")
	fmt.Println("\nCommand format:")
	fmt.Println("  Domain.method {\"param\":\"value\"}")
	fmt.Println("  Examples:")
	fmt.Println("    Page.navigate {\"url\":\"https://example.com\"}")
	fmt.Println("    Runtime.evaluate {\"expression\":\"document.title\"}")

	fmt.Println("\nCommon commands:")
	fmt.Println("  Page.navigate     - Navigate to a URL")
	fmt.Println("  Page.reload       - Reload the current page")
	fmt.Println("  Runtime.evaluate  - Evaluate JavaScript")
	fmt.Println("  DOM.getDocument   - Get the DOM document")
	fmt.Println("  Network.getAllCookies - Get all cookies")

	fmt.Println("\nAliases:")
	fmt.Println("  goto <url>        - Navigate to URL")
	fmt.Println("  title             - Get page title")
	fmt.Println("  html              - Get page HTML")
	fmt.Println("  screenshot        - Take screenshot")
	fmt.Println("  Type 'help aliases' for a full list")

	fmt.Println("\nAnnotation commands (--har-mode=enhanced only):")
	fmt.Println("  note <text>       - Add a text annotation to HAR file")
	fmt.Println("  screenshot <desc> - Capture annotated screenshot (optional description)")
	fmt.Println("  dom <desc>        - Capture DOM snapshot (optional description)")
	fmt.Println("  Annotations are saved in HAR file's _annotations field")

	fmt.Println("\nSource extraction:")
	fmt.Println("  sources                      - List all JavaScript and CSS sources")
	fmt.Println("  sources --save <dir>         - Save all inline sources to directory")
	fmt.Println("  sources --type js|css|inline - Filter by source type")
	fmt.Println("  sources --get <url>          - Display specific source content")

	fmt.Println("\nCommands:")
	fmt.Println("  help              - Show this help")
	fmt.Println("  help aliases      - List all alias commands")
	fmt.Println("  help enhanced     - List enhanced commands (remote Chrome only)")
	fmt.Println("  exit / quit       - Exit the program")

	fmt.Println("\nExit Codes:")
	fmt.Println("  0 - Success")
	fmt.Println("  1 - General error")
	fmt.Println("  2 - Command line usage error")
	fmt.Println("  3 - Browser launch/connection failed")
	fmt.Println("  4 - Page navigation failed")
	fmt.Println("  5 - Operation timed out")
}

func executeEnhancedCommand(page *browser.Page, command string) error {
	parts := strings.Fields(command)
	if len(parts) == 0 {
		return errors.New("empty command")
	}

	cmdName := parts[0]
	args := parts[1:]

	if handler, ok := enhancedCommands[cmdName]; ok {
		return handler(page, args)
	}

	return fmt.Errorf("unknown enhanced command: %s", cmdName)
}

func printAliases() {
	fmt.Println("\nAvailable Aliases:")

	categories := map[string][]string{
		"Navigation":       {"goto", "reload"},
		"Page Info":        {"title", "url", "html", "cookies"},
		"Media":            {"screenshot", "pdf"},
		"Interaction":      {"click", "focus", "type"},
		"Device Emulation": {"mobile", "desktop"},
		"Debugging":        {"pause", "resume", "step", "next", "out"},
		"Performance":      {"metrics", "coverage_start", "coverage_take", "coverage_stop"},
	}

	for category, cmds := range categories {
		fmt.Printf("\n%s:\n", category)
		for _, cmd := range cmds {
			if strings.Contains(aliases[cmd], "$1") {
				// Command takes parameters
				fmt.Printf("  %-15s -> %s\n", cmd+" <param>", aliases[cmd])
			} else {
				fmt.Printf("  %-15s -> %s\n", cmd, aliases[cmd])
			}
		}
	}
}

func printEnhancedCommands() {
	fmt.Println("\nEnhanced Commands (prefixed with @):")
	fmt.Println("\nWaiting:")
	fmt.Println("  @wait <selector>      - Wait for element to appear")
	fmt.Println("  @waitfor <ms>         - Wait for milliseconds")

	fmt.Println("\nElement Interaction:")
	fmt.Println("  @text <selector>      - Get element text")
	fmt.Println("  @hover <selector>     - Hover over element")
	fmt.Println("  @fill <sel> <text>    - Fill input field")
	fmt.Println("  @clear <selector>     - Clear input field")
	fmt.Println("  @press <key>          - Press keyboard key")
	fmt.Println("  @select <sel> <val>   - Select dropdown option")

	fmt.Println("\nElement State:")
	fmt.Println("  @visible <selector>   - Check if element is visible")
	fmt.Println("  @count <selector>     - Count matching elements")
	fmt.Println("  @attr <sel> <name>    - Get attribute value")

	fmt.Println("\nNetwork:")
	fmt.Println("  @route <pattern> <action>  - Intercept requests (abort/log)")

	fmt.Println("\nNote: These commands are only available when connected to remote Chrome")
}

// isNonBrowserCommand checks if a command can run without browser setup
func isNonBrowserCommand(cmdName string) bool {
	nonBrowserCommands := map[string]bool{
		"help":   true,
		"h":      true,
		"?":      true,
		"list":   true,
		"ls":     true,
		"search": true,
		"find":   true,
		"quick":  true,
		"qr":     true,
		"ref":    true,
	}
	return nonBrowserCommands[cmdName]
}

// handleEnhancedMode handles the new enhanced command mode
func handleEnhancedMode(command string, interactive bool, verbose bool, chromePath string) {
	registry := NewCommandRegistry()
	help := NewHelpSystem(registry)

	// If no command specified, show help and optionally enter interactive mode
	if command == "" {
		if interactive {
			// Set up Chrome context for interactive mode
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			// Try to connect to existing Chrome or launch new one
			chromeCtx, chromeCancel, err := setupChromeForEnhanced(ctx, verbose, chromePath)
			if err != nil {
				exitWithError(ExitBrowserError, ErrorTypeBrowser, "Failed to setup Chrome: %v", err)
			}
			defer chromeCancel()

			// Start interactive mode
			im := NewInteractiveMode(chromeCtx, verbose)
			if err := im.Run(); err != nil {
				exitWithError(ExitGeneralError, ErrorTypeGeneral, "Interactive mode error: %v", err)
			}
		} else {
			help.ShowHelp(nil)
		}
		return
	}

	// Parse command first
	parts := strings.Fields(command)
	if len(parts) == 0 {
		fmt.Println("No command specified")
		return
	}

	cmdName := parts[0]
	args := parts[1:]

	// Check if this is a non-browser command (help, list, search)
	if isNonBrowserCommand(cmdName) {
		// Execute without setting up browser
		if cmdName == "help" {
			if len(args) > 0 {
				help.ShowHelp(args)
			} else {
				help.ShowHelp(nil)
			}
		} else if cmdName == "list" || cmdName == "ls" {
			help.ListCommands()
		} else if cmdName == "search" || cmdName == "find" {
			if len(args) > 0 {
				help.SearchCommands(strings.Join(args, " "))
			} else {
				fmt.Println("Usage: search <term>")
			}
		} else if cmdName == "quick" || cmdName == "qr" || cmdName == "ref" {
			help.ShowQuickReference()
		}
		return
	}

	// Execute single command that requires browser
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// Set up Chrome context
	chromeCtx, chromeCancel, err := setupChromeForEnhanced(ctx, verbose, chromePath)
	if err != nil {
		exitWithError(ExitBrowserError, ErrorTypeBrowser, "Failed to setup Chrome: %v", err)
	}
	defer chromeCancel()

	// Check for special commands
	switch cmdName {
	case "help", "h":
		help.ShowHelp(args)
		return
	case "list":
		help.ListCommands()
		return
	case "search":
		if len(args) > 0 {
			help.SearchCommands(strings.Join(args, " "))
		} else {
			fmt.Println("Usage: search <term>")
		}
		return
	}

	// Execute command
	if cmd, found := registry.GetCommand(cmdName); found {
		if verbose {
			fmt.Printf("Executing: %s %v\n", cmdName, args)
		}
		if err := cmd.Handler(chromeCtx, args); err != nil {
			exitWithError(ExitGeneralError, ErrorTypeGeneral, "Command failed: %v", err)
		}
	} else {
		completions := help.GetCompletions(cmdName)
		if len(completions) > 0 {
			fmt.Printf("Unknown command '%s'. Did you mean:\n", cmdName)
			for _, c := range completions {
				fmt.Printf("  • %s\n", c)
			}
		} else {
			fmt.Printf("Unknown command: %s\n", cmdName)
			fmt.Println("Use 'help' to see available commands")
		}
	}
}

// setupChromeForEnhanced sets up Chrome context for enhanced commands
func setupChromeForEnhanced(ctx context.Context, verbose bool, chromePath string) (context.Context, context.CancelFunc, error) {
	// Check for existing Chrome instances first
	debugPorts := []int{9222, 9223, 9224, 9225}
	for _, port := range debugPorts {
		if checkRunningChrome(port) {
			if verbose {
				log.Printf("Connecting to existing Chrome on port %d", port)
			}

			remoteURL := fmt.Sprintf("ws://localhost:%d", port)
			allocCtx, allocCancel := chromedp.NewRemoteAllocator(ctx, remoteURL)

			var opts []chromedp.ContextOption
			if verbose {
				opts = append(opts, chromedp.WithLogf(log.Printf))
			}

			browserCtx, browserCancel := chromedp.NewContext(allocCtx, opts...)

			// Test connection
			testCtx, testCancel := context.WithTimeout(browserCtx, 5*time.Second)
			if err := chromedp.Run(testCtx, chromedp.Evaluate("1+1", nil)); err != nil {
				testCancel()
				browserCancel()
				allocCancel()
				continue // Try next port
			}
			testCancel()

			// Return combined cancel function
			cancel := func() {
				browserCancel()
				allocCancel()
			}

			return browserCtx, cancel, nil
		}
	}

	// Auto-discover browser if no running Chrome found
	var selectedPath string = chromePath
	if chromePath == "" {
		candidates, err := discoverBrowsers(verbose)
		if err == nil && len(candidates) > 0 {
			best := selectBestBrowser(candidates, verbose)
			if best.Path != "" {
				selectedPath = best.Path
				if verbose {
					log.Printf("Auto-selected browser: %s at %s", best.Name, selectedPath)
				}
			}
		}
	}

	// Launch new Chrome instance
	if verbose {
		log.Println("Launching new Chrome instance...")
	}

	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Flag("remote-debugging-port", "9222"),
		chromedp.NoFirstRun,
		chromedp.NoDefaultBrowserCheck,
	)

	// Override Chrome executable if we have a specific path
	if selectedPath != "" {
		opts = append(chromedp.DefaultExecAllocatorOptions[:],
			chromedp.ExecPath(selectedPath),
			chromedp.Flag("remote-debugging-port", "9222"),
			chromedp.NoFirstRun,
			chromedp.NoDefaultBrowserCheck,
		)
	}

	allocCtx, allocCancel := chromedp.NewExecAllocator(ctx, opts...)

	var browserOpts []chromedp.ContextOption
	if verbose {
		browserOpts = append(browserOpts, chromedp.WithLogf(log.Printf))
	}

	browserCtx, browserCancel := chromedp.NewContext(allocCtx, browserOpts...)

	// Test that Chrome starts successfully
	testCtx, testCancel := context.WithTimeout(browserCtx, 10*time.Second)
	if err := chromedp.Run(testCtx, chromedp.Navigate("about:blank")); err != nil {
		testCancel()
		browserCancel()
		allocCancel()
		return nil, nil, errors.Wrap(err, "failed to start Chrome")
	}
	testCancel()

	// Return combined cancel function
	cancel := func() {
		browserCancel()
		allocCancel()
	}

	return browserCtx, cancel, nil
}

// buildExtractionScript builds a JavaScript extraction script based on mode
func buildExtractionScript(selector, mode string) string {
	// Handle attr:attrName mode
	if strings.HasPrefix(mode, "attr:") {
		attrName := strings.TrimPrefix(mode, "attr:")
		return fmt.Sprintf(`
const elements = document.querySelectorAll('%s');
if (elements.length === 0) {
  JSON.stringify({error: "No elements found"});
} else if (elements.length === 1) {
  JSON.stringify({attr: '%s', value: elements[0].getAttribute('%s')});
} else {
  JSON.stringify(Array.from(elements).map(el => ({attr: '%s', value: el.getAttribute('%s')})));
}
`, selector, attrName, attrName, attrName, attrName)
	}

	switch mode {
	case "html":
		return fmt.Sprintf(`
const elements = document.querySelectorAll('%s');
if (elements.length === 0) {
  JSON.stringify({error: "No elements found"});
} else if (elements.length === 1) {
  JSON.stringify({html: elements[0].outerHTML});
} else {
  JSON.stringify(Array.from(elements).map(el => ({html: el.outerHTML})));
}
`, selector)
	case "count":
		return fmt.Sprintf(`
const count = document.querySelectorAll('%s').length;
JSON.stringify({count: count, selector: '%s'});
`, selector, selector)
	default: // text mode
		return fmt.Sprintf(`
const elements = document.querySelectorAll('%s');
if (elements.length === 0) {
  JSON.stringify({error: "No elements found"});
} else if (elements.length === 1) {
  JSON.stringify({text: elements[0].textContent});
} else {
  JSON.stringify(Array.from(elements).map(el => ({text: el.textContent})));
}
`, selector)
	}
}

// monitorURLChanges monitors for URL changes and outputs matching URLs
func monitorURLChanges(ctx context.Context, pattern string, verbose bool) error {
	var regex *regexp.Regexp
	var err error

	if pattern != "" {
		regex, err = regexp.Compile(pattern)
		if err != nil {
			return fmt.Errorf("invalid URL pattern: %v", err)
		}
	}

	var lastURL string
	// Get initial URL
	if err := chromedp.Run(ctx, chromedp.Location(&lastURL)); err != nil {
		return fmt.Errorf("failed to get initial URL: %v", err)
	}

	if verbose {
		fmt.Printf("Monitoring URL changes (initial: %s)\n", lastURL)
		if pattern != "" {
			fmt.Printf("Looking for URLs matching pattern: %s\n", pattern)
		}
	}

	// Monitor for changes
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			var currentURL string
			if err := chromedp.Run(ctx, chromedp.Location(&currentURL)); err != nil {
				if verbose {
					log.Printf("Failed to get current URL: %v", err)
				}
				continue
			}

			if currentURL != lastURL {
				fmt.Printf("URL changed: %s\n", currentURL)

				// Check if URL matches pattern
				if regex != nil && regex.MatchString(currentURL) {
					fmt.Printf("✓ URL matches pattern: %s\n", currentURL)
					return nil // Stop monitoring on match
				}

				lastURL = currentURL
			}
		}
	}
}
