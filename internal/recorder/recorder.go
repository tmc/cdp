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
	"strings"
	"sync"
	"text/template"
	"time"

	"github.com/chromedp/cdproto/dom"
	"github.com/chromedp/cdproto/fetch"
	"github.com/chromedp/cdproto/har"
	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"
	chromeErrors "github.com/tmc/misc/chrome-to-har/internal/errors"
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

type Recorder struct {
	sync.Mutex
	requests      map[network.RequestID]*network.Request
	responses     map[network.RequestID]*network.Response
	bodies        map[network.RequestID][]byte
	postData      map[network.RequestID]string
	timings       map[network.RequestID]*network.EventLoadingFinished
	requestTags   map[network.RequestID]string // Tag for each request
	annotations   []*Annotation                // Manual annotations from shell commands
	verbose       bool
	streaming     bool
	filter        *FilterOption
	template      string
	ctx           context.Context // Store context for async body fetching
	outputDir     string
	domainWriters map[string]*os.File

	// Fetch domain interception
	fetchBodies map[network.RequestID][]byte // Bodies captured via Fetch domain

	// Tag tracking
	currentTag string      // Currently active tag
	tagRanges  []*TagRange // History of tag ranges
}

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

// SetOutputDir changes the output directory, closing any open domain writers.
// New writes will go to files in the new directory.
func (r *Recorder) SetOutputDir(dir string) {
	r.Lock()
	defer r.Unlock()
	for hostname, f := range r.domainWriters {
		f.Close()
		delete(r.domainWriters, hostname)
	}
	r.outputDir = dir
}

// CloseDomainWriters closes all open domain file handles.
func (r *Recorder) CloseDomainWriters() {
	r.Lock()
	defer r.Unlock()
	for hostname, f := range r.domainWriters {
		f.Close()
		delete(r.domainWriters, hostname)
	}
}

func New(opts ...Option) (*Recorder, error) {
	r := &Recorder{
		requests:      make(map[network.RequestID]*network.Request),
		responses:     make(map[network.RequestID]*network.Response),
		bodies:        make(map[network.RequestID][]byte),
		postData:      make(map[network.RequestID]string),
		timings:       make(map[network.RequestID]*network.EventLoadingFinished),
		requestTags:   make(map[network.RequestID]string),
		annotations:   make([]*Annotation, 0),
		fetchBodies:   make(map[network.RequestID][]byte),
		domainWriters: make(map[string]*os.File),
		tagRanges:     make([]*TagRange, 0),
	}

	for _, opt := range opts {
		if err := opt(r); err != nil {
			return nil, err
		}
	}

	return r, nil
}

func (r *Recorder) HandleNetworkEvent(ctx context.Context) func(interface{}) {
	// Store context for later use in fetching response bodies
	r.Lock()
	r.ctx = ctx
	r.Unlock()

	return func(ev interface{}) {
		r.Lock()
		defer r.Unlock()

		switch e := ev.(type) {
		case *network.EventRequestWillBeSent:
			if r.verbose {
				log.Printf("Request: %s %s", e.Request.Method, e.Request.URL)
			}
			r.requests[e.RequestID] = e.Request
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

		case *network.EventLoadingFinished:
			r.timings[e.RequestID] = e

			// Skip if body already captured via Fetch domain interception.
			if _, ok := r.fetchBodies[e.RequestID]; ok {
				break
			}

			// Fetch response bodies for both streaming and non-streaming modes.
			// NOTE: GetResponseBody can fail with -32000 ("No resource with given
			// identifier found") for redirects, cached responses, and service worker
			// responses where Chrome evicts the body before we fetch it. This is a
			// known CDP limitation.
			go func(reqID network.RequestID) {
				r.Lock()
				fetchCtx := r.ctx
				r.Unlock()

				if fetchCtx == nil {
					return
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
							entry := r.buildStreamEntry(reqID, resp, nil)
							r.streamEntry(entry)
						}
						r.Unlock()
					}
					return
				}

				r.Lock()
				r.bodies[reqID] = body
				if r.verbose {
					log.Printf("Captured response body for request %s (%d bytes)", reqID, len(body))
				}

				if r.streaming {
					resp := r.responses[reqID]
					if resp != nil {
						entry := r.buildStreamEntry(reqID, resp, body)
						r.streamEntry(entry)
					}
				}
				r.Unlock()
			}(e.RequestID)
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

		// Snapshot context-dependent state before the goroutine runs,
		// since push/pop-context may change outputDir while we wait
		// for the response body.
		r.Lock()
		snapshotDir := r.outputDir
		snapshotTag := r.currentTag
		r.Unlock()

		// Response stage — capture body then continue.
		go func() {
			var body []byte
			err := chromedp.Run(ctx, chromedp.ActionFunc(func(c context.Context) error {
				var fetchErr error
				body, fetchErr = fetch.GetResponseBody(e.RequestID).Do(c)
				return fetchErr
			}))

			// Always continue the response regardless of body fetch result.
			if contErr := chromedp.Run(ctx, chromedp.ActionFunc(func(c context.Context) error {
				return fetch.ContinueResponse(e.RequestID).Do(c)
			})); contErr != nil {
				if r.verbose {
					log.Printf("fetch: continue response %s: %v", e.RequestID, contErr)
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

			if body != nil {
				r.bodies[netID] = body
				r.fetchBodies[netID] = body
			}

			if r.streaming {
				resp := r.responses[netID]
				if resp != nil {
					entry := r.buildStreamEntry(netID, resp, body)
					// Use the snapshotted outputDir so the entry goes to
					// the correct push-context subdirectory.
					savedDir := r.outputDir
					r.outputDir = snapshotDir
					r.streamEntry(entry)
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
func (r *Recorder) buildStreamEntry(reqID network.RequestID, resp *network.Response, body []byte) *har.Entry {
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
		content.Size = int64(len(body))
		content.Text = string(body)
	}
	return &har.Entry{
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
}

func (r *Recorder) streamEntry(entry *har.Entry) {
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

	if r.outputDir != "" {
		if err := r.writeToDomainFile(entry, jsonBytes); err != nil {
			if r.verbose {
				log.Printf("Error writing to domain file: %v", err)
			}
		}
		return
	}

	fmt.Println(string(jsonBytes))
}

// writeToDomainFile writes entry to a domain-specific file.
// Note: caller must hold r.Lock() - this function does not acquire the lock
// to avoid deadlock when called from streamEntry which is called from HandleNetworkEvent.
func (r *Recorder) writeToDomainFile(entry *har.Entry, data []byte) error {
	var uStr string
	if entry.Request != nil && entry.Request.URL != "" {
		uStr = entry.Request.URL
	}
	// Note: har.Response doesn't have a URL field.
	// If entry.Request.URL is empty, we can't determine the domain.

	if uStr == "" {
		return fmt.Errorf("no URL in entry")
	}

	u, err := url.Parse(uStr)
	if err != nil {
		return err
	}
	hostname := u.Hostname()
	if hostname == "" {
		hostname = "unknown_domain"
	}

	// Note: lock is already held by caller (HandleNetworkEvent -> streamEntry)
	writer, ok := r.domainWriters[hostname]
	if ok {
		// Check if the file was removed; if so, reopen it.
		if _, statErr := writer.Stat(); statErr != nil {
			writer.Close()
			delete(r.domainWriters, hostname)
			ok = false
		}
	}
	if !ok {
		// Ensure output directory exists.
		if err := os.MkdirAll(r.outputDir, 0755); err != nil {
			return err
		}

		filename := filepath.Join(r.outputDir, fmt.Sprintf("%s.jsonl", hostname))
		f, err := os.OpenFile(filename, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			return err
		}
		writer = f
		r.domainWriters[hostname] = writer
	}

	_, err = fmt.Fprintln(writer, string(data))
	return err
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
			// Check if body is base64 encoded (binary content)
			if isBinaryContent(resp.MimeType) {
				harResponse.Content.Text = base64.StdEncoding.EncodeToString(body)
				harResponse.Content.Encoding = "base64"
			} else {
				harResponse.Content.Text = string(body)
			}
			harResponse.Content.Size = int64(len(body))
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
	tagRanges := r.tagRanges
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
			Annotations: r.annotations,
			TagRanges:   tagRanges,
		},
	}

	jsonBytes, err := json.MarshalIndent(harWithAnnotations, "", "  ")
	if err != nil {
		return chromeErrors.Wrap(err, chromeErrors.NetworkRecordError, "failed to marshal HAR data")
	}

	if err := os.WriteFile(filename, jsonBytes, 0644); err != nil {
		return chromeErrors.WithContext(
			chromeErrors.FileError("write", filename, err),
			"format", "har",
		)
	}

	if r.verbose {
		if len(r.annotations) > 0 {
			log.Printf("Included %d annotations in HAR file", len(r.annotations))
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
		return nil, chromeErrors.WithContext(
			chromeErrors.Wrap(err, chromeErrors.ValidationError, "failed to parse template"),
			"template", r.template,
		)
	}

	var buf strings.Builder
	if err := t.Execute(&buf, entry); err != nil {
		return nil, chromeErrors.WithContext(
			chromeErrors.Wrap(err, chromeErrors.ValidationError, "failed to execute template"),
			"template", r.template,
		)
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
	r.Lock()
	defer r.Unlock()

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

	r.annotations = append(r.annotations, annotation)

	if r.verbose {
		log.Printf("Added note: %s (URL: %s)", description, currentURL)
	}

	return nil
}

// AddScreenshot captures a screenshot with description
func (r *Recorder) AddScreenshot(ctx context.Context, description string) error {
	r.Lock()
	defer r.Unlock()

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
	if err := chromedp.Run(screenshotCtx, chromedp.FullScreenshot(&buf, 90)); err != nil {
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

	r.annotations = append(r.annotations, annotation)

	if r.verbose {
		log.Printf("Added screenshot: %s (%d bytes, URL: %s)", description, len(buf), currentURL)
	}

	return nil
}

// AddDOMSnapshot captures the current DOM state
func (r *Recorder) AddDOMSnapshot(ctx context.Context, description string) error {
	r.Lock()
	defer r.Unlock()

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

	r.annotations = append(r.annotations, annotation)

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
