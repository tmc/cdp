package targets

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"
)

// TargetInfo represents a debugging target from the JSON API
type TargetInfo struct {
	ID                        string `json:"id"`
	Type                      string `json:"type"`
	Title                     string `json:"title"`
	URL                       string `json:"url"`
	Description               string `json:"description"`
	FaviconURL                string `json:"faviconUrl"`
	DevtoolsFrontendURL       string `json:"devtoolsFrontendUrl"`
	DevtoolsFrontendURLCompat string `json:"devtoolsFrontendUrlCompat"`
	WebSocketDebuggerURL      string `json:"webSocketDebuggerUrl"`
	Port                      int    `json:"-"`
}

// VersionInfo represents the version information from /json/version
type VersionInfo struct {
	Browser         string `json:"Browser"`
	ProtocolVersion string `json:"Protocol-Version"`
	UserAgent       string `json:"User-Agent,omitempty"`
	V8Version       string `json:"V8-Version,omitempty"`
	WebKitVersion   string `json:"WebKit-Version,omitempty"`
}

// Discovery implements Chrome-style target discovery using HTTP JSON endpoints
type Discovery struct {
	ports   []int
	timeout time.Duration
	client  *http.Client
}

// NewDiscovery creates a new target discovery instance
func NewDiscovery(timeout time.Duration) *Discovery {
	return &Discovery{
		ports:   []int{9229, 9230, 9231, 9232, 9233, 9234, 9235, 9236, 9237, 9238, 9222, 9223, 9224},
		timeout: timeout,
		client: &http.Client{
			Timeout: timeout,
		},
	}
}

// SetPorts sets the ports scanned for debugging targets.
func (d *Discovery) SetPorts(ports []int) {
	d.ports = ports
}

// DiscoverTargets discovers all available debugging targets using Chrome's method
func (d *Discovery) DiscoverTargets(ctx context.Context) ([]TargetInfo, error) {
	var mu sync.Mutex
	var targets []TargetInfo
	var wg sync.WaitGroup

	// Scan all ports concurrently like Chrome does
	for _, port := range d.ports {
		wg.Add(1)
		go func(p int) {
			defer wg.Done()

			portTargets, err := d.discoverPort(ctx, p)
			if err == nil && len(portTargets) > 0 {
				mu.Lock()
				targets = append(targets, portTargets...)
				mu.Unlock()
			}
		}(port)
	}

	wg.Wait()
	return targets, nil
}

// DiscoverPort discovers targets on a specific port.
func (d *Discovery) DiscoverPort(ctx context.Context, port int) ([]TargetInfo, error) {
	return d.discoverPort(ctx, port)
}

// discoverPort discovers targets on a specific port (internal implementation)
func (d *Discovery) discoverPort(ctx context.Context, port int) ([]TargetInfo, error) {
	// First check if anything is listening by trying to get version info
	versionURL := fmt.Sprintf("http://localhost:%d/json/version", port)

	req, err := http.NewRequestWithContext(ctx, "GET", versionURL, nil)
	if err != nil {
		return nil, err
	}

	resp, err := d.client.Do(req)
	if err != nil {
		return nil, err // No service on this port
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	// Parse version info to determine what kind of debugger this is
	var version VersionInfo
	if err := json.NewDecoder(resp.Body).Decode(&version); err != nil {
		return nil, fmt.Errorf("failed to parse version info: %w", err)
	}

	// Now get the list of targets
	listURL := fmt.Sprintf("http://localhost:%d/json/list", port)

	req, err = http.NewRequestWithContext(ctx, "GET", listURL, nil)
	if err != nil {
		return nil, err
	}

	resp, err = d.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code for list: %d", resp.StatusCode)
	}

	var targets []TargetInfo
	if err := json.NewDecoder(resp.Body).Decode(&targets); err != nil {
		return nil, fmt.Errorf("failed to parse targets: %w", err)
	}

	// Add port information to each target
	for i := range targets {
		targets[i].Port = port
	}

	return targets, nil
}

// GetVersion gets version information for a specific port.
func (d *Discovery) GetVersion(ctx context.Context, port int) (*VersionInfo, error) {
	versionURL := fmt.Sprintf("http://localhost:%d/json/version", port)

	req, err := http.NewRequestWithContext(ctx, "GET", versionURL, nil)
	if err != nil {
		return nil, err
	}

	resp, err := d.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var version VersionInfo
	if err := json.NewDecoder(resp.Body).Decode(&version); err != nil {
		return nil, fmt.Errorf("failed to parse version info: %w", err)
	}

	return &version, nil
}

// IsNodeTarget returns true if the target is a Node.js debugging target
func IsNodeTarget(target TargetInfo) bool {
	return target.Type == "node"
}

// IsChromeTarget returns true if the target is a Chrome/browser debugging target
func IsChromeTarget(target TargetInfo) bool {
	return target.Type == "page" || target.Type == "background_page" || target.Type == "service_worker"
}

// FilterNodeTargets filters targets to only include Node.js targets
func FilterNodeTargets(targets []TargetInfo) []TargetInfo {
	var nodeTargets []TargetInfo
	for _, target := range targets {
		if IsNodeTarget(target) {
			nodeTargets = append(nodeTargets, target)
		}
	}
	return nodeTargets
}

// FilterChromeTargets filters targets to only include Chrome/browser targets
func FilterChromeTargets(targets []TargetInfo) []TargetInfo {
	var chromeTargets []TargetInfo
	for _, target := range targets {
		if IsChromeTarget(target) {
			chromeTargets = append(chromeTargets, target)
		}
	}
	return chromeTargets
}
