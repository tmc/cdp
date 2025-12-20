package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/chromedp/cdproto/fetch"
	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/cdproto/runtime"
	"github.com/chromedp/chromedp"
	"github.com/pkg/errors"
)

// NetworkController handles network operations
type NetworkController struct {
	debugger         *ChromeDebugger
	verbose          bool
	interceptEnabled bool
	blockedPatterns  []string
	requestMap       map[network.RequestID]*network.Request
}

// NewNetworkController creates a new network controller
func NewNetworkController(debugger *ChromeDebugger, verbose bool) *NetworkController {
	return &NetworkController{
		debugger:    debugger,
		verbose:     verbose,
		requestMap:  make(map[network.RequestID]*network.Request),
	}
}

// StartMonitoring starts monitoring network activity
func (nc *NetworkController) StartMonitoring(ctx context.Context, duration time.Duration) error {
	if !nc.debugger.connected {
		return errors.New("not connected to Chrome")
	}

	fmt.Println("Starting network monitoring...")

	err := chromedp.Run(nc.debugger.chromeCtx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			// Enable network domain
			if err := network.Enable().Do(ctx); err != nil {
				return err
			}

			// Set up event listeners
			nc.setupNetworkEventListeners()

			return nil
		}),
	)

	if err != nil {
		return errors.Wrap(err, "failed to start network monitoring")
	}

	// Monitor for specified duration or until interrupted
	if duration > 0 {
		fmt.Printf("Monitoring for %s...\n", duration)
		select {
		case <-time.After(duration):
			fmt.Println("Monitoring duration completed")
		case <-ctx.Done():
			fmt.Println("Monitoring interrupted")
		}
	} else {
		fmt.Println("Monitoring indefinitely (press Ctrl+C to stop)...")
		<-ctx.Done()
	}

	return nil
}

// EnableInterception enables request interception
func (nc *NetworkController) EnableInterception(ctx context.Context, patterns []string) error {
	if !nc.debugger.connected {
		return errors.New("not connected to Chrome")
	}

	nc.blockedPatterns = patterns

	err := chromedp.Run(nc.debugger.chromeCtx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			// Enable fetch domain for interception
			if err := fetch.Enable().Do(ctx); err != nil {
				return err
			}

			// Enable network domain
			if err := network.Enable().Do(ctx); err != nil {
				return err
			}

			// Set request patterns for interception
			var requestPatterns []*fetch.RequestPattern
			for _, pattern := range patterns {
				requestPatterns = append(requestPatterns, &fetch.RequestPattern{
					URLPattern: pattern,
				})
			}

			// Enable interception
			if err := fetch.Enable().WithPatterns(requestPatterns).Do(ctx); err != nil {
				return err
			}

			nc.interceptEnabled = true
			nc.setupInterceptionListeners()

			return nil
		}),
	)

	if err != nil {
		return errors.Wrap(err, "failed to enable interception")
	}

	fmt.Printf("✓ Interception enabled for patterns: %v\n", patterns)
	return nil
}

// BlockRequests blocks requests matching the given patterns
func (nc *NetworkController) BlockRequests(ctx context.Context, patterns []string) error {
	if !nc.debugger.connected {
		return errors.New("not connected to Chrome")
	}

	err := chromedp.Run(nc.debugger.chromeCtx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			// Enable network domain
			if err := network.Enable().Do(ctx); err != nil {
				return err
			}

			// Use JavaScript to block URLs since CDP blocking may not be available
			// This is a simplified implementation
			patternsJSON, _ := json.Marshal(patterns)
			blockScript := fmt.Sprintf(`
				(function() {
					const patterns = %s;
					const originalFetch = window.fetch;
					window.fetch = function(url, options) {
						for (const pattern of patterns) {
							if (url.includes(pattern)) {
								console.log('Blocked request to:', url);
								return Promise.reject(new Error('Blocked by CHDB'));
							}
						}
						return originalFetch.apply(this, arguments);
					};
					return 'URL blocking enabled';
				})()
			`, string(patternsJSON))
			_, _, err := runtime.Evaluate(blockScript).Do(ctx)
			return err
		}),
	)

	if err != nil {
		return errors.Wrap(err, "failed to block requests")
	}

	fmt.Printf("✓ Blocked request patterns: %v\n", patterns)
	return nil
}

// SetThrottling applies network throttling
func (nc *NetworkController) SetThrottling(ctx context.Context, profile string) error {
	if !nc.debugger.connected {
		return errors.New("not connected to Chrome")
	}

	var downloadThroughput, uploadThroughput, latency float64

	// Define common throttling profiles
	switch strings.ToLower(profile) {
	case "offline":
		downloadThroughput = 0
		uploadThroughput = 0
		latency = 0
	case "slow-3g":
		downloadThroughput = 400 * 1024 / 8    // 400 Kbps
		uploadThroughput = 400 * 1024 / 8     // 400 Kbps
		latency = 400
	case "fast-3g":
		downloadThroughput = 1.6 * 1024 * 1024 / 8  // 1.6 Mbps
		uploadThroughput = 750 * 1024 / 8            // 750 Kbps
		latency = 150
	case "4g":
		downloadThroughput = 4 * 1024 * 1024 / 8    // 4 Mbps
		uploadThroughput = 3 * 1024 * 1024 / 8      // 3 Mbps
		latency = 20
	case "none", "disabled":
		// Disable throttling
		err := chromedp.Run(nc.debugger.chromeCtx,
			chromedp.ActionFunc(func(ctx context.Context) error {
				return network.EmulateNetworkConditions(false, 0, 0, 0).Do(ctx)
			}),
		)
		if err == nil {
			fmt.Println("✓ Network throttling disabled")
		}
		return err
	default:
		return fmt.Errorf("unknown throttling profile: %s (available: offline, slow-3g, fast-3g, 4g, none)", profile)
	}

	err := chromedp.Run(nc.debugger.chromeCtx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			offline := (downloadThroughput == 0)
			return network.EmulateNetworkConditions(
				offline,
				latency,
				downloadThroughput,
				uploadThroughput,
			).Do(ctx)
		}),
	)

	if err != nil {
		return errors.Wrap(err, "failed to set network throttling")
	}

	fmt.Printf("✓ Network throttling set to: %s\n", profile)
	return nil
}

// ModifyRequest modifies a request before it's sent
func (nc *NetworkController) ModifyRequest(ctx context.Context, requestID string, headers map[string]string, body string) error {
	if !nc.debugger.connected {
		return errors.New("not connected to Chrome")
	}

	if !nc.interceptEnabled {
		return errors.New("interception not enabled")
	}

	err := chromedp.Run(nc.debugger.chromeCtx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			// Continue request with modifications
			continueReq := fetch.ContinueRequest(fetch.RequestID(requestID))

			if len(headers) > 0 {
				var headerList []*fetch.HeaderEntry
				for name, value := range headers {
					headerList = append(headerList, &fetch.HeaderEntry{
						Name:  name,
						Value: value,
					})
				}
				continueReq = continueReq.WithHeaders(headerList)
			}

			if body != "" {
				continueReq = continueReq.WithPostData(body)
			}

			return continueReq.Do(ctx)
		}),
	)

	if err != nil {
		return errors.Wrap(err, "failed to modify request")
	}

	fmt.Printf("✓ Modified request: %s\n", requestID)
	return nil
}

// GetHAR exports network activity as HAR
func (nc *NetworkController) GetHAR(ctx context.Context) (string, error) {
	if !nc.debugger.connected {
		return "", errors.New("not connected to Chrome")
	}

	// Use JavaScript to access Chrome's HAR export capability
	expression := `
		(function() {
			// This is a simplified HAR structure
			// In practice, you'd collect this data from network events
			const har = {
				log: {
					version: "1.2",
					creator: {
						name: "CHDB",
						version: "1.0.0"
					},
					entries: []
				}
			};

			// Access performance timeline for network entries
			const perfEntries = performance.getEntriesByType('navigation')
				.concat(performance.getEntriesByType('resource'));

			perfEntries.forEach(entry => {
				if (entry.name) {
					har.log.entries.push({
						startedDateTime: new Date(entry.startTime).toISOString(),
						time: entry.duration || 0,
						request: {
							method: "GET", // Simplified
							url: entry.name,
							httpVersion: "HTTP/1.1",
							headers: [],
							queryString: [],
							cookies: [],
							headersSize: -1,
							bodySize: -1
						},
						response: {
							status: 200, // Simplified
							statusText: "OK",
							httpVersion: "HTTP/1.1",
							headers: [],
							cookies: [],
							content: {
								size: entry.transferSize || 0,
								mimeType: "text/html"
							},
							redirectURL: "",
							headersSize: -1,
							bodySize: entry.transferSize || 0
						},
						cache: {},
						timings: {
							blocked: -1,
							dns: entry.domainLookupEnd - entry.domainLookupStart || -1,
							connect: entry.connectEnd - entry.connectStart || -1,
							send: 0,
							wait: entry.responseStart - entry.requestStart || 0,
							receive: entry.responseEnd - entry.responseStart || 0,
							ssl: entry.secureConnectionStart ? entry.connectEnd - entry.secureConnectionStart : -1
						}
					});
				}
			});

			return JSON.stringify(har, null, 2);
		})()
	`

	var result string
	err := chromedp.Run(nc.debugger.chromeCtx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			res, _, err := runtime.Evaluate(expression).Do(ctx)
			if err != nil {
				return err
			}

			if res.Value != nil {
				var harData interface{}
				if json.Unmarshal(res.Value, &harData) == nil {
					if harStr, ok := harData.(string); ok {
						result = harStr
					}
				}
			}

			return nil
		}),
	)

	if err != nil {
		return "", errors.Wrap(err, "failed to export HAR")
	}

	return result, nil
}

// setupNetworkEventListeners sets up listeners for network events
func (nc *NetworkController) setupNetworkEventListeners() {
	chromedp.ListenTarget(nc.debugger.chromeCtx, func(ev interface{}) {
		switch e := ev.(type) {
		case *network.EventRequestWillBeSent:
			nc.handleRequestWillBeSent(e)
		case *network.EventResponseReceived:
			nc.handleResponseReceived(e)
		case *network.EventLoadingFinished:
			nc.handleLoadingFinished(e)
		case *network.EventLoadingFailed:
			nc.handleLoadingFailed(e)
		}
	})
}

// setupInterceptionListeners sets up listeners for request interception
func (nc *NetworkController) setupInterceptionListeners() {
	chromedp.ListenTarget(nc.debugger.chromeCtx, func(ev interface{}) {
		switch e := ev.(type) {
		case *fetch.EventRequestPaused:
			nc.handleRequestPaused(e)
		}
	})
}

// handleRequestWillBeSent handles request events
func (nc *NetworkController) handleRequestWillBeSent(ev *network.EventRequestWillBeSent) {
	nc.requestMap[ev.RequestID] = ev.Request

	if nc.verbose {
		fmt.Printf("🌐 Request: %s %s\n", ev.Request.Method, ev.Request.URL)
	}
}

// handleResponseReceived handles response events
func (nc *NetworkController) handleResponseReceived(ev *network.EventResponseReceived) {
	if nc.verbose {
		fmt.Printf("📥 Response: %d %s (Type: %s)\n",
			ev.Response.Status,
			ev.Response.URL,
			ev.Type)
	}
}

// handleLoadingFinished handles completed requests
func (nc *NetworkController) handleLoadingFinished(ev *network.EventLoadingFinished) {
	if nc.verbose {
		fmt.Printf("✅ Finished: %s\n", ev.RequestID)
	}
}

// handleLoadingFailed handles failed requests
func (nc *NetworkController) handleLoadingFailed(ev *network.EventLoadingFailed) {
	if nc.verbose {
		fmt.Printf("❌ Failed: %s - %s\n", ev.RequestID, ev.ErrorText)
	}
}

// handleRequestPaused handles intercepted requests
func (nc *NetworkController) handleRequestPaused(ev *fetch.EventRequestPaused) {
	// Check if request should be blocked
	shouldBlock := false
	for _, pattern := range nc.blockedPatterns {
		if strings.Contains(ev.Request.URL, pattern) {
			shouldBlock = true
			break
		}
	}

	if shouldBlock {
		// Block the request
		chromedp.Run(nc.debugger.chromeCtx,
			chromedp.ActionFunc(func(ctx context.Context) error {
				return fetch.FailRequest(ev.RequestID, network.ErrorReasonBlockedByClient).Do(ctx)
			}),
		)
		if nc.verbose {
			fmt.Printf("🚫 Blocked: %s\n", ev.Request.URL)
		}
	} else {
		// Continue the request
		chromedp.Run(nc.debugger.chromeCtx,
			chromedp.ActionFunc(func(ctx context.Context) error {
				return fetch.ContinueRequest(ev.RequestID).Do(ctx)
			}),
		)
	}
}