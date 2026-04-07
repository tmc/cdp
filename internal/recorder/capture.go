package recorder

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/chromedp/cdproto/har"
	"github.com/chromedp/cdproto/runtime"
)

// CaptureEvent represents a structured event captured from JS injection
// scripts (gRPC-Web streaming, WebRTC DataChannel, etc.).
type CaptureEvent struct {
	Type      string    `json:"type"`      // "grpc-chunk", "grpc-complete", "grpc-request", "datachannel", "sdp"
	Timestamp time.Time `json:"timestamp"` // When the event was captured
	URL       string    `json:"url,omitempty"`
	Method    string    `json:"method,omitempty"`
	Headers   string    `json:"headers,omitempty"`
	Body      string    `json:"body,omitempty"`
	ChunkIdx  int       `json:"chunkIdx,omitempty"`
	Channel   string    `json:"channel,omitempty"`  // DataChannel label
	Direction string    `json:"direction,omitempty"` // "local" or "remote" for SDP
	Data      string    `json:"data,omitempty"`
}

const (
	// Console message prefixes used by injected JS capture scripts.
	grpcPrefix        = "CDP_GRPC:"
	dataChannelPrefix = "CDP_DC:"
)

// HandleConsoleCapture processes a console API event and routes structured
// capture messages (from injected JS) to the appropriate output files.
// Returns true if the message was a capture event (caller can suppress it
// from normal console output).
func (r *Recorder) HandleConsoleCapture(ev *runtime.EventConsoleAPICalled) bool {
	if len(ev.Args) == 0 || ev.Args[0].Value == nil {
		return false
	}

	var msg string
	if err := json.Unmarshal(ev.Args[0].Value, &msg); err != nil {
		return false
	}

	switch {
	case strings.HasPrefix(msg, grpcPrefix):
		r.handleGRPCCapture(msg[len(grpcPrefix):])
		return true
	case strings.HasPrefix(msg, dataChannelPrefix):
		r.handleDataChannelCapture(msg[len(dataChannelPrefix):])
		return true
	default:
		return false
	}
}

func (r *Recorder) handleGRPCCapture(payload string) {
	var ev struct {
		Type            string `json:"type"`
		URL             string `json:"url"`
		Method          string `json:"method"`
		Headers         string `json:"headers"`
		RespHeaders     string `json:"respHeaders"`
		RespContentType string `json:"respContentType"`
		Body            string `json:"body"`
		ChunkIdx        int    `json:"chunkIdx"`
		Chunk           string `json:"chunk"`
		Status          int    `json:"status"`
		StatusText      string `json:"statusText"`
		Full            string `json:"full"`
	}
	if err := json.Unmarshal([]byte(payload), &ev); err != nil {
		if r.verbose {
			log.Printf("capture: malformed gRPC event: %v", err)
		}
		return
	}

	r.Lock()
	defer r.Unlock()

	switch ev.Type {
	case "request":
		// Build a HAR entry for the request (response will come later via "complete").
		if r.verbose {
			log.Printf("capture: gRPC request %s %s (%d bytes body)", ev.Method, ev.URL, len(ev.Body))
		}
		ce := &CaptureEvent{
			Type:      "grpc-request",
			Timestamp: time.Now(),
			URL:       ev.URL,
			Method:    ev.Method,
			Headers:   ev.Headers,
			Body:      ev.Body,
		}
		r.writeCaptureEvent(ce)

	case "chunk":
		if r.verbose {
			log.Printf("capture: gRPC chunk #%d for %s (%d bytes)", ev.ChunkIdx, ev.URL, len(ev.Chunk))
		}
		ce := &CaptureEvent{
			Type:      "grpc-chunk",
			Timestamp: time.Now(),
			URL:       ev.URL,
			ChunkIdx:  ev.ChunkIdx,
			Data:      ev.Chunk,
		}
		r.writeCaptureEvent(ce)

	case "complete":
		if r.verbose {
			log.Printf("capture: gRPC complete %s (%d bytes, status %d)", ev.URL, len(ev.Full), ev.Status)
		}
		ce := &CaptureEvent{
			Type:      "grpc-complete",
			Timestamp: time.Now(),
			URL:       ev.URL,
			Data:      ev.Full,
		}
		r.writeCaptureEvent(ce)

		// Also emit a synthetic HAR entry so it appears in domain JSONL.
		if r.streaming {
			respMIME := ev.RespContentType
			if respMIME == "" {
				respMIME = "application/octet-stream"
			}
			statusText := ev.StatusText
			if statusText == "" {
				statusText = "OK"
			}
			entry := &har.Entry{
				StartedDateTime: time.Now().Format(time.RFC3339),
				Comment:         "captured-via-fetch-wrapper",
				Request: &har.Request{
					Method:      ev.Method,
					URL:         ev.URL,
					HTTPVersion: "HTTP/1.1",
					Headers:     parseHeaderString(ev.Headers),
				},
				Response: &har.Response{
					Status:     int64(ev.Status),
					StatusText: statusText,
					Headers:    parseHeaderString(ev.RespHeaders),
					Content: &har.Content{
						MimeType: respMIME,
						Size:     int64(len(ev.Full)),
						Text:     ev.Full,
					},
				},
			}
			if ev.Body != "" {
				reqMIME := "application/x-www-form-urlencoded"
				// Use request content-type header if available.
				for _, h := range entry.Request.Headers {
					if strings.EqualFold(h.Name, "content-type") {
						reqMIME = h.Value
						break
					}
				}
				entry.Request.PostData = &har.PostData{
					MimeType: reqMIME,
					Text:     ev.Body,
				}
			}
			r.scrubEntry(entry)
			r.streamEntry(entry)
		}
	}
}

func (r *Recorder) handleDataChannelCapture(payload string) {
	var ev struct {
		Type    string `json:"type"`
		Label   string `json:"label"`
		Dir     string `json:"dir"`
		Data    string `json:"data"`
		Binary  bool   `json:"binary"`
		SDP     string `json:"sdp"`
		SDPType string `json:"sdpType"`
	}
	if err := json.Unmarshal([]byte(payload), &ev); err != nil {
		if r.verbose {
			log.Printf("capture: malformed DataChannel event: %v", err)
		}
		return
	}

	r.Lock()
	defer r.Unlock()

	switch ev.Type {
	case "message":
		if r.verbose {
			log.Printf("capture: DC message on %q (%s, %d bytes, binary=%v)",
				ev.Label, ev.Dir, len(ev.Data), ev.Binary)
		}
		ce := &CaptureEvent{
			Type:      "datachannel",
			Timestamp: time.Now(),
			Channel:   ev.Label,
			Direction: ev.Dir,
			Data:      ev.Data,
		}
		r.writeCaptureEvent(ce)

		// Synthesize a HAR entry so DataChannel messages appear in HARL output.
		if r.streaming {
			mimeType := "application/x-protobuf"
			if !ev.Binary {
				mimeType = "text/plain"
			}
			method := "RECEIVE"
			if ev.Dir == "outgoing" {
				method = "SEND"
			}
			entry := &har.Entry{
				StartedDateTime: time.Now().Format(time.RFC3339),
				Comment:         fmt.Sprintf("datachannel:%s", ev.Label),
				Request: &har.Request{
					Method:      method,
					URL:         fmt.Sprintf("datachannel://webrtc-datachannel/%s/%s", ev.Label, ev.Dir),
					HTTPVersion: "webrtc",
				},
				Response: &har.Response{
					Status:     200,
					StatusText: ev.Dir,
					Content: &har.Content{
						MimeType: mimeType,
						Size:     int64(len(ev.Data)),
						Text:     ev.Data,
					},
				},
			}
			r.scrubEntry(entry)
			r.streamEntry(entry)
		}

	case "sdp-local", "sdp-remote":
		if r.verbose {
			log.Printf("capture: SDP %s (%s)", ev.Type, ev.SDPType)
		}
		dir := strings.TrimPrefix(ev.Type, "sdp-")
		ce := &CaptureEvent{
			Type:      "sdp",
			Timestamp: time.Now(),
			Direction: dir,
			Data:      ev.SDP,
		}
		r.writeCaptureEvent(ce)

		// Synthesize a HAR entry for SDP exchange.
		if r.streaming {
			entry := &har.Entry{
				StartedDateTime: time.Now().Format(time.RFC3339),
				Comment:         fmt.Sprintf("sdp:%s:%s", dir, ev.SDPType),
				Request: &har.Request{
					Method:      "SDP",
					URL:         fmt.Sprintf("webrtc://webrtc-signaling/sdp/%s/%s", dir, ev.SDPType),
					HTTPVersion: "webrtc",
				},
				Response: &har.Response{
					Status:     200,
					StatusText: ev.SDPType,
					Content: &har.Content{
						MimeType: "application/sdp",
						Size:     int64(len(ev.SDP)),
						Text:     ev.SDP,
					},
				},
			}
			r.streamEntry(entry)
		}
	}
}

// parseHeaderString converts a newline-separated "Name: Value" string into
// HAR name-value pairs.
func parseHeaderString(s string) []*har.NameValuePair {
	if s == "" {
		return nil
	}
	var pairs []*har.NameValuePair
	for _, line := range strings.Split(s, "\n") {
		if i := strings.IndexByte(line, ':'); i > 0 {
			pairs = append(pairs, &har.NameValuePair{
				Name:  strings.TrimSpace(line[:i]),
				Value: strings.TrimSpace(line[i+1:]),
			})
		}
	}
	return pairs
}

// writeCaptureEvent writes a capture event to the _capture.jsonl file in the
// current output directory. Caller must hold r.Lock.
func (r *Recorder) writeCaptureEvent(ce *CaptureEvent) {
	if r.outputDir == "" {
		// No output directory; print to stdout.
		if data, err := json.Marshal(ce); err == nil {
			fmt.Println(string(data))
		}
		return
	}

	if err := os.MkdirAll(r.outputDir, 0755); err != nil {
		if r.verbose {
			log.Printf("capture: mkdir %s: %v", r.outputDir, err)
		}
		return
	}

	filename := filepath.Join(r.outputDir, "_capture.jsonl")
	f, err := os.OpenFile(filename, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		if r.verbose {
			log.Printf("capture: open %s: %v", filename, err)
		}
		return
	}
	defer f.Close()

	data, err := json.Marshal(ce)
	if err != nil {
		return
	}
	fmt.Fprintln(f, string(data))
}
