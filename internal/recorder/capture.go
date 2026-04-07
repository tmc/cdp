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
		Type     string `json:"type"`
		URL      string `json:"url"`
		Method   string `json:"method"`
		Headers  string `json:"headers"`
		Body     string `json:"body"`
		ChunkIdx int    `json:"chunkIdx"`
		Chunk    string `json:"chunk"`
		Status   int    `json:"status"`
		Full     string `json:"full"`
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
			entry := &har.Entry{
				StartedDateTime: time.Now().Format(time.RFC3339),
				Comment:         "captured-via-fetch-wrapper",
				Request: &har.Request{
					Method:      ev.Method,
					URL:         ev.URL,
					HTTPVersion: "HTTP/1.1",
				},
				Response: &har.Response{
					Status:     int64(ev.Status),
					StatusText: "OK",
					Content: &har.Content{
						MimeType: "application/x-protobuf", // gRPC-Web content type
						Size:     int64(len(ev.Full)),
						Text:     ev.Full,
					},
				},
			}
			if ev.Body != "" {
				entry.Request.PostData = &har.PostData{
					MimeType: "application/x-www-form-urlencoded",
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

	case "sdp-local", "sdp-remote":
		if r.verbose {
			log.Printf("capture: SDP %s (%s)", ev.Type, ev.SDPType)
		}
		ce := &CaptureEvent{
			Type:      "sdp",
			Timestamp: time.Now(),
			Direction: strings.TrimPrefix(ev.Type, "sdp-"),
			Data:      ev.SDP,
		}
		r.writeCaptureEvent(ce)
	}
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
