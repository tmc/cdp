package recorder

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/chromedp/cdproto/har"
	"github.com/chromedp/cdproto/network"
)

// wsBinaryFrameLimit caps captured binary frame payloads. Frames larger than
// this are truncated and the full original size is recorded in
// dataTruncatedBytes so callers can detect truncation.
const wsBinaryFrameLimit = 64 * 1024

// wsMessage is one frame in the Chrome DevTools _webSocketMessages convention.
// It is serialized as part of wsHAREntry below.
type wsMessage struct {
	Type               string  `json:"type"`                           // "send" or "receive"
	Time               float64 `json:"time"`                           // unix seconds (float)
	Opcode             int     `json:"opcode"`                         // 1=text, 2=binary, 8=close, 9=ping, 10=pong
	Data               string  `json:"data"`                           // utf-8 text (opcode 1) or base64 (others)
	DataTruncatedBytes int     `json:"data_truncated_bytes,omitempty"` // original size if Data was truncated
}

// wsConn tracks an in-flight WebSocket connection.
type wsConn struct {
	requestID       network.RequestID
	url             string
	startedAt       time.Time
	requestHeaders  map[string]string
	responseStatus  int
	responseHeaders map[string]string
	messages        []wsMessage
	closed          bool
	closedAt        time.Time
	errorMessage    string
	tag             string
	outputDir       string // snapshotted at WebSocketCreated time
	pageDomain      string // snapshotted at WebSocketCreated time
}

// wsHAREntry is the JSON shape we emit for a WebSocket connection. It mirrors
// the HAR entry layout for HTTP requests and adds the Chrome DevTools
// _webSocketMessages extension plus a _resourceType marker for grep-friendly
// filtering.
type wsHAREntry struct {
	StartedDateTime   string        `json:"startedDateTime"`
	Time              float64       `json:"time"`
	Request           *har.Request  `json:"request"`
	Response          *har.Response `json:"response"`
	Cache             *har.Cache    `json:"cache,omitempty"`
	Timings           *har.Timings  `json:"timings,omitempty"`
	Comment           string        `json:"comment,omitempty"`
	ResourceType      string        `json:"_resourceType"`
	WebSocketMessages []wsMessage   `json:"_webSocketMessages"`
}

// wsLockedFields is the set of recorder fields used for WS tracking. These
// are guarded by Recorder.Mutex (the same lock as the rest of the recorder
// state) so HandleNetworkEvent can dispatch without acquiring two locks.
type wsLockedFields struct {
	conns map[network.RequestID]*wsConn
	once  sync.Once
}

// initWS lazily initializes the WS map. Caller must hold r.Mutex.
func (r *Recorder) initWS() {
	if r.ws.conns == nil {
		r.ws.conns = make(map[network.RequestID]*wsConn)
	}
}

// handleWebSocketEvent dispatches CDP WebSocket events. Caller must hold
// r.Mutex.
func (r *Recorder) handleWebSocketEvent(ev interface{}) {
	switch e := ev.(type) {
	case *network.EventWebSocketCreated:
		r.wsCreated(e)
	case *network.EventWebSocketWillSendHandshakeRequest:
		r.wsHandshakeRequest(e)
	case *network.EventWebSocketHandshakeResponseReceived:
		r.wsHandshakeResponse(e)
	case *network.EventWebSocketFrameSent:
		r.wsFrame(e.RequestID, "send", e.Response)
	case *network.EventWebSocketFrameReceived:
		r.wsFrame(e.RequestID, "receive", e.Response)
	case *network.EventWebSocketFrameError:
		r.wsFrameError(e)
	case *network.EventWebSocketClosed:
		r.wsClosed(e)
	}
}

func (r *Recorder) wsCreated(e *network.EventWebSocketCreated) {
	r.initWS()
	c := &wsConn{
		requestID:       e.RequestID,
		url:             e.URL,
		startedAt:       time.Now(),
		requestHeaders:  map[string]string{},
		responseHeaders: map[string]string{},
		tag:             r.currentTag,
		outputDir:       r.outputDir,
		pageDomain:      r.pageDomain,
	}
	r.ws.conns[e.RequestID] = c
	if r.verbose {
		log.Printf("WebSocket created: %s", e.URL)
	}
	// Stream an initial entry immediately so consumers see the upgrade
	// even before frames arrive.
	r.streamWSEntry(c)
}

func (r *Recorder) wsHandshakeRequest(e *network.EventWebSocketWillSendHandshakeRequest) {
	r.initWS()
	c, ok := r.ws.conns[e.RequestID]
	if !ok {
		return
	}
	if e.Request != nil {
		for k, v := range e.Request.Headers {
			c.requestHeaders[k] = fmt.Sprint(v)
		}
	}
}

func (r *Recorder) wsHandshakeResponse(e *network.EventWebSocketHandshakeResponseReceived) {
	r.initWS()
	c, ok := r.ws.conns[e.RequestID]
	if !ok {
		return
	}
	if e.Response != nil {
		c.responseStatus = int(e.Response.Status)
		for k, v := range e.Response.Headers {
			c.responseHeaders[k] = fmt.Sprint(v)
		}
		// HandshakeResponseReceived also carries the request headers Chrome
		// actually sent over the wire (post any browser-added defaults).
		// Prefer those.
		if len(e.Response.RequestHeaders) > 0 {
			for k, v := range e.Response.RequestHeaders {
				c.requestHeaders[k] = fmt.Sprint(v)
			}
		}
	}
	if r.verbose {
		log.Printf("WebSocket handshake: %s -> %d", c.url, c.responseStatus)
	}
	r.streamWSEntry(c)
}

func (r *Recorder) wsFrame(reqID network.RequestID, dir string, frame *network.WebSocketFrame) {
	r.initWS()
	c, ok := r.ws.conns[reqID]
	if !ok || frame == nil {
		return
	}
	msg := buildWSMessage(dir, frame)
	c.messages = append(c.messages, msg)
	if r.streaming {
		r.streamWSEntry(c)
	}
}

// buildWSMessage converts a CDP WebSocketFrame into the DevTools
// _webSocketMessages shape, base64-encoding non-text payloads and
// truncating large binary frames.
func buildWSMessage(dir string, frame *network.WebSocketFrame) wsMessage {
	opcode := int(frame.Opcode)
	msg := wsMessage{
		Type:   dir,
		Time:   float64(time.Now().UnixNano()) / 1e9,
		Opcode: opcode,
	}
	switch opcode {
	case 1:
		// Text frame — payloadData is utf-8.
		msg.Data = frame.PayloadData
	default:
		// Binary / control frame — payloadData is already base64 from CDP.
		// Decode to measure original size, then re-encode (truncated if needed)
		// so consumers always see canonical base64.
		raw, err := base64.StdEncoding.DecodeString(frame.PayloadData)
		if err != nil {
			// Fall back to the raw string; size is best-effort.
			msg.Data = frame.PayloadData
			break
		}
		if len(raw) > wsBinaryFrameLimit {
			msg.DataTruncatedBytes = len(raw)
			raw = raw[:wsBinaryFrameLimit]
		}
		msg.Data = base64.StdEncoding.EncodeToString(raw)
	}
	return msg
}

func (r *Recorder) wsFrameError(e *network.EventWebSocketFrameError) {
	r.initWS()
	c, ok := r.ws.conns[e.RequestID]
	if !ok {
		return
	}
	c.errorMessage = e.ErrorMessage
	if r.verbose {
		log.Printf("WebSocket frame error: %s: %s", c.url, e.ErrorMessage)
	}
}

func (r *Recorder) wsClosed(e *network.EventWebSocketClosed) {
	r.initWS()
	c, ok := r.ws.conns[e.RequestID]
	if !ok {
		return
	}
	c.closed = true
	c.closedAt = time.Now()
	if r.verbose {
		log.Printf("WebSocket closed: %s (%d frames)", c.url, len(c.messages))
	}
	r.streamWSEntry(c)
}

// streamWSEntry emits a snapshot of the connection. Caller must hold r.Mutex.
// Repeated calls re-emit the entry as frames accumulate so streaming
// consumers see incremental updates; the final emission on close is the
// authoritative one.
func (r *Recorder) streamWSEntry(c *wsConn) {
	if !r.streaming {
		return
	}

	entry := r.buildWSEntry(c)
	r.scrubWSEntry(entry)

	jsonBytes, err := json.Marshal(entry)
	if err != nil {
		if r.verbose {
			log.Printf("Error marshaling WS entry: %v", err)
		}
		return
	}

	if r.outputDir != "" || c.outputDir != "" {
		dir := c.outputDir
		if dir == "" {
			dir = r.outputDir
		}
		if err := r.writeRawToDomainFileAtPage(c.url, c.pageDomain, dir, jsonBytes); err != nil {
			if r.verbose {
				log.Printf("Error writing WS entry: %v", err)
			}
		}
		return
	}
	if r.outputFile != "" {
		if err := appendJSONL(r.outputFile, jsonBytes); err != nil && r.verbose {
			log.Printf("Error writing WS entry to stream file: %v", err)
		}
		return
	}

	fmt.Println(string(jsonBytes))
}

// buildWSEntry materializes the JSON shape for a connection. Caller must
// hold r.Mutex.
func (r *Recorder) buildWSEntry(c *wsConn) *wsHAREntry {
	reqHeaders := make([]*har.NameValuePair, 0, len(c.requestHeaders))
	for k, v := range c.requestHeaders {
		reqHeaders = append(reqHeaders, &har.NameValuePair{Name: k, Value: v})
	}
	respHeaders := make([]*har.NameValuePair, 0, len(c.responseHeaders))
	for k, v := range c.responseHeaders {
		respHeaders = append(respHeaders, &har.NameValuePair{Name: k, Value: v})
	}

	status := c.responseStatus
	statusText := ""
	if status == 0 {
		// Handshake not yet seen — emit the upgrade-pending state so the
		// URL shows up immediately. Use 0/"" so it's distinguishable.
	} else if status == 101 {
		statusText = "Switching Protocols"
	}

	totalMs := float64(0)
	if c.closed {
		totalMs = float64(c.closedAt.Sub(c.startedAt)) / float64(time.Millisecond)
	} else {
		totalMs = float64(time.Since(c.startedAt)) / float64(time.Millisecond)
	}

	entry := &wsHAREntry{
		StartedDateTime: c.startedAt.UTC().Format(time.RFC3339Nano),
		Time:            totalMs,
		Request: &har.Request{
			Method:      "GET",
			URL:         c.url,
			HTTPVersion: "HTTP/1.1",
			Headers:     reqHeaders,
		},
		Response: &har.Response{
			Status:      int64(status),
			StatusText:  statusText,
			HTTPVersion: "HTTP/1.1",
			Headers:     respHeaders,
			Content:     &har.Content{MimeType: "x-unknown"},
		},
		ResourceType:      "websocket",
		WebSocketMessages: append([]wsMessage(nil), c.messages...),
	}

	parts := []string{}
	if c.tag != "" {
		parts = append(parts, "tag:"+c.tag)
	}
	if c.errorMessage != "" {
		parts = append(parts, "error:"+c.errorMessage)
	}
	if c.closed {
		parts = append(parts, "closed")
	}
	if len(parts) > 0 {
		entry.Comment = joinComments(parts)
	}
	return entry
}

// scrubWSEntry redacts secrets in headers and text frames. Caller must hold
// r.Mutex.
func (r *Recorder) scrubWSEntry(entry *wsHAREntry) {
	if r.scrubber == nil || !r.scrubber.Enabled() {
		return
	}
	if entry.Request != nil {
		entry.Request.URL = scrubURL(r.scrubber, entry.Request.URL)
		for i := range entry.Request.Headers {
			entry.Request.Headers[i].Value = r.scrubber.ScrubHeaderValue(
				entry.Request.Headers[i].Name, entry.Request.Headers[i].Value)
		}
	}
	if entry.Response != nil {
		for i := range entry.Response.Headers {
			entry.Response.Headers[i].Value = r.scrubber.ScrubHeaderValue(
				entry.Response.Headers[i].Name, entry.Response.Headers[i].Value)
		}
	}
	for i, m := range entry.WebSocketMessages {
		if m.Opcode == 1 {
			entry.WebSocketMessages[i].Data, _ = r.scrubber.ScrubText(m.Data)
		}
	}
}

func joinComments(parts []string) string {
	out := ""
	for i, p := range parts {
		if i > 0 {
			out += " "
		}
		out += p
	}
	return out
}
