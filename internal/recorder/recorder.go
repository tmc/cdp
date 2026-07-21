package recorder

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"text/template"
	"time"

	"github.com/chromedp/cdproto/dom"
	"github.com/chromedp/cdproto/fetch"
	"github.com/chromedp/cdproto/har"
	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"

	"github.com/tmc/cdp/internal/scrub"
	"github.com/tmc/cdp/internal/sitegroup"
)

type Annotation struct {
	Type        string    `json:"type"`        // "note", "screenshot", "dom"
	Timestamp   time.Time `json:"timestamp"`   // When annotation was created
	Description string    `json:"description"` // User-provided description
	Data        string    `json:"data"`        // Base64-encoded screenshot or DOM HTML
	MimeType    string    `json:"mimeType"`    // For screenshots: "image/png"
	URL         string    `json:"url"`         // Current page URL at time of annotation
}

// TagRange represents a tagged range of network activity.
type TagRange struct {
	Tag       string    `json:"tag"`
	StartTime time.Time `json:"startTime"`
	EndTime   time.Time `json:"endTime,omitempty"`
}

type capturedBody struct {
	Data         []byte
	OriginalSize int
}

const (
	fetchBodyTimeout     = 10 * time.Second
	fetchBodyGrace       = 250 * time.Millisecond
	fetchContinueTimeout = 2 * time.Second

	// fetchBodyMaxContentLength caps the declared response size for which the
	// body is captured while the response is paused. Fetch.getResponseBody
	// transfers the whole body over the CDP connection while the page waits,
	// so large responses are continued immediately (metadata only) rather than
	// holding the page paused. Bodies without a Content-Length are still
	// captured, bounded by fetchBodyGrace.
	fetchBodyMaxContentLength = 2 << 20 // 2 MiB
)

func (b capturedBody) truncated() bool {
	return b.OriginalSize > len(b.Data)
}

type Recorder struct {
	sync.Mutex
	requests     map[network.RequestID]*network.Request
	responses    map[network.RequestID]*network.Response
	bodies       map[network.RequestID]capturedBody
	postData     map[network.RequestID]string
	timings      map[network.RequestID]*network.EventLoadingFinished
	requestTags  map[network.RequestID]string // Tag for each request
	requestPages map[network.RequestID]string // Page domain for each request
	annotations  []*Annotation                // Manual annotations from shell commands
	verbose      bool
	streaming    bool
	filter       *FilterOption
	template     string
	ctx          context.Context // Store context for async body fetching
	outputDir    string
	outputFile   string
	maxBodyBytes int64
	groupByPage  bool

	// Fetch domain interception
	fetchBodies map[network.RequestID]capturedBody // Bodies captured via Fetch domain

	// Tag tracking
	currentTag string      // Currently active tag
	pageDomain string      // Full hostname of the current top-level page
	tagRanges  []*TagRange // History of tag ranges

	// Secret scrubbing
	scrubber *scrub.Scrubber

	// WebSocket capture (see websocket_capture.go).
	ws wsLockedFields

	// Writer goroutine: decouples disk I/O from the chromedp event loop.
	// Per-host file handles live exclusively inside writerLoop; callers
	// send commands via the writes channel. See writer.go.
	writes         chan writerCmd
	writerDone     chan struct{}
	writerStopOnce sync.Once
	writerStopped  atomic.Bool // true after stopWriter completes
	dropped        uint64      // atomic; incremented when writes channel is full
}

// writeQueueSize bounds the writer goroutine's intake buffer. Sized for a
// burst of streamed entries during a heavy navigation (a large page can
// generate hundreds of network entries within a few hundred milliseconds);
// disk throughput drains this in milliseconds at steady state.
const writeQueueSize = 1024

type FilterOption struct {
	JQExpr   string
	Template string
}

type Option func(*Recorder) error

func WithVerbose(verbose bool) Option {
	return func(r *Recorder) error {
		r.verbose = verbose
		return nil
	}
}

func WithStreaming(streaming bool) Option {
	return func(r *Recorder) error {
		r.streaming = streaming
		return nil
	}
}

func WithFilter(filter string) Option {
	return func(r *Recorder) error {
		if filter != "" {
			r.filter = &FilterOption{JQExpr: filter}
		}
		return nil
	}
}

func WithTemplate(template string) Option {
	return func(r *Recorder) error {
		r.template = template
		return nil
	}
}

func WithOutputDir(dir string) Option {
	return func(r *Recorder) error {
		r.outputDir = dir
		return nil
	}
}

// WithOutputFile writes streamed HARL entries to a single JSONL file.
func WithOutputFile(file string) Option {
	return func(r *Recorder) error {
		r.outputFile = file
		return nil
	}
}

// WithMaxBodyBytes caps stored HTTP response bodies. A non-positive limit keeps
// full bodies.
func WithMaxBodyBytes(n int64) Option {
	return func(r *Recorder) error {
		if n < 0 {
			return fmt.Errorf("max body bytes must be non-negative")
		}
		r.maxBodyBytes = n
		return nil
	}
}

// WithGroupByPage selects whether output directories use the navigated page's
// registrable domain. The default is enabled.
func WithGroupByPage(enabled bool) Option {
	return func(r *Recorder) error {
		r.groupByPage = enabled
		return nil
	}
}

func WithScrubber(s *scrub.Scrubber) Option {
	return func(r *Recorder) error {
		r.scrubber = s
		return nil
	}
}

// SetOutputDir changes the output directory, closing any open domain writers.
// New writes will go to files in the new directory.
func (r *Recorder) SetOutputDir(dir string) {
	r.closeAllWriters() // synchronous — drains any in-flight writes first
	r.Lock()
	r.outputDir = dir
	r.Unlock()
}

// CloseDomainWriters closes all open domain file handles. The writer
// goroutine continues running; subsequent writes will reopen handles
// lazily.
func (r *Recorder) CloseDomainWriters() {
	r.closeAllWriters()
}

// Close stops the writer goroutine after draining any queued writes.
// Safe to call more than once. Should be called before the Recorder
// goes out of scope so file handles flush; HAR/Save flows that need
// to read the on-disk record after writing must Close first.
func (r *Recorder) Close() {
	r.stopWriter()
}

func New(opts ...Option) (*Recorder, error) {
	r := &Recorder{
		requests:     make(map[network.RequestID]*network.Request),
		responses:    make(map[network.RequestID]*network.Response),
		bodies:       make(map[network.RequestID]capturedBody),
		postData:     make(map[network.RequestID]string),
		timings:      make(map[network.RequestID]*network.EventLoadingFinished),
		requestTags:  make(map[network.RequestID]string),
		requestPages: make(map[network.RequestID]string),
		annotations:  make([]*Annotation, 0),
		fetchBodies:  make(map[network.RequestID]capturedBody),
		tagRanges:    make([]*TagRange, 0),
		writes:       make(chan writerCmd, writeQueueSize),
		writerDone:   make(chan struct{}),
		groupByPage:  true,
	}

	for _, opt := range opts {
		if err := opt(r); err != nil {
			return nil, err
		}
	}

	go r.writerLoop()

	return r, nil
}

func pageDomain(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "unknown_domain"
	}
	return sitegroup.Host(u.Hostname())
}

func requestDomain(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "unknown_domain"
	}
	return sitegroup.Host(u.Hostname())
}

func (r *Recorder) captureBody(body []byte) capturedBody {
	c := capturedBody{
		Data:         body,
		OriginalSize: len(body),
	}
	if r.maxBodyBytes > 0 && int64(len(body)) > r.maxBodyBytes {
		c.Data = append([]byte(nil), body[:int(r.maxBodyBytes)]...)
	}
	return c
}

func setContentBody(content *har.Content, mimeType string, body capturedBody) {
	content.Size = int64(body.OriginalSize)
	if isBinaryContent(mimeType) {
		content.Text = base64.StdEncoding.EncodeToString(body.Data)
		content.Encoding = "base64"
	} else {
		content.Text = string(body.Data)
	}
	if body.truncated() {
		content.Comment = fmt.Sprintf("body truncated: captured %d of %d bytes", len(body.Data), body.OriginalSize)
	}
}

func (r *Recorder) HandleNetworkEvent(ctx context.Context) func(interface{}) {
	// Store context for later use in fetching response bodies
	r.Lock()
	r.ctx = ctx
	r.Unlock()

	return func(ev interface{}) {
		r.Lock()
		defer r.Unlock()
		if e, ok := ev.(*page.EventFrameNavigated); ok && e.Frame != nil && e.Frame.ParentID == "" {
			r.pageDomain = pageDomain(e.Frame.URL)
			if r.verbose {
				log.Printf("Page navigated: %s (group %s)", e.Frame.URL, r.pageDomain)
			}
			return
		}

		// WebSocket events are dispatched to a dedicated handler. They share
		// the recorder mutex with the HTTP path so streaming entries to the
		// same per-host JSONL file is race-free.
		switch ev.(type) {
		case *network.EventWebSocketCreated,
			*network.EventWebSocketWillSendHandshakeRequest,
			*network.EventWebSocketHandshakeResponseReceived,
			*network.EventWebSocketFrameSent,
			*network.EventWebSocketFrameReceived,
			*network.EventWebSocketFrameError,
			*network.EventWebSocketClosed:
			r.handleWebSocketEvent(ev)
			return
		}

		switch e := ev.(type) {
		case *network.EventRequestWillBeSent:
			// The request for the top-level document can arrive before
			// Page.frameNavigated. Use it to seed the page group so the
			// document itself is not stranded under unknown_domain.
			if r.pageDomain == "" && e.Type == network.ResourceTypeDocument && e.Request != nil {
				r.pageDomain = pageDomain(e.Request.URL)
			}
			if r.verbose {
				log.Printf("Request: %s %s", e.Request.Method, e.Request.URL)
			}
			r.requests[e.RequestID] = e.Request
			r.requestPages[e.RequestID] = r.pageDomain
			// Tag this request with the current tag
			if r.currentTag != "" {
				r.requestTags[e.RequestID] = r.currentTag
			}

			// Capture POST data if present
			if e.Request.HasPostData && len(e.Request.PostDataEntries) > 0 {
				// Concatenate all post data entries
				var postDataBuilder strings.Builder
				for _, entry := range e.Request.PostDataEntries {
					if entry.Bytes != "" {
						postDataBuilder.WriteString(entry.Bytes)
					}
				}
				if postDataBuilder.Len() > 0 {
					r.postData[e.RequestID] = postDataBuilder.String()
					if r.verbose {
						log.Printf("Captured POST data for %s (%d bytes)", e.Request.URL, postDataBuilder.Len())
					}
				}
			}

		case *network.EventResponseReceived:
			if r.verbose {
				log.Printf("Response: %d %s", e.Response.Status, e.Response.URL)
			}
			r.responses[e.RequestID] = e.Response

			// Streaming deferred to LoadingFinished for complete entry with body.

		case *network.EventLoadingFailed:
			// A failed request (connection reset/refused, blocked, aborted,
			// CORS failure) never reaches LoadingFinished, so it would otherwise
			// be dropped from the capture entirely. Write a request-only entry
			// with the error recorded so outgoing requests are not lost.
			if _, ok := r.fetchBodies[e.RequestID]; ok {
				break
			}
			if entry := r.buildFailedEntry(e.RequestID, e); entry != nil {
				r.streamEntryAtPage(entry, r.requestPages[e.RequestID], r.outputDir)
			}

		case *network.EventLoadingFinished:
			r.timings[e.RequestID] = e

			// Skip if body already captured via Fetch domain interception.
			if _, ok := r.fetchBodies[e.RequestID]; ok {
				break
			}

			// Snapshot context directory before goroutine to prevent
			// race with push/pop-context changing the output dir.
			snapshotDir := r.outputDir
			snapshotTag := r.currentTag
			snapshotPage := r.requestPages[e.RequestID]

			// Fetch response bodies for both streaming and non-streaming modes.
			// NOTE: GetResponseBody can fail with -32000 ("No resource with given
			// identifier found") for redirects, cached responses, and service worker
			// responses where Chrome evicts the body before we fetch it. This is a
			// known CDP limitation.
			go func(reqID network.RequestID, snapDir, snapTag, snapPage string) {
				r.Lock()
				fetchCtx := r.ctx
				r.Unlock()

				if fetchCtx == nil {
					return
				}

				// Fetch request post data if not already captured inline.
				// Large uploads (e.g. resumable file uploads) set HasPostData
				// but omit PostDataEntries; GetRequestPostData retrieves them.
				r.Lock()
				_, hasPostData := r.postData[reqID]
				req := r.requests[reqID]
				r.Unlock()
				if !hasPostData && req != nil && req.HasPostData {
					var postData []byte
					if err := chromedp.Run(fetchCtx, chromedp.ActionFunc(func(ctx context.Context) error {
						var fetchErr error
						postData, fetchErr = network.GetRequestPostData(reqID).Do(ctx)
						return fetchErr
					})); err == nil && len(postData) > 0 {
						r.Lock()
						r.postData[reqID] = string(postData)
						r.Unlock()
					}
				}

				var body []byte
				err := chromedp.Run(fetchCtx, chromedp.ActionFunc(func(ctx context.Context) error {
					var fetchErr error
					body, fetchErr = network.GetResponseBody(reqID).Do(ctx)
					return fetchErr
				}))
				if err != nil {
					// Body unavailable — expected for redirects, cached, and
					// service-worker responses. Stream entry without body.
					if r.streaming {
						r.Lock()
						resp := r.responses[reqID]
						if resp != nil {
							if snapTag != "" {
								r.requestTags[reqID] = snapTag
							}
							savedDir := r.outputDir
							r.outputDir = snapDir
							entry := r.buildStreamEntry(reqID, resp, nil)
							r.streamEntryAtPage(entry, snapPage, snapDir)
							r.outputDir = savedDir
						}
						r.Unlock()
					}
					return
				}

				r.Lock()
				captured := r.captureBody(body)
				r.bodies[reqID] = captured
				if r.verbose {
					log.Printf("Captured response body for request %s (%d bytes)", reqID, len(body))
				}

				if r.streaming {
					resp := r.responses[reqID]
					if resp != nil {
						if snapTag != "" {
							r.requestTags[reqID] = snapTag
						}
						savedDir := r.outputDir
						r.outputDir = snapDir
						entry := r.buildStreamEntry(reqID, resp, &captured)
						r.streamEntryAtPage(entry, snapPage, snapDir)
						r.outputDir = savedDir
					}
				}
				r.Unlock()
			}(e.RequestID, snapshotDir, snapshotTag, snapshotPage)
		}
	}
}

// HandleFetchEvent returns an event handler for Fetch domain events.
// When Fetch.enable is called with RequestStageResponse patterns, Chrome
// pauses each matching response and fires EventRequestPaused. This handler
// captures the response body (guaranteed available while paused), records it,
// and continues the response so the page receives it normally.
//
// This captures traffic that the Network domain may miss, such as gRPC-Web
// streaming fetches and service-worker-intercepted requests.
// fetchContentLength returns the declared Content-Length from paused-response
// headers, or -1 when absent or unparseable (so it never trips the size gate).
func fetchContentLength(headers []*fetch.HeaderEntry) int64 {
	for _, h := range headers {
		if strings.EqualFold(h.Name, "content-length") {
			n, err := strconv.ParseInt(strings.TrimSpace(h.Value), 10, 64)
			if err != nil {
				return -1
			}
			return n
		}
	}
	return -1
}

func (r *Recorder) HandleFetchEvent(ctx context.Context) func(interface{}) {
	return func(ev interface{}) {
		e, ok := ev.(*fetch.EventRequestPaused)
		if !ok {
			return
		}

		// Only handle responses (have a status code).
		if e.ResponseStatusCode == 0 {
			// Request stage — let it through.
			go func() {
				if err := chromedp.Run(ctx, chromedp.ActionFunc(func(c context.Context) error {
					return fetch.ContinueRequest(e.RequestID).Do(c)
				})); err != nil {
					if r.verbose {
						log.Printf("fetch: continue request %s: %v", e.RequestID, err)
					}
				}
			}()
			return
		}

		// Large responses are continued immediately without capturing the body:
		// pulling a big body over CDP while the response is paused stalls the
		// page. Small and unknown-size responses fall through to body capture.
		if fetchContentLength(e.ResponseHeaders) > fetchBodyMaxContentLength {
			go func() {
				continueCtx, cancel := context.WithTimeout(ctx, fetchContinueTimeout)
				defer cancel()
				if err := chromedp.Run(continueCtx, chromedp.ActionFunc(func(c context.Context) error {
					return fetch.ContinueResponse(e.RequestID).Do(c)
				})); err != nil && r.verbose {
					log.Printf("fetch: continue large response %s: %v", e.RequestID, err)
				}
			}()
			return
		}

		// Snapshot context-dependent state before the goroutine runs,
		// since push/pop-context may change outputDir while we wait
		// for the response body.
		r.Lock()
		snapshotDir := r.outputDir
		snapshotTag := r.currentTag
		snapshotPage := r.pageDomain
		r.Unlock()

		// Response stage — capture body then continue.
		go func() {
			type bodyResult struct {
				body []byte
				err  error
			}
			bodyCh := make(chan bodyResult, 1)
			go func() {
				bodyCtx, bodyCancel := context.WithTimeout(ctx, fetchBodyTimeout)
				defer bodyCancel()
				var body []byte
				err := chromedp.Run(bodyCtx, chromedp.ActionFunc(func(c context.Context) error {
					var fetchErr error
					body, fetchErr = fetch.GetResponseBody(e.RequestID).Do(c)
					return fetchErr
				}))
				bodyCh <- bodyResult{body: body, err: err}
			}()

			var body []byte
			var err error
			bodyReady := false
			select {
			case result := <-bodyCh:
				body, err, bodyReady = result.body, result.err, true
			case <-time.After(fetchBodyGrace):
				// Continue promptly for streaming or stalled responses. A
				// completed body is preferred, but it must not hold Chrome
				// paused while navigation waits for load.
			}

			// Always continue the response regardless of body fetch result. Use
			// a fresh deadline so a timed-out body request cannot leave Chrome
			// paused indefinitely.
			continueCtx, continueCancel := context.WithTimeout(ctx, fetchContinueTimeout)
			contErr := chromedp.Run(continueCtx, chromedp.ActionFunc(func(c context.Context) error {
				return fetch.ContinueResponse(e.RequestID).Do(c)
			}))
			continueCancel()
			if contErr != nil {
				if r.verbose {
					log.Printf("fetch: continue response %s: %v", e.RequestID, contErr)
				}
			}
			if !bodyReady {
				select {
				case result := <-bodyCh:
					body, err = result.body, result.err
				case <-time.After(100 * time.Millisecond):
					err = context.DeadlineExceeded
				}
			}

			if err != nil {
				if r.verbose {
					log.Printf("fetch: get body %s: %v", e.RequestID, err)
				}
			}

			// Map fetch request to network request ID if available.
			netID := e.NetworkID
			if netID == "" {
				// Synthesize an ID for requests not seen by Network domain.
				netID = network.RequestID("fetch:" + string(e.RequestID))
			}

			r.Lock()
			defer r.Unlock()

			// Store the request if not already known from Network events.
			if _, exists := r.requests[netID]; !exists {
				r.requests[netID] = e.Request
				r.requestPages[netID] = snapshotPage
				if snapshotTag != "" {
					r.requestTags[netID] = snapshotTag
				}
				if e.Request.HasPostData && len(e.Request.PostDataEntries) > 0 {
					var b strings.Builder
					for _, entry := range e.Request.PostDataEntries {
						if entry.Bytes != "" {
							b.WriteString(entry.Bytes)
						}
					}
					if b.Len() > 0 {
						r.postData[netID] = b.String()
					}
				}
			}

			// Build a synthetic response from fetch headers.
			if _, exists := r.responses[netID]; !exists {
				hdrs := make(map[string]interface{}, len(e.ResponseHeaders))
				mimeType := ""
				for _, h := range e.ResponseHeaders {
					hdrs[h.Name] = h.Value
					if strings.EqualFold(h.Name, "content-type") {
						mimeType = h.Value
					}
				}
				r.responses[netID] = &network.Response{
					URL:        e.Request.URL,
					Status:     e.ResponseStatusCode,
					StatusText: e.ResponseStatusText,
					Headers:    network.Headers(hdrs),
					MimeType:   mimeType,
				}
			}

			var captured *capturedBody
			if body != nil {
				c := r.captureBody(body)
				r.bodies[netID] = c
				r.fetchBodies[netID] = c
				captured = &c
			}

			if r.streaming {
				resp := r.responses[netID]
				if resp != nil {
					entry := r.buildStreamEntry(netID, resp, captured)
					// Use the snapshotted outputDir so the entry goes to
					// the correct push-context subdirectory.
					savedDir := r.outputDir
					r.outputDir = snapshotDir
					r.streamEntryAtPage(entry, snapshotPage, snapshotDir)
					r.outputDir = savedDir
				}
			}

			if r.verbose {
				log.Printf("fetch: captured %s %s (%d bytes body)",
					e.Request.Method, e.Request.URL, len(body))
			}
		}()
	}
}

// buildStreamEntry creates a HAR entry from stored request/response data.
// Caller must hold r.Lock.
func (r *Recorder) buildStreamEntry(reqID network.RequestID, resp *network.Response, body *capturedBody) *har.Entry {
	harReq := &har.Request{
		Method: "GET",
		URL:    resp.URL,
	}
	if req, ok := r.requests[reqID]; ok && req != nil {
		harReq.Method = req.Method
		harReq.URL = req.URL
		harReq.HTTPVersion = "HTTP/1.1"
		harReq.Headers = convertHeaders(req.Headers)
		if pd, ok := r.postData[reqID]; ok && pd != "" {
			mimeType := ""
			if ct, ok := req.Headers["content-type"]; ok {
				mimeType, _ = ct.(string)
			}
			harReq.PostData = &har.PostData{
				MimeType: mimeType,
				Text:     pd,
			}
		}
	}
	content := &har.Content{
		MimeType: resp.MimeType,
		Size:     int64(resp.EncodedDataLength),
	}
	if body != nil {
		setContentBody(content, resp.MimeType, *body)
	}
	entry := &har.Entry{
		StartedDateTime: time.Now().Format(time.RFC3339),
		Request:         harReq,
		Response: &har.Response{
			Status:      int64(resp.Status),
			StatusText:  resp.StatusText,
			HTTPVersion: resp.Protocol,
			Headers:     convertHeaders(resp.Headers),
			Content:     content,
		},
	}
	r.scrubEntry(entry)
	return entry
}

// buildFailedEntry creates a HAR entry for a request that failed to load and
// so never produced a response. The request side is recorded in full; the
// response is a synthetic status 0 carrying the error text, matching how HAR
// tools represent failed requests. Caller must hold r.Lock.
func (r *Recorder) buildFailedEntry(reqID network.RequestID, e *network.EventLoadingFailed) *har.Entry {
	req, ok := r.requests[reqID]
	if !ok || req == nil {
		return nil
	}
	if r.verbose {
		log.Printf("Loading failed: %s %s (%s)", req.Method, req.URL, e.ErrorText)
	}
	harReq := &har.Request{
		Method:      req.Method,
		URL:         req.URL,
		HTTPVersion: "HTTP/1.1",
		Headers:     convertHeaders(req.Headers),
	}
	if pd, ok := r.postData[reqID]; ok && pd != "" {
		mimeType := ""
		if ct, ok := req.Headers["content-type"]; ok {
			mimeType, _ = ct.(string)
		}
		harReq.PostData = &har.PostData{MimeType: mimeType, Text: pd}
	}
	comment := "loading failed: " + e.ErrorText
	if e.Canceled {
		comment += " (canceled)"
	}
	if e.BlockedReason != "" {
		comment += " blocked: " + e.BlockedReason.String()
	}
	entry := &har.Entry{
		StartedDateTime: time.Now().Format(time.RFC3339),
		Comment:         comment,
		Request:         harReq,
		Response: &har.Response{
			Status:     0,
			StatusText: e.ErrorText,
			Content:    &har.Content{},
		},
	}
	r.scrubEntry(entry)
	return entry
}

// scrubEntry redacts secrets from a HAR entry in place.
func (r *Recorder) scrubEntry(entry *har.Entry) {
	if r.scrubber == nil || !r.scrubber.Enabled() {
		return
	}
	if entry.Request != nil {
		for i := range entry.Request.Headers {
			entry.Request.Headers[i].Value = r.scrubber.ScrubHeaderValue(
				entry.Request.Headers[i].Name, entry.Request.Headers[i].Value)
		}
		if entry.Request.URL != "" {
			entry.Request.URL = scrubURL(r.scrubber, entry.Request.URL)
		}
	}
	if entry.Response != nil {
		for i := range entry.Response.Headers {
			entry.Response.Headers[i].Value = r.scrubber.ScrubHeaderValue(
				entry.Response.Headers[i].Name, entry.Response.Headers[i].Value)
		}
		if entry.Response.Content != nil && entry.Response.Content.Text != "" {
			entry.Response.Content.Text, _ = r.scrubber.ScrubText(entry.Response.Content.Text)
		}
	}
}

// scrubURL redacts sensitive query parameter values in a URL.
func scrubURL(s *scrub.Scrubber, rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}
	q := u.Query()
	changed := false
	for key, vals := range q {
		for i, v := range vals {
			scrubbed := s.ScrubQueryParam(key, v)
			if scrubbed != v {
				vals[i] = scrubbed
				changed = true
			}
		}
	}
	if !changed {
		return rawURL
	}
	u.RawQuery = q.Encode()
	return u.String()
}

func (r *Recorder) streamEntry(entry *har.Entry) {
	r.Lock()
	page := r.pageDomain
	dir := r.outputDir
	r.Unlock()
	r.streamEntryAtPage(entry, page, dir)
}

func (r *Recorder) streamEntryAtPage(entry *har.Entry, page, dir string) {
	if r.filter != nil && r.filter.JQExpr != "" {
		filtered, err := r.applyJQFilter(entry)
		if err != nil {
			if r.verbose {
				log.Printf("Error applying filter: %v", err)
			}
			return
		}
		if filtered == nil {
			return // Entry filtered out
		}
		entry = filtered
	}

	if r.template != "" {
		templated, err := r.applyTemplate(entry)
		if err != nil {
			if r.verbose {
				log.Printf("Error applying template: %v", err)
			}
			return
		}
		entry = templated
	}

	jsonBytes, err := json.Marshal(entry)
	if err != nil {
		if r.verbose {
			log.Printf("Error marshaling entry: %v", err)
		}
		return
	}

	if dir != "" {
		if err := r.writeToDomainFileAtPage(entry, page, dir, jsonBytes); err != nil {
			if r.verbose {
				log.Printf("Error writing to domain file: %v", err)
			}
		}
		return
	}
	if r.outputFile != "" {
		if err := appendJSONL(r.outputFile, jsonBytes); err != nil && r.verbose {
			log.Printf("Error writing to stream file: %v", err)
		}
		return
	}

	fmt.Println(string(jsonBytes))
}

func appendJSONL(file string, data []byte) error {
	f, err := os.OpenFile(file, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		// The parent directory may have been removed mid-capture; recreate it
		// and retry once so streaming survives an rm -rf of the output dir.
		if dir := filepath.Dir(file); dir != "." {
			if mkErr := os.MkdirAll(dir, 0755); mkErr != nil {
				return err
			}
			f, err = os.OpenFile(file, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		}
		if err != nil {
			return err
		}
	}
	defer f.Close()

	_, err = fmt.Fprintln(f, string(data))
	return err
}

// writeToDomainFile streams entry to a domain-specific file via the writer
// goroutine. Returns immediately; disk I/O happens asynchronously.
func (r *Recorder) writeToDomainFile(entry *har.Entry, data []byte) error {
	page := r.pageDomain
	dir := r.outputDir
	return r.writeToDomainFileAtPage(entry, page, dir, data)
}

func (r *Recorder) writeToDomainFileAtPage(entry *har.Entry, page, dir string, data []byte) error {
	var uStr string
	if entry.Request != nil && entry.Request.URL != "" {
		uStr = entry.Request.URL
	}
	if uStr == "" {
		return fmt.Errorf("no URL in entry")
	}
	return r.writeRawToDomainFileAtPage(uStr, page, dir, data)
}

// writeRawToDomainFile enqueues a pre-marshaled JSON line for the writer
// goroutine to deliver to the per-host JSONL file. Used by both HTTP and
// WebSocket streaming paths. Non-blocking: under sustained backpressure the
// write is dropped and Recorder.dropped is incremented (see writer.go).
func (r *Recorder) writeRawToDomainFile(rawURL, dir string, data []byte) error {
	page := r.pageDomain
	return r.writeRawToDomainFileAtPage(rawURL, page, dir, data)
}

func (r *Recorder) writeRawToDomainFileAtPage(rawURL, page, dir string, data []byte) error {
	if !r.groupByPage {
		page = requestDomain(rawURL)
	}
	return r.enqueueWrite(rawURL, page, dir, data)
}

// HAR returns the HAR data structure
func (r *Recorder) HAR() (*har.HAR, error) {
	r.Lock()
	defer r.Unlock()

	h := &har.HAR{
		Log: &har.Log{
			Version: "1.2",
			Creator: &har.Creator{
				Name:    "chrome-to-har",
				Version: "1.0",
			},
			Pages:   make([]*har.Page, 0),
			Entries: make([]*har.Entry, 0),
		},
	}

	for reqID, req := range r.requests {
		resp := r.responses[reqID]
		if resp == nil {
			continue
		}

		timing := r.timings[reqID]
		if timing == nil {
			continue
		}

		// Build request with all fields
		harRequest := &har.Request{
			Method:      req.Method,
			URL:         req.URL,
			HTTPVersion: "HTTP/1.1", // Default to HTTP/1.1
			Headers:     convertHeaders(req.Headers),
			Cookies:     r.convertCookies(req.Headers),
			QueryString: parseQueryString(req.URL),
			HeadersSize: calculateHeadersSize(req.Headers),
			BodySize:    int64(len(r.postData[reqID])),
		}

		// Add POST data if present
		if postData, ok := r.postData[reqID]; ok && postData != "" {
			harRequest.PostData = &har.PostData{
				MimeType: getContentType(req.Headers),
				Text:     postData,
			}
		}

		// Build response with all fields
		harResponse := &har.Response{
			Status:      int64(resp.Status),
			StatusText:  resp.StatusText,
			HTTPVersion: resp.Protocol,
			Headers:     convertHeaders(resp.Headers),
			Cookies:     r.convertResponseCookies(resp.Headers),
			Content: &har.Content{
				Size:     int64(resp.EncodedDataLength),
				MimeType: resp.MimeType,
			},
			HeadersSize: calculateHeadersSize(resp.Headers),
			BodySize:    int64(resp.EncodedDataLength),
		}

		// Add response body if captured
		if body, ok := r.bodies[reqID]; ok {
			setContentBody(harResponse.Content, resp.MimeType, body)
		}

		entry := &har.Entry{
			StartedDateTime: time.Now().Format(time.RFC3339),
			Request:         harRequest,
			Response:        harResponse,
			Time:            float64(timing.Timestamp.Time().UnixNano()) / float64(time.Millisecond),
		}

		// Add tag to entry comment if present
		if tag, ok := r.requestTags[reqID]; ok && tag != "" {
			entry.Comment = fmt.Sprintf("tag:%s", tag)
		}

		r.scrubEntry(entry)
		h.Log.Entries = append(h.Log.Entries, entry)
	}

	return h, nil
}

// WriteHAR writes the HAR file to disk with annotations
func (r *Recorder) WriteHAR(filename string) error {
	if r.verbose {
		log.Printf("Writing HAR file to %s", filename)
	}

	h, err := r.HAR()
	if err != nil {
		return err
	}

	// Close any open tag range
	r.Lock()
	if r.currentTag != "" && len(r.tagRanges) > 0 {
		lastRange := r.tagRanges[len(r.tagRanges)-1]
		if lastRange.EndTime.IsZero() {
			lastRange.EndTime = time.Now()
		}
	}
	annotations := append([]*Annotation(nil), r.annotations...)
	tagRanges := append([]*TagRange(nil), r.tagRanges...)
	r.Unlock()

	// Create a wrapper that includes annotations and tag ranges
	// HAR spec doesn't officially support custom fields in Log, but we can add them
	harWithAnnotations := struct {
		Log *struct {
			*har.Log
			Annotations []*Annotation `json:"_annotations,omitempty"`
			TagRanges   []*TagRange   `json:"_tagRanges,omitempty"`
		} `json:"log"`
	}{
		Log: &struct {
			*har.Log
			Annotations []*Annotation `json:"_annotations,omitempty"`
			TagRanges   []*TagRange   `json:"_tagRanges,omitempty"`
		}{
			Log:         h.Log,
			Annotations: annotations,
			TagRanges:   tagRanges,
		},
	}

	jsonBytes, err := json.MarshalIndent(harWithAnnotations, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal HAR data: %w", err)
	}

	if err := os.WriteFile(filename, jsonBytes, 0644); err != nil {
		return fmt.Errorf("failed to write %s (format=har): %w", filename, err)
	}

	if r.verbose {
		if len(annotations) > 0 {
			log.Printf("Included %d annotations in HAR file", len(annotations))
		}
		if len(tagRanges) > 0 {
			log.Printf("Included %d tag ranges in HAR file", len(tagRanges))
		}
	}

	return nil
}

func convertHeaders(headers map[string]interface{}) []*har.NameValuePair {
	pairs := make([]*har.NameValuePair, 0, len(headers))
	for name, value := range headers {
		pairs = append(pairs, &har.NameValuePair{
			Name:  name,
			Value: fmt.Sprint(value),
		})
	}
	return pairs
}

func (r *Recorder) convertCookies(headers map[string]interface{}) []*har.Cookie {
	if cookieHeader, ok := headers["Cookie"]; ok {
		cookies := make([]*har.Cookie, 0)
		for _, cookie := range strings.Split(fmt.Sprint(cookieHeader), ";") {
			parts := strings.SplitN(strings.TrimSpace(cookie), "=", 2)
			if len(parts) != 2 {
				continue
			}
			cookies = append(cookies, &har.Cookie{
				Name:  parts[0],
				Value: parts[1],
			})
		}
		return cookies
	}
	return nil
}

func (r *Recorder) convertResponseCookies(headers map[string]interface{}) []*har.Cookie {
	if setCookieHeader, ok := headers["Set-Cookie"]; ok {
		cookies := make([]*har.Cookie, 0)
		// Set-Cookie can be a single string or array
		setCookieStr := fmt.Sprint(setCookieHeader)
		for _, cookie := range strings.Split(setCookieStr, ";") {
			parts := strings.SplitN(strings.TrimSpace(cookie), "=", 2)
			if len(parts) != 2 {
				continue
			}
			cookies = append(cookies, &har.Cookie{
				Name:  parts[0],
				Value: parts[1],
			})
		}
		return cookies
	}
	return nil
}

func parseQueryString(urlStr string) []*har.NameValuePair {
	parsedURL, err := url.Parse(urlStr)
	if err != nil {
		return nil
	}

	queryParams := make([]*har.NameValuePair, 0)
	for name, values := range parsedURL.Query() {
		for _, value := range values {
			queryParams = append(queryParams, &har.NameValuePair{
				Name:  name,
				Value: value,
			})
		}
	}
	return queryParams
}

func getContentType(headers map[string]interface{}) string {
	if ct, ok := headers["Content-Type"]; ok {
		return fmt.Sprint(ct)
	}
	return "application/octet-stream"
}

func calculateHeadersSize(headers map[string]interface{}) int64 {
	size := int64(0)
	for name, value := range headers {
		// Header format: "Name: Value\r\n"
		size += int64(len(name) + len(fmt.Sprint(value)) + 4)
	}
	return size
}

func isBinaryContent(mimeType string) bool {
	binaryTypes := []string{
		"image/", "audio/", "video/", "application/octet-stream",
		"application/pdf", "application/zip", "application/gzip",
	}
	for _, prefix := range binaryTypes {
		if strings.HasPrefix(mimeType, prefix) {
			return true
		}
	}
	return false
}

func (r *Recorder) applyTemplate(entry *har.Entry) (*har.Entry, error) {
	t, err := template.New("har").Parse(r.template)
	if err != nil {
		return nil, fmt.Errorf("failed to parse template %q: %w", r.template, err)
	}

	var buf strings.Builder
	if err := t.Execute(&buf, entry); err != nil {
		return nil, fmt.Errorf("failed to execute template %q: %w", r.template, err)
	}

	return &har.Entry{
		StartedDateTime: entry.StartedDateTime,
		Response: &har.Response{
			Content: &har.Content{
				Text: buf.String(),
			},
		},
	}, nil
}

// AddNote adds a text annotation to the recording
func (r *Recorder) AddNote(ctx context.Context, description string) error {
	// Get current URL with a short timeout
	var currentURL string
	urlCtx, urlCancel := context.WithTimeout(ctx, 2*time.Second)
	defer urlCancel()
	if err := chromedp.Run(urlCtx, chromedp.Location(&currentURL)); err != nil {
		if r.verbose {
			log.Printf("Warning: Could not get current URL: %v", err)
		}
	}

	annotation := &Annotation{
		Type:        "note",
		Timestamp:   time.Now(),
		Description: description,
		URL:         currentURL,
	}

	r.Lock()
	r.annotations = append(r.annotations, annotation)
	r.Unlock()

	if r.verbose {
		log.Printf("Added note: %s (URL: %s)", description, currentURL)
	}

	return nil
}

// AddScreenshot captures a screenshot with description
func (r *Recorder) AddScreenshot(ctx context.Context, description string) error {
	// Get current URL with a short timeout
	var currentURL string
	urlCtx, urlCancel := context.WithTimeout(ctx, 2*time.Second)
	defer urlCancel()
	if err := chromedp.Run(urlCtx, chromedp.Location(&currentURL)); err != nil {
		if r.verbose {
			log.Printf("Warning: Could not get current URL: %v", err)
		}
	}

	// Capture screenshot with timeout
	var buf []byte
	screenshotCtx, screenshotCancel := context.WithTimeout(ctx, 10*time.Second)
	defer screenshotCancel()
	if err := chromedp.Run(screenshotCtx, chromedp.FullScreenshot(&buf, 100)); err != nil {
		return fmt.Errorf("capturing screenshot: %w", err)
	}

	annotation := &Annotation{
		Type:        "screenshot",
		Timestamp:   time.Now(),
		Description: description,
		Data:        base64.StdEncoding.EncodeToString(buf),
		MimeType:    "image/png",
		URL:         currentURL,
	}

	r.Lock()
	r.annotations = append(r.annotations, annotation)
	r.Unlock()

	if r.verbose {
		log.Printf("Added screenshot: %s (%d bytes, URL: %s)", description, len(buf), currentURL)
	}

	return nil
}

// AddDOMSnapshot captures the current DOM state
func (r *Recorder) AddDOMSnapshot(ctx context.Context, description string) error {
	// Get current URL with a short timeout
	var currentURL string
	urlCtx, urlCancel := context.WithTimeout(ctx, 2*time.Second)
	defer urlCancel()
	if err := chromedp.Run(urlCtx, chromedp.Location(&currentURL)); err != nil {
		if r.verbose {
			log.Printf("Warning: Could not get current URL: %v", err)
		}
	}

	// Get the outer HTML of the document with timeout
	var domHTML string
	domCtx, domCancel := context.WithTimeout(ctx, 10*time.Second)
	defer domCancel()
	if err := chromedp.Run(domCtx, chromedp.ActionFunc(func(ctx context.Context) error {
		node, err := dom.GetDocument().Do(ctx)
		if err != nil {
			return err
		}
		domHTML, err = dom.GetOuterHTML().WithNodeID(node.NodeID).Do(ctx)
		return err
	})); err != nil {
		return fmt.Errorf("capturing DOM: %w", err)
	}

	annotation := &Annotation{
		Type:        "dom",
		Timestamp:   time.Now(),
		Description: description,
		Data:        domHTML,
		MimeType:    "text/html",
		URL:         currentURL,
	}

	r.Lock()
	r.annotations = append(r.annotations, annotation)
	r.Unlock()

	if r.verbose {
		log.Printf("Added DOM snapshot: %s (%d bytes, URL: %s)", description, len(domHTML), currentURL)
	}

	return nil
}

// GetAnnotations returns all annotations
func (r *Recorder) GetAnnotations() []*Annotation {
	r.Lock()
	defer r.Unlock()
	return r.annotations
}

// SetTag sets the current tag for subsequent network requests.
// An empty tag clears the current tag.
func (r *Recorder) SetTag(tag string) {
	r.Lock()
	defer r.Unlock()

	// Close out previous tag range if any
	if r.currentTag != "" && len(r.tagRanges) > 0 {
		lastRange := r.tagRanges[len(r.tagRanges)-1]
		if lastRange.EndTime.IsZero() {
			lastRange.EndTime = time.Now()
		}
	}

	r.currentTag = tag

	// Start new tag range if tag is not empty
	if tag != "" {
		r.tagRanges = append(r.tagRanges, &TagRange{
			Tag:       tag,
			StartTime: time.Now(),
		})
		if r.verbose {
			log.Printf("Tag set to: %s", tag)
		}
	} else if r.verbose {
		log.Printf("Tag cleared")
	}
}

// GetCurrentTag returns the currently active tag.
func (r *Recorder) GetCurrentTag() string {
	r.Lock()
	defer r.Unlock()
	return r.currentTag
}

// GetTagRanges returns all tag ranges recorded.
func (r *Recorder) GetTagRanges() []*TagRange {
	r.Lock()
	defer r.Unlock()

	// Make a copy to avoid external modification
	ranges := make([]*TagRange, len(r.tagRanges))
	copy(ranges, r.tagRanges)
	return ranges
}

// GetRequestTag returns the tag associated with a specific request.
func (r *Recorder) GetRequestTag(reqID network.RequestID) string {
	r.Lock()
	defer r.Unlock()
	return r.requestTags[reqID]
}
