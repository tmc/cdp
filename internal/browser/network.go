package browser

import (
	"context"
	"encoding/base64"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"

	"errors"

	"github.com/chromedp/cdproto/fetch"
	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"
	"github.com/tmc/cdp/internal/blocking"
)

// Request represents an intercepted network request
type Request struct {
	ID       string
	URL      string
	Method   string
	Headers  map[string]string
	PostData string

	page        *Page
	requestID   fetch.RequestID
	intercepted bool
	mu          sync.Mutex
}

// Response represents a network response
type Response struct {
	URL        string
	Status     int
	StatusText string
	Headers    map[string]string
	Body       []byte
}

// Route represents a network route handler
type Route struct {
	pattern *regexp.Regexp
	handler RouteHandler
}

// RouteHandler handles intercepted requests
type RouteHandler func(*Request) error

// NetworkManager manages network interception and monitoring
type NetworkManager struct {
	page    *Page
	routes  []Route
	enabled bool
	monitor bool
	mu      sync.RWMutex

	// Request tracking
	requests  map[network.RequestID]*Request
	responses map[network.RequestID]*Response

	// Blocking engine
	blockingEngine *blocking.BlockingEngine
}

// NewNetworkManager creates a new network manager
func NewNetworkManager(page *Page) *NetworkManager {
	return &NetworkManager{
		page:      page,
		routes:    make([]Route, 0),
		requests:  make(map[network.RequestID]*Request),
		responses: make(map[network.RequestID]*Response),
	}
}

// NewNetworkManagerWithBlocking creates a new network manager with blocking support
func NewNetworkManagerWithBlocking(page *Page, blockingEngine *blocking.BlockingEngine) *NetworkManager {
	return &NetworkManager{
		page:           page,
		routes:         make([]Route, 0),
		requests:       make(map[network.RequestID]*Request),
		responses:      make(map[network.RequestID]*Response),
		blockingEngine: blockingEngine,
	}
}

// Enable enables network interception
func (nm *NetworkManager) Enable() error {
	nm.mu.Lock()
	defer nm.mu.Unlock()

	if nm.enabled {
		return nil
	}

	actions := []chromedp.Action{fetch.Enable()}
	if !nm.monitor {
		actions = append([]chromedp.Action{network.Enable()}, actions...)
	}
	if err := chromedp.Run(nm.page.ctx, actions...); err != nil {
		return fmt.Errorf("enabling network interception: %w", err)
	}

	if !nm.monitor {
		chromedp.ListenTarget(nm.page.ctx, nm.handleNetworkEvent)
		nm.monitor = true
	}

	nm.enabled = true
	return nil
}

// Monitor enables passive network event tracking.
func (nm *NetworkManager) Monitor() error {
	nm.mu.Lock()
	defer nm.mu.Unlock()

	if nm.monitor {
		return nil
	}

	if err := chromedp.Run(nm.page.ctx, network.Enable()); err != nil {
		return fmt.Errorf("enabling network monitor: %w", err)
	}
	chromedp.ListenTarget(nm.page.ctx, nm.handleNetworkEvent)
	nm.monitor = true
	return nil
}

// Disable disables network interception
func (nm *NetworkManager) Disable() error {
	nm.mu.Lock()
	defer nm.mu.Unlock()

	if !nm.enabled {
		return nil
	}

	if err := chromedp.Run(nm.page.ctx, fetch.Disable()); err != nil {
		return fmt.Errorf("disabling network interception: %w", err)
	}

	nm.enabled = false
	return nil
}

// Route adds a route handler for matching URLs
func (p *Page) Route(pattern string, handler RouteHandler) error {
	// Ensure network manager exists
	if p.networkManager == nil {
		p.networkManager = NewNetworkManager(p)
	}

	// Compile pattern as regex
	re, err := regexp.Compile(pattern)
	if err != nil {
		return fmt.Errorf("compiling route pattern: %w", err)
	}

	p.networkManager.mu.Lock()
	p.networkManager.routes = append(p.networkManager.routes, Route{
		pattern: re,
		handler: handler,
	})
	p.networkManager.mu.Unlock()

	// Enable network interception if not already enabled
	return p.networkManager.Enable()
}

// handleNetworkEvent processes network events
func (nm *NetworkManager) handleNetworkEvent(ev interface{}) {
	switch ev := ev.(type) {
	case *fetch.EventRequestPaused:
		go nm.handleRequestPaused(ev)
	case *network.EventRequestWillBeSent:
		nm.handleRequestWillBeSent(ev)
	case *network.EventResponseReceived:
		nm.handleResponseReceived(ev)
	}
}

func (nm *NetworkManager) handleRequestWillBeSent(ev *network.EventRequestWillBeSent) {
	req := &Request{
		ID:      string(ev.RequestID),
		URL:     ev.Request.URL,
		Method:  ev.Request.Method,
		Headers: headerMap(ev.Request.Headers),
		page:    nm.page,
	}
	nm.mu.Lock()
	nm.requests[ev.RequestID] = req
	nm.mu.Unlock()
}

func (nm *NetworkManager) handleResponseReceived(ev *network.EventResponseReceived) {
	resp := &Response{
		URL:        ev.Response.URL,
		Status:     int(ev.Response.Status),
		StatusText: ev.Response.StatusText,
		Headers:    headerMap(ev.Response.Headers),
	}

	nm.mu.Lock()
	nm.responses[ev.RequestID] = resp
	nm.mu.Unlock()
}

// handleRequestPaused handles intercepted requests
func (nm *NetworkManager) handleRequestPaused(ev *fetch.EventRequestPaused) {
	req := &Request{
		ID:          string(ev.RequestID),
		URL:         ev.Request.URL,
		Method:      ev.Request.Method,
		Headers:     headerMap(ev.Request.Headers),
		page:        nm.page,
		requestID:   ev.RequestID,
		intercepted: true,
	}

	// Get post data if available
	if ev.Request.HasPostData && len(ev.Request.PostDataEntries) > 0 {
		// Concatenate post data entries
		var postData strings.Builder
		for _, entry := range ev.Request.PostDataEntries {
			data, err := base64.StdEncoding.DecodeString(entry.Bytes)
			if err == nil {
				postData.Write(data)
				continue
			}
			postData.WriteString(entry.Bytes)
		}
		req.PostData = postData.String()
	}

	// Store request
	nm.mu.Lock()
	nm.requests[pausedRequestKey(ev)] = req
	nm.mu.Unlock()

	// Check if the request should be blocked
	if nm.blockingEngine != nil && nm.blockingEngine.ShouldBlock(req.URL) {
		// Block the request
		if err := req.Abort("accessdenied"); err != nil {
			// If blocking fails, continue the request
			req.Continue()
		}
		return
	}

	// Check if any route matches
	nm.mu.RLock()
	routes := append([]Route(nil), nm.routes...)
	nm.mu.RUnlock()

	for _, route := range routes {
		if route.pattern.MatchString(req.URL) {
			// Call handler
			if err := route.handler(req); err == nil {
				return // Handler processed the request
			}
		}
	}

	// No handler matched, continue request normally
	req.Continue()
}

func pausedRequestKey(ev *fetch.EventRequestPaused) network.RequestID {
	if ev.NetworkID != "" {
		return network.RequestID(ev.NetworkID)
	}
	return network.RequestID(ev.RequestID)
}

func headerMap(headers map[string]interface{}) map[string]string {
	m := make(map[string]string, len(headers))
	for name, value := range headers {
		m[name] = fmt.Sprint(value)
	}
	return m
}

// Continue continues the request
func (r *Request) Continue(opts ...ContinueOption) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if !r.intercepted {
		return errors.New("request not intercepted")
	}

	options := &ContinueOptions{
		PostData: r.PostData,
	}

	for _, opt := range opts {
		opt(options)
	}

	params := fetch.ContinueRequest(r.requestID)
	if options.URL != "" {
		params = params.WithURL(options.URL)
	}
	if options.Method != "" {
		params = params.WithMethod(options.Method)
	}
	if options.PostData != "" {
		params = params.WithPostData(options.PostData)
	}
	if len(options.Headers) > 0 {
		params = params.WithHeaders(fetchHeaders(options.Headers))
	}
	if err := chromedp.Run(r.page.ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		return params.Do(ctx)
	})); err != nil {
		return err
	}
	r.intercepted = false
	return nil
}

// Abort aborts the request
func (r *Request) Abort(reason string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if !r.intercepted {
		return errors.New("request not intercepted")
	}

	errorReason := network.ErrorReasonAborted
	switch reason {
	case "failed":
		errorReason = network.ErrorReasonFailed
	case "timedout":
		errorReason = network.ErrorReasonTimedOut
	case "accessdenied":
		errorReason = network.ErrorReasonAccessDenied
	}

	if err := chromedp.Run(r.page.ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		return fetch.FailRequest(r.requestID, errorReason).Do(ctx)
	})); err != nil {
		return err
	}
	r.intercepted = false
	return nil
}

// Fulfill fulfills the request with a custom response
func (r *Request) Fulfill(opts ...FulfillOption) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if !r.intercepted {
		return errors.New("request not intercepted")
	}

	options := &FulfillOptions{
		Status:  200,
		Headers: make(map[string]string),
		Body:    []byte{},
	}

	for _, opt := range opts {
		opt(options)
	}

	// Build response headers
	responseHeaders := make([]*fetch.HeaderEntry, 0, len(options.Headers))
	for name, value := range options.Headers {
		responseHeaders = append(responseHeaders, &fetch.HeaderEntry{
			Name:  name,
			Value: value,
		})
	}

	// Fulfill request
	if err := chromedp.Run(r.page.ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		return fetch.FulfillRequest(r.requestID, int64(options.Status)).
			WithResponseHeaders(responseHeaders).
			WithBody(base64.StdEncoding.EncodeToString(options.Body)).
			Do(ctx)
	})); err != nil {
		return err
	}
	r.intercepted = false
	return nil
}

func fetchHeaders(headers map[string]string) []*fetch.HeaderEntry {
	entries := make([]*fetch.HeaderEntry, 0, len(headers))
	for name, value := range headers {
		entries = append(entries, &fetch.HeaderEntry{Name: name, Value: value})
	}
	return entries
}

// ContinueOptions configures request continuation
type ContinueOptions struct {
	URL      string
	Method   string
	PostData string
	Headers  map[string]string
}

// ContinueOption modifies continue options
type ContinueOption func(*ContinueOptions)

// WithURL changes the request URL
func WithURL(url string) ContinueOption {
	return func(o *ContinueOptions) {
		o.URL = url
	}
}

// WithMethod changes the request method
func WithMethod(method string) ContinueOption {
	return func(o *ContinueOptions) {
		o.Method = method
	}
}

// WithPostData changes the post data
func WithPostData(data string) ContinueOption {
	return func(o *ContinueOptions) {
		o.PostData = data
	}
}

// WithHeaders sets request headers
func WithHeaders(headers map[string]string) ContinueOption {
	return func(o *ContinueOptions) {
		o.Headers = headers
	}
}

// FulfillOptions configures response fulfillment
type FulfillOptions struct {
	Status      int
	Headers     map[string]string
	Body        []byte
	ContentType string
}

// FulfillOption modifies fulfill options
type FulfillOption func(*FulfillOptions)

// WithStatus sets response status
func WithStatus(status int) FulfillOption {
	return func(o *FulfillOptions) {
		o.Status = status
	}
}

// WithResponseHeaders sets response headers
func WithResponseHeaders(headers map[string]string) FulfillOption {
	return func(o *FulfillOptions) {
		o.Headers = headers
	}
}

// WithBody sets response body
func WithBody(body []byte) FulfillOption {
	return func(o *FulfillOptions) {
		o.Body = body
	}
}

// WithContentType sets content type
func WithContentType(contentType string) FulfillOption {
	return func(o *FulfillOptions) {
		o.ContentType = contentType
		o.Headers["Content-Type"] = contentType
	}
}

// WaitForRequest waits for a request matching the pattern
func (p *Page) WaitForRequest(pattern string, timeout ...int) (*Request, error) {
	re, err := regexp.Compile(pattern)
	if err != nil {
		return nil, fmt.Errorf("compiling pattern: %w", err)
	}

	// Default timeout
	timeoutMs := 30000
	if len(timeout) > 0 {
		timeoutMs = timeout[0]
	}

	ctx, cancel := context.WithTimeout(p.ctx, time.Duration(timeoutMs)*time.Millisecond)
	defer cancel()

	// Enable network if needed
	if p.networkManager == nil {
		p.networkManager = NewNetworkManager(p)
		if err := p.networkManager.Enable(); err != nil {
			return nil, err
		}
	}

	// Wait for matching request
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil, errors.New("timeout waiting for request")
		case <-ticker.C:
			p.networkManager.mu.RLock()
			for _, req := range p.networkManager.requests {
				if re.MatchString(req.URL) {
					p.networkManager.mu.RUnlock()
					return req, nil
				}
			}
			p.networkManager.mu.RUnlock()
		}
	}
}

// WaitForResponse waits for a response matching the pattern
func (p *Page) WaitForResponse(pattern string, timeout ...int) (*Response, error) {
	re, err := regexp.Compile(pattern)
	if err != nil {
		return nil, fmt.Errorf("compiling pattern: %w", err)
	}

	// Default timeout
	timeoutMs := 30000
	if len(timeout) > 0 {
		timeoutMs = timeout[0]
	}

	ctx, cancel := context.WithTimeout(p.ctx, time.Duration(timeoutMs)*time.Millisecond)
	defer cancel()

	// Enable network if needed
	if p.networkManager == nil {
		p.networkManager = NewNetworkManager(p)
		if err := p.networkManager.Enable(); err != nil {
			return nil, err
		}
	}

	// Wait for matching response
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil, errors.New("timeout waiting for response")
		case <-ticker.C:
			p.networkManager.mu.RLock()
			for _, resp := range p.networkManager.responses {
				if re.MatchString(resp.URL) {
					p.networkManager.mu.RUnlock()
					return resp, nil
				}
			}
			p.networkManager.mu.RUnlock()
		}
	}
}
