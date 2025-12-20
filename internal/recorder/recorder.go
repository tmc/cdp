package recorder

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net/url"
	"os"
	"strings"
	"sync"
	"text/template"
	"time"

	"github.com/chromedp/cdproto/dom"
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

type Recorder struct {
	sync.Mutex
	requests    map[network.RequestID]*network.Request
	responses   map[network.RequestID]*network.Response
	bodies      map[network.RequestID][]byte
	postData    map[network.RequestID]string
	timings     map[network.RequestID]*network.EventLoadingFinished
	annotations []*Annotation // Manual annotations from shell commands
	verbose     bool
	streaming   bool
	filter      *FilterOption
	template    string
	ctx         context.Context // Store context for async body fetching
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

func New(opts ...Option) (*Recorder, error) {
	r := &Recorder{
		requests:    make(map[network.RequestID]*network.Request),
		responses:   make(map[network.RequestID]*network.Response),
		bodies:      make(map[network.RequestID][]byte),
		postData:    make(map[network.RequestID]string),
		timings:     make(map[network.RequestID]*network.EventLoadingFinished),
		annotations: make([]*Annotation, 0),
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

			if r.streaming {
				entry := &har.Entry{
					StartedDateTime: time.Now().Format(time.RFC3339),
					Request: &har.Request{
						Method:      e.Request.Method,
						URL:         e.Request.URL,
						HTTPVersion: "HTTP/1.1", // Default to HTTP/1.1 as Protocol isn't available
						Headers:     convertHeaders(e.Request.Headers),
					},
				}
				r.streamEntry(entry)
			}

		case *network.EventResponseReceived:
			if r.verbose {
				log.Printf("Response: %d %s", e.Response.Status, e.Response.URL)
			}
			r.responses[e.RequestID] = e.Response

			if r.streaming {
				entry := &har.Entry{
					StartedDateTime: time.Now().Format(time.RFC3339),
					Request: &har.Request{
						Method: r.requests[e.RequestID].Method,
						URL:    e.Response.URL,
					},
					Response: &har.Response{
						Status:      int64(e.Response.Status),
						StatusText:  e.Response.StatusText,
						HTTPVersion: e.Response.Protocol,
						Headers:     convertHeaders(e.Response.Headers),
						Content: &har.Content{
							MimeType: e.Response.MimeType,
							Size:     int64(e.Response.EncodedDataLength),
						},
					},
				}
				r.streamEntry(entry)
			}

		case *network.EventLoadingFinished:
			r.timings[e.RequestID] = e

			// Always fetch response bodies (both streaming and non-streaming modes)
			go func(reqID network.RequestID) {
				// Use stored context for fetching body
				r.Lock()
				fetchCtx := r.ctx
				r.Unlock()

				if fetchCtx == nil {
					if r.verbose {
						log.Printf("No context available for fetching response body")
					}
					return
				}

				body, err := network.GetResponseBody(reqID).Do(fetchCtx)
				if err != nil {
					if r.verbose {
						log.Printf("Error getting response body for request %s: %v", reqID, err)
					}
					return
				}

				r.Lock()
				r.bodies[reqID] = body
				if r.verbose {
					log.Printf("Captured response body for request %s (%d bytes)", reqID, len(body))
				}
				r.Unlock()

				if r.streaming {
					// TODO: Update streaming entry with body
				}
			}(e.RequestID)
		}
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
	fmt.Println(string(jsonBytes))
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

	// Create a wrapper that includes annotations
	// HAR spec doesn't officially support custom fields in Log, but we can add them
	harWithAnnotations := struct {
		Log *struct {
			*har.Log
			Annotations []*Annotation `json:"_annotations,omitempty"`
		} `json:"log"`
	}{
		Log: &struct {
			*har.Log
			Annotations []*Annotation `json:"_annotations,omitempty"`
		}{
			Log:         h.Log,
			Annotations: r.annotations,
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

	if r.verbose && len(r.annotations) > 0 {
		log.Printf("Included %d annotations in HAR file", len(r.annotations))
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
