// Package browser provides abstractions for managing Chrome browser instances.
package browser

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/chromedp/cdproto/fetch"
	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"
	"github.com/tmc/cdp/internal/blocking"
	"github.com/tmc/cdp/internal/chromeprofiles"
)

// filteredErrorf filters out noisy chromedp error messages
func filteredErrorf(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	// Filter out noisy DOM event messages
	if strings.Contains(msg, "unhandled node event") ||
		strings.Contains(msg, "TopLayerElementsUpdated") {
		return
	}
	log.Printf(format, args...)
}

// HTTPRequestData represents data for HTTP requests
type HTTPRequestData struct {
	Method      string
	URL         string
	Data        string
	Headers     map[string]string
	ContentType string
}

// requestInterceptor handles request interception and modification
type requestInterceptor struct {
	mu           sync.RWMutex
	targetURL    string
	requestData  *HTTPRequestData
	intercepting bool
}

// Browser represents a managed Chrome browser instance
type Browser struct {
	ctx            context.Context
	cancelFunc     context.CancelFunc
	opts           *Options
	profileMgr     chromeprofiles.ProfileManager
	interceptor    *requestInterceptor
	blockingEngine *blocking.BlockingEngine
	attachedToTab  bool // True if connected to existing tab (don't close on cleanup)
}

// New creates a new Browser with the provided options
func New(ctx context.Context, profileMgr chromeprofiles.ProfileManager, opts ...Option) (*Browser, error) {
	// Create default options
	options := defaultOptions()

	// Apply all option functions
	for _, opt := range opts {
		if err := opt(options); err != nil {
			return nil, fmt.Errorf("applying browser option: %w", err)
		}
	}

	browser := &Browser{
		opts:        options,
		profileMgr:  profileMgr,
		interceptor: &requestInterceptor{},
	}

	// Initialize blocking engine if blocking is enabled
	if options.BlockingEnabled {
		blockingConfig := &blocking.Config{
			Verbose:       options.BlockingVerbose,
			Enabled:       options.BlockingEnabled,
			URLPatterns:   options.BlockedURLPatterns,
			Domains:       options.BlockedDomains,
			RegexPatterns: options.BlockedRegexPatterns,
			AllowURLs:     options.AllowedURLs,
			AllowDomains:  options.AllowedDomains,
			RuleFile:      options.BlockingRuleFile,
		}

		blockingEngine, err := blocking.NewBlockingEngine(blockingConfig)
		if err != nil {
			return nil, fmt.Errorf("creating blocking engine: %w", err)
		}

		// Add common blocking rules if requested
		if options.BlockCommonAds {
			if err := blockingEngine.AddCommonAdBlockRules(); err != nil {
				return nil, fmt.Errorf("adding common ad blocking rules: %w", err)
			}
		}

		if options.BlockCommonTracking {
			if err := blockingEngine.AddCommonTrackingBlockRules(); err != nil {
				return nil, fmt.Errorf("adding common tracking blocking rules: %w", err)
			}
		}

		browser.blockingEngine = blockingEngine

		if options.Verbose {
			log.Printf("Blocking engine initialized with %d rules", len(blockingEngine.ListRules()))
		}
	}

	return browser, nil
}

// Launch starts the browser or connects to a running instance
func (b *Browser) Launch(ctx context.Context) error {
	// If using remote Chrome, connect to it instead of launching
	if b.opts.UseRemote {
		if b.opts.RemoteHost == "" {
			b.opts.RemoteHost = "localhost"
		}

		if b.opts.RemoteTabID != "" {
			// Connect to specific tab
			return b.ConnectToTab(ctx, b.opts.RemoteHost, b.opts.RemotePort, b.opts.RemoteTabID)
		}
		// Connect to first existing page tab (avoids creating a new blank tab)
		return b.ConnectToFirstTab(ctx, b.opts.RemoteHost, b.opts.RemotePort)
	}

	// Set up the profile directory if needed
	if b.opts.UseProfile && b.profileMgr != nil {
		if err := b.profileMgr.SetupWorkdir(); err != nil {
			return fmt.Errorf("setting up working directory: %w", err)
		}

		// Check for Brave session isolation needs
		sessionDetector := NewSessionDetector(b.opts.Verbose)
		needsIsolation := sessionDetector.NeedsBraveSessionIsolation(b.ctx, b.opts.ChromePath, true)

		if needsIsolation {
			// Display warning about Brave session reuse
			if b.opts.Verbose {
				log.Println(sessionDetector.ImportantWarning())
			}

			// Use Brave session isolation instead of standard profile copy
			if err := b.profileMgr.BraveSessionIsolation(b.opts.ProfileName, b.opts.CookieDomains); err != nil {
				return fmt.Errorf("creating Brave isolated profile: %w", err)
			}

			if b.opts.Verbose {
				log.Printf("Brave session isolation created for profile '%s'", b.opts.ProfileName)
			}
		} else {
			// Standard profile copy
			if err := b.profileMgr.CopyProfile(b.opts.ProfileName, b.opts.CookieDomains); err != nil {
				return fmt.Errorf("copying profile: %w", err)
			}
		}
	}

	// Set up the Chrome options with security hardening
	chromeLaunchOpts := b.getSecureChromeOptions()

	// If headless mode is enabled
	if b.opts.Headless {
		chromeLaunchOpts = append(chromeLaunchOpts, chromedp.Headless)
	}

	// If a profile is being used
	if b.opts.UseProfile && b.profileMgr != nil {
		userDataDir := b.profileMgr.WorkDir()
		chromeLaunchOpts = append(chromeLaunchOpts, chromedp.UserDataDir(userDataDir))
		if b.opts.Verbose {
			log.Printf("Using UserDataDir: %s", userDataDir)
		}
	}

	// Add Chrome path if specified
	if b.opts.ChromePath != "" {
		chromeLaunchOpts = append(chromeLaunchOpts, chromedp.ExecPath(b.opts.ChromePath))
		if b.opts.Verbose {
			log.Printf("Using Chrome executable: %s", b.opts.ChromePath)
		}
	}

	// Add remote debugging port if specified
	if b.opts.DebugPort > 0 {
		portStr := fmt.Sprintf("%d", b.opts.DebugPort)
		chromeLaunchOpts = append(chromeLaunchOpts, chromedp.Flag("remote-debugging-port", portStr))
	}

	// Enable logging if verbose
	if b.opts.Verbose {
		chromeLaunchOpts = append(chromeLaunchOpts, chromedp.CombinedOutput(os.Stdout))
	}

	// Add custom Chrome flags
	for _, flag := range b.opts.ChromeFlags {
		chromeLaunchOpts = append(chromeLaunchOpts, chromedp.Flag(flag, true))
	}

	// Apply proxy settings if configured
	if b.opts.ProxyServer != "" {
		chromeLaunchOpts = append(chromeLaunchOpts, chromedp.ProxyServer(b.opts.ProxyServer))

		// Add proxy bypass list if specified
		if b.opts.ProxyBypassList != "" {
			chromeLaunchOpts = append(chromeLaunchOpts, chromedp.Flag("proxy-bypass-list", b.opts.ProxyBypassList))
		}
	}

	// Create the allocator context
	allocCtx, allocCancel := chromedp.NewExecAllocator(ctx, chromeLaunchOpts...)

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
		browserCtx, browserCancel = chromedp.NewContext(
			allocCtx,
			chromedp.WithErrorf(filteredErrorf),
		)
	}

	// Store context and cancel functions
	b.ctx = browserCtx
	b.cancelFunc = func() {
		// Removed verbose logging to reduce noise in tests
		browserCancel()
		allocCancel()
	}

	// Add monitoring for context cancellation if verbose
	// DISABLED: This monitoring goroutine is causing noise in tests
	// if b.opts.Verbose {
	// 	go func() {
	// 		<-b.ctx.Done()
	// 		log.Printf("Browser context was canceled: %v", b.ctx.Err())
	// 	}()
	// }

	// Test connection with a simple evaluation to ensure browser launches properly
	// DISABLED FOR DEBUGGING - This seems to interfere with Brave Browser navigation
	// if b.opts.Verbose {
	// 	log.Println("Testing Chrome connection...")
	// }

	// testCtx, testCancel := context.WithTimeout(browserCtx, 5*time.Second)
	// defer testCancel()

	// var result bool
	// if err := chromedp.Run(testCtx, chromedp.Evaluate(`true`, &result)); err != nil {
	// 	b.cancelFunc()
	// }

	if b.opts.Verbose {
		log.Printf("Successfully launched Chrome browser")
	}

	// Set up proxy authentication if credentials are provided
	if b.opts.ProxyServer != "" && b.opts.ProxyUsername != "" && b.opts.ProxyPassword != "" {
		if err := b.setupProxyAuthentication(); err != nil {
			b.cancelFunc()
			return fmt.Errorf("setting up proxy authentication: %w", err)
		}
	}

	// Set up network interception for blocking if blocking engine is present
	if b.blockingEngine != nil {
		if err := b.setupNetworkBlocking(); err != nil {
			b.cancelFunc()
			return fmt.Errorf("setting up network blocking: %w", err)
		}
	}

	return nil
}

// Navigate visits the specified URL
func (b *Browser) Navigate(url string) error {
	if b.ctx == nil {
		return notLaunchedError()
	}

	if b.opts.Verbose {
		log.Printf("Navigating to: %s", url)
	}

	// NOTE: Creating a timeout context from b.ctx causes issues with Brave Browser
	// For now, we use b.ctx directly for navigation
	// TODO: Investigate why Brave doesn't handle timeout contexts properly
	_ = time.Duration(b.opts.Timeout) * time.Second // Keep for future fix

	// Enable network events if we need to wait for network idle
	if b.opts.Verbose {
		log.Printf("WaitNetworkIdle setting: %v", b.opts.WaitNetworkIdle)
	}
	if b.opts.WaitNetworkIdle {
		if b.opts.Verbose {
			log.Printf("Enabling network events...")
		}
		if err := chromedp.Run(b.ctx, network.Enable()); err != nil {
			if b.opts.Verbose {
				log.Printf("network.Enable failed: %v", err)
			}
			return wrapError(ErrNetwork, "enable network events", err)
		}
		if b.opts.Verbose {
			log.Printf("Successfully enabled network events")
		}
	}

	// Navigate to the URL
	if err := chromedp.Run(b.ctx, chromedp.Navigate(url)); err != nil {
		if b.opts.Verbose {
			log.Printf("Navigation error: %v", err)
		}
		return wrapError(ErrNavigation, "navigate to URL", err)
	}

	// Execute pre-navigation scripts after basic navigation but before waiting for network idle
	if err := b.executeScriptsBefore(); err != nil {
		if b.opts.Verbose {
			log.Printf("Pre-navigation script error: %v", err)
		}
		return fmt.Errorf("execute pre-navigation scripts: %w", err)
	}

	// If waiting for network idle is requested
	if b.opts.WaitNetworkIdle {
		// Use a reasonable wait timeout, but don't exceed the browser context
		waitTimeout := time.Duration(b.opts.StableTimeout) * time.Second
		waitCtx, waitCancel := context.WithTimeout(b.ctx, waitTimeout)
		defer waitCancel()

		if err := chromedp.Run(waitCtx, chromedp.ActionFunc(func(ctx context.Context) error {
			// This will wait until there are no more than 2 network connections for at least 500ms
			ch := make(chan struct{})
			lctx, cancel := context.WithCancel(ctx)
			chromedp.ListenTarget(lctx, func(ev interface{}) {
				switch ev.(type) {
				case *network.EventLoadingFinished, *network.EventLoadingFailed,
					*page.EventLoadEventFired, *page.EventDomContentEventFired:
					select {
					case ch <- struct{}{}:
					default:
					}
				}
			})

			// Wait for idle using a timer
			idleTimer := time.NewTimer(500 * time.Millisecond)
			defer cancel()

			for {
				select {
				case <-waitCtx.Done():
					return waitCtx.Err()
				case <-idleTimer.C:
					// We've been idle for 500ms
					return nil
				case <-ch:
					// Reset the timer when any network event occurs
					if !idleTimer.Stop() {
						<-idleTimer.C
					}
					idleTimer.Reset(500 * time.Millisecond)
				}
			}
		})); err != nil {
			return wrapError(ErrNetworkIdle, fmt.Sprintf("wait for network idle after navigating to %s", url), err)
		}
	}

	// If waiting for full page stability
	if b.opts.WaitForStability {
		// Use the new stability detection system
		pages, err := b.Pages()
		if err != nil || len(pages) == 0 {
			// If we can't get pages, fall back to creating a page context
			if b.opts.Verbose {
				log.Println("Creating page context for stability detection")
			}

			// Create a temporary page wrapper
			page := &Page{ctx: b.ctx, browser: b}

			// Configure stability detection
			if b.opts.StabilityConfig != nil {
				page.stabilityDetector = NewStabilityDetector(page, b.opts.StabilityConfig)
			} else {
				page.ConfigureStability(WithVerboseLogging(b.opts.Verbose))
			}

			// Wait for stability
			waitTimeout := time.Duration(b.opts.StableTimeout) * time.Second
			waitCtx, waitCancel := context.WithTimeout(b.ctx, waitTimeout)
			defer waitCancel()

			if err := page.WaitForStability(waitCtx, b.opts.StabilityConfig); err != nil {
				if b.opts.Verbose {
					log.Printf("Stability detection failed: %v", err)
				}
				// Don't fail navigation on stability timeout, just log it
			}
		} else {
			// Use the first page (main tab)
			page := pages[0]

			// Configure stability detection
			if b.opts.StabilityConfig != nil {
				page.stabilityDetector = NewStabilityDetector(page, b.opts.StabilityConfig)
			} else {
				page.ConfigureStability(WithVerboseLogging(b.opts.Verbose))
			}

			// Wait for stability
			waitTimeout := time.Duration(b.opts.StableTimeout) * time.Second
			waitCtx, waitCancel := context.WithTimeout(b.ctx, waitTimeout)
			defer waitCancel()

			if err := page.WaitForStability(waitCtx, b.opts.StabilityConfig); err != nil {
				if b.opts.Verbose {
					log.Printf("Stability detection failed: %v", err)
				}
				// Don't fail navigation on stability timeout, just log it
			}
		}
	}

	// If waiting for a specific CSS selector
	if b.opts.WaitSelector != "" {
		selectorTimeout := time.Duration(b.opts.StableTimeout) * time.Second
		waitCtx, waitCancel := context.WithTimeout(b.ctx, selectorTimeout)
		defer waitCancel()

		if err := chromedp.Run(waitCtx, chromedp.WaitVisible(b.opts.WaitSelector, chromedp.ByQuery)); err != nil {
			return wrapError(ErrTimeout, fmt.Sprintf("wait for selector %q", b.opts.WaitSelector), err)
		}
	}

	// Execute post-navigation scripts
	if err := b.executeScriptsAfter(); err != nil {
		if b.opts.Verbose {
			log.Printf("Post-navigation script error: %v", err)
		}
		return fmt.Errorf("execute post-navigation scripts: %w", err)
	}

	return nil
}

// GetHTML returns the current page's HTML content
func (b *Browser) GetHTML() (string, error) {
	if b.ctx == nil {
		return "", notLaunchedError()
	}

	var html string
	if err := chromedp.Run(b.ctx, chromedp.OuterHTML("html", &html)); err != nil {
		return "", wrapError(ErrScript, "get page HTML", err)
	}

	return html, nil
}

// GetCurrentPage returns a Page wrapper for the current browser context
// This is useful when connected to a remote tab
func (b *Browser) GetCurrentPage() *Page {
	if b.ctx == nil {
		return nil
	}

	return &Page{
		ctx:     b.ctx,
		cancel:  func() {}, // Browser owns the context
		browser: b,
	}
}

// Context returns the browser's context
func (b *Browser) Context() context.Context {
	return b.ctx
}

// Close shuts down the browser
func (b *Browser) Close() error {
	// If we're attached to an existing tab, don't cancel (which would close the tab)
	// Just disconnect gracefully
	if b.attachedToTab {
		// Don't call cancelFunc - let the tab continue running
		return nil
	}

	if b.cancelFunc != nil {
		b.cancelFunc()
	}

	if b.profileMgr != nil {
		if err := b.profileMgr.Cleanup(); err != nil {
			return fmt.Errorf("failed to clean up profile: %w", err)
		}
	}

	return nil
}

// getSecureChromeOptions returns Chrome options with security hardening enabled
func (b *Browser) getSecureChromeOptions() []chromedp.ExecAllocatorOption {
	securityProfile := b.opts.SecurityProfile
	if securityProfile == "" {
		securityProfile = "balanced" // Default to balanced security
	}

	// Base options for all security profiles
	baseOpts := []chromedp.ExecAllocatorOption{
		chromedp.NoFirstRun,
		chromedp.NoDefaultBrowserCheck,
		// chromedp.WSURLReadTimeout(180 * time.Second), // This seems to cause issues with Brave
	}

	// Security-focused options based on profile
	switch securityProfile {
	case "strict":
		return append(baseOpts, b.getStrictSecurityOptions()...)
	case "balanced":
		return append(baseOpts, b.getBalancedSecurityOptions()...)
	case "permissive":
		if b.opts.Verbose {
			log.Println("WARNING: Running with permissive security settings. This should only be used for testing!")
		}
		return append(baseOpts, b.getPermissiveSecurityOptions()...)
	default:
		if b.opts.Verbose {
			log.Printf("Unknown security profile '%s', defaulting to balanced", securityProfile)
		}
		return append(baseOpts, b.getBalancedSecurityOptions()...)
	}
}

// getStrictSecurityOptions returns the most secure Chrome options
func (b *Browser) getStrictSecurityOptions() []chromedp.ExecAllocatorOption {
	opts := []chromedp.ExecAllocatorOption{
		// Enable sandboxing (CRITICAL SECURITY FIX)
		chromedp.Flag("no-sandbox", false),             // Ensure sandbox is NOT disabled
		chromedp.Flag("disable-setuid-sandbox", false), // Keep setuid sandbox enabled

		// Enable site isolation and process isolation
		chromedp.Flag("site-per-process", true),
		chromedp.Flag("enable-features", "SitePerProcess,NetworkServiceSandbox,StrictOriginIsolation"),

		// Security-focused flags
		chromedp.Flag("disable-web-security", false),                 // Keep web security enabled
		chromedp.Flag("disable-features", "TranslateUI,MediaRouter"), // Only disable non-security features
		chromedp.Flag("enable-strict-mixed-content-checking", true),
		chromedp.Flag("enable-strict-powerful-feature-restrictions", true),

		// Block dangerous content
		chromedp.Flag("block-new-web-contents", true),
		chromedp.Flag("disable-plugins", true),
		chromedp.Flag("disable-java", true),
		chromedp.Flag("disable-3d-apis", true),
		chromedp.Flag("disable-webgl", true),

		// Disable extensions for security
		chromedp.Flag("disable-extensions", true),
		chromedp.Flag("disable-default-apps", true),

		// Essential stability flags (security-neutral)
		chromedp.Flag("disable-background-networking", true),
		chromedp.Flag("disable-background-timer-throttling", true),
		chromedp.Flag("disable-backgrounding-occluded-windows", true),
		chromedp.Flag("disable-renderer-backgrounding", true),
		chromedp.Flag("disable-sync", true),
		chromedp.Flag("metrics-recording-only", true),
		chromedp.Flag("no-first-run", true),

		// GPU handling - keep enabled for security
		chromedp.Flag("disable-gpu", false),
		chromedp.Flag("gpu-sandbox-failures-fatal", true),

		// Memory management
		chromedp.Flag("disable-dev-shm-usage", false), // Only disable if absolutely necessary
		chromedp.Flag("memory-pressure-off", false),

		// Secure defaults
		chromedp.Flag("force-color-profile", "srgb"),
	}

	// Only use mock keychain when NOT using a profile
	// When using a profile, we need access to the real keychain to decrypt cookies
	if !b.opts.UseProfile {
		opts = append(opts,
			chromedp.Flag("password-store", "basic"),
			chromedp.Flag("use-mock-keychain", true),
		)
	}

	return opts
}

// getBalancedSecurityOptions returns moderate security options with good compatibility
func (b *Browser) getBalancedSecurityOptions() []chromedp.ExecAllocatorOption {
	opts := []chromedp.ExecAllocatorOption{
		// Enable core sandboxing
		chromedp.Flag("no-sandbox", false),
		chromedp.Flag("disable-setuid-sandbox", false),

		// Enable site isolation
		chromedp.Flag("site-per-process", true),
		chromedp.Flag("enable-features", "SitePerProcess,NetworkServiceSandbox"),

		// Essential security
		chromedp.Flag("disable-web-security", false),
		chromedp.Flag("block-new-web-contents", true),

		// Disable risky features
		chromedp.Flag("disable-extensions", true),
		chromedp.Flag("disable-default-apps", true),
		chromedp.Flag("disable-plugins", true),

		// Automation-friendly flags (prevent popups/prompts)
		chromedp.Flag("disable-fre", true),                                                   // Disable First Run Experience
		chromedp.Flag("auto-accept-browser-signin-for-tests", true),                          // Auto-accept sign-in prompts
		chromedp.Flag("disable-features", "OfferMigrationToDiceUsers,OptGuideOnDeviceModel"), // Disable account migration and ML prompts

		// Stability flags
		chromedp.Flag("disable-background-networking", true),
		chromedp.Flag("disable-background-timer-throttling", true),
		chromedp.Flag("disable-backgrounding-occluded-windows", true),
		chromedp.Flag("disable-renderer-backgrounding", true),
		chromedp.Flag("disable-sync", true),
		chromedp.Flag("disable-breakpad", true),
		chromedp.Flag("disable-hang-monitor", true),
		chromedp.Flag("disable-ipc-flooding-protection", true),
		chromedp.Flag("disable-prompt-on-repost", true),
		chromedp.Flag("metrics-recording-only", true),
		chromedp.Flag("no-first-run", true),

		// GPU - disable for headless stability
		chromedp.DisableGPU,

		// Memory management
		chromedp.Flag("disable-dev-shm-usage", true),

		// Defaults
		chromedp.Flag("force-color-profile", "srgb"),
	}

	// Only use mock keychain when NOT using a profile
	// When using a profile, we need access to the real keychain to decrypt cookies
	if !b.opts.UseProfile {
		opts = append(opts,
			chromedp.Flag("password-store", "basic"),
			chromedp.Flag("use-mock-keychain", true),
		)
	}

	return opts
}

// getPermissiveSecurityOptions returns less secure options for compatibility (TESTING ONLY)
func (b *Browser) getPermissiveSecurityOptions() []chromedp.ExecAllocatorOption {
	opts := []chromedp.ExecAllocatorOption{
		// WARNING: These options reduce security and should only be used for testing
		chromedp.Flag("disable-web-security", true),
		chromedp.Flag("disable-features", "VizDisplayCompositor,OfferMigrationToDiceUsers,OptGuideOnDeviceModel"),
		chromedp.Flag("disable-client-side-phishing-detection", true),
		chromedp.Flag("disable-popup-blocking", true),
		chromedp.Flag("safebrowsing-disable-auto-update", true),

		// Still maintain some basic security
		chromedp.Flag("disable-extensions", true),
		chromedp.Flag("disable-default-apps", true),

		// Automation-friendly flags (prevent popups/prompts)
		chromedp.Flag("disable-fre", true),                          // Disable First Run Experience
		chromedp.Flag("auto-accept-browser-signin-for-tests", true), // Auto-accept sign-in prompts

		// Stability flags
		chromedp.Flag("disable-background-networking", true),
		chromedp.Flag("disable-background-timer-throttling", true),
		chromedp.Flag("disable-backgrounding-occluded-windows", true),
		chromedp.Flag("disable-renderer-backgrounding", true),
		chromedp.Flag("disable-sync", true),
		chromedp.Flag("disable-breakpad", true),
		chromedp.Flag("disable-hang-monitor", true),
		chromedp.Flag("disable-ipc-flooding-protection", true),
		chromedp.Flag("disable-prompt-on-repost", true),
		chromedp.Flag("metrics-recording-only", true),
		chromedp.Flag("no-first-run", true),

		// GPU and memory
		chromedp.DisableGPU,
		chromedp.Flag("disable-dev-shm-usage", true),

		// Defaults
		chromedp.Flag("force-color-profile", "srgb"),
	}

	// Only use mock keychain when NOT using a profile
	// When using a profile, we need access to the real keychain to decrypt cookies
	if !b.opts.UseProfile {
		opts = append(opts,
			chromedp.Flag("password-store", "basic"),
			chromedp.Flag("use-mock-keychain", true),
		)
	}

	return opts
}

// WaitForSelector waits for a CSS selector to be visible
func (b *Browser) WaitForSelector(selector string, timeout time.Duration) error {
	if b.ctx == nil {
		return notLaunchedError()
	}

	waitCtx, waitCancel := context.WithTimeout(b.ctx, timeout)
	defer waitCancel()

	if err := chromedp.Run(waitCtx, chromedp.WaitVisible(selector, chromedp.ByQuery)); err != nil {
		return withField(timeoutError("failed to wait for selector", err), "selector", selector)
	}
	return nil
}

// ExecuteScript runs JavaScript in the browser and returns the result
func (b *Browser) ExecuteScript(script string) (interface{}, error) {
	if b.ctx == nil {
		return nil, notLaunchedError()
	}

	var result interface{}
	err := chromedp.Run(b.ctx, chromedp.Evaluate(script, &result))
	if err != nil {
		return nil, withField(scriptError("failed to execute script", err), "script", script)
	}

	return result, nil
}

// ExecuteScriptWithTimeout runs JavaScript with a custom timeout
func (b *Browser) ExecuteScriptWithTimeout(script string, timeout time.Duration) (interface{}, error) {
	if b.ctx == nil {
		return nil, notLaunchedError()
	}

	ctx, cancel := context.WithTimeout(b.ctx, timeout)
	defer cancel()

	var result interface{}
	err := chromedp.Run(ctx, chromedp.Evaluate(script, &result))
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return nil, withField(timeoutError("script execution timed out", err), "script", script)
		}
		return nil, withField(scriptError("failed to execute script", err), "script", script)
	}

	return result, nil
}

// ExecuteScripts runs multiple JavaScript scripts in sequence
func (b *Browser) ExecuteScripts(scripts []string) ([]interface{}, error) {
	if b.ctx == nil {
		return nil, notLaunchedError()
	}

	if len(scripts) == 0 {
		return []interface{}{}, nil
	}

	results := make([]interface{}, len(scripts))

	for i, script := range scripts {
		if b.opts.Verbose {
			log.Printf("Executing script %d/%d", i+1, len(scripts))
		}

		result, err := b.ExecuteScript(script)
		if err != nil {
			return results, withField(scriptError(fmt.Sprintf("failed to execute script %d", i+1), err), "script_index", i+1)
		}

		results[i] = result
	}

	return results, nil
}

// ExecuteScriptsWithTimeout runs multiple JavaScript scripts with individual timeouts
func (b *Browser) ExecuteScriptsWithTimeout(scripts []string, timeout time.Duration) ([]interface{}, error) {
	if b.ctx == nil {
		return nil, notLaunchedError()
	}

	if len(scripts) == 0 {
		return []interface{}{}, nil
	}

	results := make([]interface{}, len(scripts))

	for i, script := range scripts {
		if b.opts.Verbose {
			log.Printf("Executing script %d/%d with timeout %v", i+1, len(scripts), timeout)
		}

		result, err := b.ExecuteScriptWithTimeout(script, timeout)
		if err != nil {
			return results, withField(scriptError(fmt.Sprintf("failed to execute script %d with timeout", i+1), err), "script_index", i+1)
		}

		results[i] = result
	}

	return results, nil
}

// executeScriptsBefore executes all pre-navigation scripts
func (b *Browser) executeScriptsBefore() error {
	if len(b.opts.ScriptBefore) == 0 {
		return nil
	}

	if b.opts.Verbose {
		log.Printf("Executing %d pre-navigation scripts", len(b.opts.ScriptBefore))
	}

	// Use a shorter timeout for pre-navigation scripts to avoid blocking navigation
	timeout := 5 * time.Second

	_, err := b.ExecuteScriptsWithTimeout(b.opts.ScriptBefore, timeout)
	if err != nil {
		return fmt.Errorf("failed to execute pre-navigation scripts: %w", err)
	}

	if b.opts.Verbose {
		log.Printf("Successfully executed all pre-navigation scripts")
	}

	return nil
}

// executeScriptsAfter executes all post-navigation scripts
func (b *Browser) executeScriptsAfter() error {
	if len(b.opts.ScriptAfter) == 0 {
		return nil
	}

	if b.opts.Verbose {
		log.Printf("Executing %d post-navigation scripts", len(b.opts.ScriptAfter))
	}

	// Use a longer timeout for post-navigation scripts as they may interact with content
	timeout := 10 * time.Second

	_, err := b.ExecuteScriptsWithTimeout(b.opts.ScriptAfter, timeout)
	if err != nil {
		return fmt.Errorf("failed to execute post-navigation scripts: %w", err)
	}

	if b.opts.Verbose {
		log.Printf("Successfully executed all post-navigation scripts")
	}

	return nil
}

// GetTitle returns the page title
func (b *Browser) GetTitle() (string, error) {
	if b.ctx == nil {
		return "", notLaunchedError()
	}

	var title string
	if err := chromedp.Run(b.ctx, chromedp.Title(&title)); err != nil {
		return "", scriptError("failed to get page title", err)
	}

	return title, nil
}

// GetURL returns the current page URL
func (b *Browser) GetURL() (string, error) {
	if b.ctx == nil {
		return "", notLaunchedError()
	}

	var url string
	if err := chromedp.Run(b.ctx, chromedp.Location(&url)); err != nil {
		return "", scriptError("failed to get page URL", err)
	}

	return url, nil
}

// SetRequestHeaders sets custom headers for all subsequent requests
func (b *Browser) SetRequestHeaders(headers map[string]string) error {
	if b.ctx == nil {
		return notLaunchedError()
	}

	// Convert to the format expected by CDP
	cdpHeaders := make(map[string]interface{})
	for k, v := range headers {
		cdpHeaders[k] = v
	}

	if err := chromedp.Run(b.ctx, network.SetExtraHTTPHeaders(network.Headers(cdpHeaders))); err != nil {
		return networkError("failed to set request headers", err)
	}
	return nil
}

// SetBasicAuth sets basic authentication headers
func (b *Browser) SetBasicAuth(username, password string) error {
	auth := username + ":" + password
	encodedAuth := base64.StdEncoding.EncodeToString([]byte(auth))
	return b.SetRequestHeaders(map[string]string{
		"Authorization": "Basic " + encodedAuth,
	})
}

// detectContentType attempts to detect the content type based on the data
func detectContentType(data string, headers map[string]string) string {
	// Check if Content-Type is already set in headers
	for k, v := range headers {
		if strings.ToLower(k) == "content-type" {
			return v
		}
	}

	// Auto-detect based on data format
	data = strings.TrimSpace(data)
	if data == "" {
		return "text/plain"
	}

	// Check for JSON
	if (strings.HasPrefix(data, "{") && strings.HasSuffix(data, "}")) ||
		(strings.HasPrefix(data, "[") && strings.HasSuffix(data, "]")) {
		return "application/json"
	}

	// Check for URL-encoded form data
	if strings.Contains(data, "=") && (strings.Contains(data, "&") || !strings.Contains(data, " ")) {
		return "application/x-www-form-urlencoded"
	}

	// Default to plain text
	return "text/plain"
}

// HTTPRequest performs an HTTP request with the specified method and data
func (b *Browser) HTTPRequest(method, url, data string, headers map[string]string) error {
	if b.ctx == nil {
		return notLaunchedError()
	}

	if b.opts.Verbose {
		log.Printf("Making %s request to: %s", method, url)
		if data != "" {
			log.Printf("Request data: %s", data)
		}
	}

	// Normalize method to uppercase
	method = strings.ToUpper(method)

	// For GET requests without data, use the regular Navigate method
	if method == "GET" && data == "" {
		return b.Navigate(url)
	}

	// Set up request interception for POST/PUT requests
	requestData := &HTTPRequestData{
		Method:      method,
		URL:         url,
		Data:        data,
		Headers:     headers,
		ContentType: detectContentType(data, headers),
	}

	// Enable request interception
	if err := b.enableRequestInterception(requestData); err != nil {
		return withField(networkError("failed to enable request interception", err), "method", method)
	}

	// Execute pre-navigation scripts before making the request
	if err := b.executeScriptsBefore(); err != nil {
		if b.opts.Verbose {
			log.Printf("Pre-navigation script error: %v", err)
		}
		return fmt.Errorf("executing pre-navigation scripts: %w", err)
	}

	// Navigate to the URL (this will trigger our interceptor)
	navTimeout := time.Duration(b.opts.Timeout) * time.Second
	if navTimeout <= 0 {
		navTimeout = 60 * time.Second // Default fallback
	}
	navCtx, navCancel := context.WithTimeout(b.ctx, navTimeout)
	defer navCancel()

	if err := chromedp.Run(navCtx, chromedp.Navigate(url)); err != nil {
		return withField(navigationError("failed to navigate with custom method", err), "method", method)
	}

	// Wait for network idle if requested (similar to Navigate method)
	if b.opts.WaitNetworkIdle {
		waitTimeout := time.Duration(b.opts.StableTimeout) * time.Second
		waitCtx, waitCancel := context.WithTimeout(b.ctx, waitTimeout)
		defer waitCancel()

		if err := chromedp.Run(waitCtx, chromedp.ActionFunc(func(ctx context.Context) error {
			// This will wait until there are no more than 2 network connections for at least 500ms
			ch := make(chan struct{})
			lctx, cancel := context.WithCancel(ctx)
			chromedp.ListenTarget(lctx, func(ev interface{}) {
				switch ev.(type) {
				case *network.EventLoadingFinished, *network.EventLoadingFailed:
					select {
					case ch <- struct{}{}:
					default:
					}
				}
			})

			// Wait for idle using a timer
			idleTimer := time.NewTimer(500 * time.Millisecond)
			defer cancel()

			for {
				select {
				case <-waitCtx.Done():
					return waitCtx.Err()
				case <-idleTimer.C:
					// We've been idle for 500ms
					return nil
				case <-ch:
					// Reset the timer when any network event occurs
					if !idleTimer.Stop() {
						<-idleTimer.C
					}
					idleTimer.Reset(500 * time.Millisecond)
				}
			}
		})); err != nil {
			if b.opts.Verbose {
				log.Printf("Warning: failed to wait for network idle: %v", err)
			}
		}
	}

	// Wait for specific selector if requested
	if b.opts.WaitSelector != "" {
		selectorTimeout := time.Duration(b.opts.StableTimeout) * time.Second
		waitCtx, waitCancel := context.WithTimeout(b.ctx, selectorTimeout)
		defer waitCancel()

		if err := chromedp.Run(waitCtx, chromedp.WaitVisible(b.opts.WaitSelector, chromedp.ByQuery)); err != nil {
			if b.opts.Verbose {
				log.Printf("Warning: failed to wait for selector: %v", err)
			}
		}
	}

	// Execute post-navigation scripts
	if err := b.executeScriptsAfter(); err != nil {
		if b.opts.Verbose {
			log.Printf("Post-navigation script error: %v", err)
		}
		return fmt.Errorf("executing post-navigation scripts: %w", err)
	}

	// Disable request interception after the request
	if err := b.disableRequestInterception(); err != nil {
		// Log the error but don't fail the request
		if b.opts.Verbose {
			log.Printf("Warning: failed to disable request interception: %v", err)
		}
	}

	return nil
}

// enableRequestInterception sets up request interception for custom HTTP methods
func (b *Browser) enableRequestInterception(requestData *HTTPRequestData) error {
	b.interceptor.mu.Lock()
	b.interceptor.targetURL = requestData.URL
	b.interceptor.requestData = requestData
	b.interceptor.intercepting = true
	b.interceptor.mu.Unlock()

	// Enable network events
	if err := chromedp.Run(b.ctx, network.Enable()); err != nil {
		return networkError("failed to enable network events", err)
	}

	// Enable fetch domain for request interception with patterns
	if err := chromedp.Run(b.ctx,
		fetch.Enable().WithPatterns([]*fetch.RequestPattern{{URLPattern: "*"}})); err != nil {
		return networkError("failed to enable fetch domain", err)
	}

	// Set up the request interceptor
	chromedp.ListenTarget(b.ctx, func(ev interface{}) {
		switch e := ev.(type) {
		case *fetch.EventRequestPaused:
			go b.handleInterceptedRequest(e)
		}
	})

	if b.opts.Verbose {
		log.Printf("Request interception enabled for %s", requestData.URL)
	}

	return nil
}

// disableRequestInterception disables request interception
func (b *Browser) disableRequestInterception() error {
	b.interceptor.mu.Lock()
	b.interceptor.intercepting = false
	b.interceptor.targetURL = ""
	b.interceptor.requestData = nil
	b.interceptor.mu.Unlock()

	// Disable fetch domain
	if err := chromedp.Run(b.ctx, fetch.Disable()); err != nil {
		return networkError("failed to disable fetch domain", err)
	}

	if b.opts.Verbose {
		log.Printf("Request interception disabled")
	}

	return nil
}

// handleInterceptedRequest processes intercepted requests
func (b *Browser) handleInterceptedRequest(ev *fetch.EventRequestPaused) {
	b.interceptor.mu.RLock()
	intercepting := b.interceptor.intercepting
	targetURL := b.interceptor.targetURL
	requestData := b.interceptor.requestData
	b.interceptor.mu.RUnlock()

	if !intercepting || requestData == nil {
		// Continue the request as-is
		if err := chromedp.Run(b.ctx, fetch.ContinueRequest(ev.RequestID)); err != nil && b.opts.Verbose {
			log.Printf("Error continuing unmodified request: %v", err)
		}
		return
	}

	// Check if this is the request we want to modify
	requestURL := ev.Request.URL
	if !b.shouldInterceptRequest(requestURL, targetURL) {
		// Continue the request as-is
		if err := chromedp.Run(b.ctx, fetch.ContinueRequest(ev.RequestID)); err != nil && b.opts.Verbose {
			log.Printf("Error continuing request: %v", err)
		}
		return
	}

	if b.opts.Verbose {
		log.Printf("Intercepting request to %s, modifying to %s %s", requestURL, requestData.Method, requestData.URL)
	}

	// Build the modified request using fetch.ContinueRequest with minimal modifications
	continueParams := fetch.ContinueRequest(ev.RequestID)

	if b.opts.Verbose {
		log.Printf("DEBUG: Original method: %s, Target method: %s", ev.Request.Method, requestData.Method)
	}

	// For now, only modify the method due to Brave compatibility issues with WithPostData
	continueParams = continueParams.WithMethod(requestData.Method)

	// TODO: WithPostData causes "Invalid parameters (-32602)" error in Brave browser
	// This is a known compatibility issue that needs further investigation
	if requestData.Data != "" && b.opts.Verbose {
		log.Printf("WARNING: POST data not sent due to Brave compatibility issue with WithPostData")
		log.Printf("POST data would be: %s", requestData.Data)
	}

	// Continue with the modified request
	if err := chromedp.Run(b.ctx, continueParams); err != nil {
		if b.opts.Verbose {
			log.Printf("Error continuing modified request: %v", err)
		}
		// Fall back to continuing without modification
		if fallbackErr := chromedp.Run(b.ctx, fetch.ContinueRequest(ev.RequestID)); fallbackErr != nil && b.opts.Verbose {
			log.Printf("Error in fallback continue: %v", fallbackErr)
		}
	}
}

// shouldInterceptRequest determines if a request should be intercepted and modified
func (b *Browser) shouldInterceptRequest(requestURL, targetURL string) bool {
	// Simple URL matching - exact match or base URL match
	if requestURL == targetURL {
		return true
	}

	// Check if the request URL starts with the target URL (for redirects)
	if strings.HasPrefix(requestURL, targetURL) {
		return true
	}

	// For more sophisticated matching, we could add URL parsing here
	return false
}

// setupProxyAuthentication configures proxy authentication using Fetch domain
func (b *Browser) setupProxyAuthentication() error {
	if b.opts.Verbose {
		log.Printf("Setting up proxy authentication for user: %s", b.opts.ProxyUsername)
	}

	// Enable network domain to intercept auth challenges
	if err := chromedp.Run(b.ctx, network.Enable()); err != nil {
		return networkError("failed to enable network domain for proxy auth", err)
	}

	// Enable fetch domain for handling authentication
	if err := chromedp.Run(b.ctx, fetch.Enable()); err != nil {
		return networkError("failed to enable fetch domain for proxy auth", err)
	}

	// Listen for auth required events
	chromedp.ListenTarget(b.ctx, func(ev interface{}) {
		switch e := ev.(type) {
		case *fetch.EventAuthRequired:
			go b.handleProxyAuthChallenge(e)
		}
	})

	return nil
}

// handleProxyAuthChallenge responds to proxy authentication challenges
func (b *Browser) handleProxyAuthChallenge(ev *fetch.EventAuthRequired) {
	if b.opts.Verbose {
		log.Printf("Handling proxy auth challenge for %s", ev.AuthChallenge.Origin)
	}

	// Only handle proxy auth challenges
	if ev.AuthChallenge.Source != fetch.AuthChallengeSourceProxy {
		// Continue without providing credentials for non-proxy challenges
		if err := chromedp.Run(b.ctx, fetch.ContinueWithAuth(ev.RequestID, &fetch.AuthChallengeResponse{
			Response: fetch.AuthChallengeResponseResponseDefault,
		})); err != nil && b.opts.Verbose {
			log.Printf("Error continuing without auth: %v", err)
		}
		return
	}

	// Provide proxy credentials
	authResponse := &fetch.AuthChallengeResponse{
		Response: fetch.AuthChallengeResponseResponseProvideCredentials,
		Username: b.opts.ProxyUsername,
		Password: b.opts.ProxyPassword,
	}

	if err := chromedp.Run(b.ctx, fetch.ContinueWithAuth(ev.RequestID, authResponse)); err != nil {
		if b.opts.Verbose {
			log.Printf("Error providing proxy credentials: %v", err)
		}
	} else if b.opts.Verbose {
		log.Printf("Successfully provided proxy credentials")
	}
}

// setupNetworkBlocking configures network interception for blocking requests
func (b *Browser) setupNetworkBlocking() error {
	if b.opts.Verbose {
		log.Printf("Setting up network blocking...")
	}

	// Enable network domain
	if err := chromedp.Run(b.ctx, network.Enable()); err != nil {
		return fmt.Errorf("enabling network domain for blocking: %w", err)
	}

	// Enable fetch domain for request interception
	if err := chromedp.Run(b.ctx, fetch.Enable()); err != nil {
		return fmt.Errorf("enabling fetch domain for blocking: %w", err)
	}

	// Set up the request interceptor
	chromedp.ListenTarget(b.ctx, func(ev interface{}) {
		switch e := ev.(type) {
		case *fetch.EventRequestPaused:
			go b.handleBlockingRequest(e)
		}
	})

	if b.opts.Verbose {
		log.Printf("Network blocking enabled")
	}

	return nil
}

// handleBlockingRequest processes intercepted requests for blocking
func (b *Browser) handleBlockingRequest(ev *fetch.EventRequestPaused) {
	// Check if the request should be blocked
	if b.blockingEngine != nil && b.blockingEngine.ShouldBlock(ev.Request.URL) {
		// Block the request
		if err := chromedp.Run(b.ctx, fetch.FailRequest(ev.RequestID, network.ErrorReasonAccessDenied)); err != nil && b.opts.Verbose {
			log.Printf("Error blocking request %s: %v", ev.Request.URL, err)
		}
		return
	}

	// Continue the request as normal
	if err := chromedp.Run(b.ctx, fetch.ContinueRequest(ev.RequestID)); err != nil && b.opts.Verbose {
		log.Printf("Error continuing request %s: %v", ev.Request.URL, err)
	}
}

// GetBlockingStats returns blocking statistics
func (b *Browser) GetBlockingStats() (processed, blocked int64) {
	if b.blockingEngine == nil {
		return 0, 0
	}
	return b.blockingEngine.GetStats()
}

// BlockingEngine returns the blocking engine (if any)
func (b *Browser) BlockingEngine() *blocking.BlockingEngine {
	return b.blockingEngine
}

// BlockURLPattern adds a URL pattern to block. Creates a blocking engine if none exists.
func (b *Browser) BlockURLPattern(pattern string) error {
	if b.blockingEngine == nil {
		// Create a new blocking engine with this pattern
		config := &blocking.Config{
			Enabled:     true,
			URLPatterns: []string{pattern},
			Verbose:     b.opts.Verbose,
		}
		var err error
		b.blockingEngine, err = blocking.NewBlockingEngine(config)
		if err != nil {
			return fmt.Errorf("creating blocking engine: %w", err)
		}
		// Setup network blocking
		return b.setupNetworkBlocking()
	}

	// Add pattern to existing engine's config and rebuild
	b.blockingEngine = nil
	config := &blocking.Config{
		Enabled:     true,
		URLPatterns: []string{pattern},
		Verbose:     b.opts.Verbose,
	}
	var err error
	b.blockingEngine, err = blocking.NewBlockingEngine(config)
	if err != nil {
		return fmt.Errorf("recreating blocking engine: %w", err)
	}
	return nil
}
