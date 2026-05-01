package recorder

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chromedp/cdproto/network"
)

// TestWebSocketCapture exercises the WebSocket event handlers end-to-end:
// it feeds a synthetic CDP event sequence (created → handshake →
// frames → closed) through the recorder and asserts the per-host JSONL
// file contains a final entry with status 101, _resourceType=websocket,
// and the expected _webSocketMessages.
func TestWebSocketCapture(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	r, err := New(WithStreaming(true), WithOutputDir(dir))
	if err != nil {
		t.Fatal(err)
	}

	handler := r.HandleNetworkEvent(context.Background())

	const reqID = network.RequestID("ws-1")
	wssURL := "wss://realtime.example.com/socket"

	handler(&network.EventWebSocketCreated{RequestID: reqID, URL: wssURL})
	handler(&network.EventWebSocketWillSendHandshakeRequest{
		RequestID: reqID,
		Request: &network.WebSocketRequest{
			Headers: map[string]interface{}{
				"Upgrade":               "websocket",
				"Sec-WebSocket-Version": "13",
			},
		},
	})
	handler(&network.EventWebSocketHandshakeResponseReceived{
		RequestID: reqID,
		Response: &network.WebSocketResponse{
			Status:     101,
			StatusText: "Switching Protocols",
			Headers: map[string]interface{}{
				"Upgrade":              "websocket",
				"Sec-WebSocket-Accept": "abc=",
			},
		},
	})

	// One text frame each direction.
	handler(&network.EventWebSocketFrameSent{
		RequestID: reqID,
		Response:  &network.WebSocketFrame{Opcode: 1, PayloadData: `{"hello":"server"}`},
	})
	handler(&network.EventWebSocketFrameReceived{
		RequestID: reqID,
		Response:  &network.WebSocketFrame{Opcode: 1, PayloadData: `{"hello":"client"}`},
	})

	// Large binary frame should be truncated. The CDP encodes binary
	// payload as base64; build a 100KB payload and verify only the first
	// 64KB are kept and DataTruncatedBytes records the original size.
	bigPayload := make([]byte, 100*1024)
	for i := range bigPayload {
		bigPayload[i] = byte(i % 256)
	}
	handler(&network.EventWebSocketFrameReceived{
		RequestID: reqID,
		Response:  &network.WebSocketFrame{Opcode: 2, PayloadData: base64.StdEncoding.EncodeToString(bigPayload)},
	})

	handler(&network.EventWebSocketClosed{RequestID: reqID})

	// Force any open writer to flush.
	r.CloseDomainWriters()

	path := filepath.Join(dir, "realtime.example.com.jsonl")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("expected JSONL file at %s: %v", path, err)
	}

	lines := strings.Split(strings.TrimRight(string(raw), "\n"), "\n")
	if len(lines) == 0 {
		t.Fatalf("no entries written")
	}

	// The last line is the close-time snapshot — that's the authoritative
	// one with all frames present.
	var entry wsHAREntry
	if err := json.Unmarshal([]byte(lines[len(lines)-1]), &entry); err != nil {
		t.Fatalf("decode last line: %v\nline: %s", err, lines[len(lines)-1])
	}

	if entry.ResourceType != "websocket" {
		t.Errorf("_resourceType = %q, want %q", entry.ResourceType, "websocket")
	}
	if entry.Request == nil || entry.Request.URL != wssURL {
		t.Errorf("request URL = %v, want %s", entry.Request, wssURL)
	}
	if entry.Response == nil || entry.Response.Status != 101 {
		t.Errorf("response status = %v, want 101", entry.Response)
	}

	if got, want := len(entry.WebSocketMessages), 3; got != want {
		t.Fatalf("_webSocketMessages length = %d, want %d", got, want)
	}

	if entry.WebSocketMessages[0].Type != "send" || entry.WebSocketMessages[0].Opcode != 1 {
		t.Errorf("first message: %+v", entry.WebSocketMessages[0])
	}
	if entry.WebSocketMessages[1].Type != "receive" || entry.WebSocketMessages[1].Data != `{"hello":"client"}` {
		t.Errorf("second message: %+v", entry.WebSocketMessages[1])
	}

	bin := entry.WebSocketMessages[2]
	if bin.Type != "receive" || bin.Opcode != 2 {
		t.Errorf("binary frame metadata wrong: %+v", bin)
	}
	if bin.DataTruncatedBytes != len(bigPayload) {
		t.Errorf("data_truncated_bytes = %d, want %d", bin.DataTruncatedBytes, len(bigPayload))
	}
	decoded, err := base64.StdEncoding.DecodeString(bin.Data)
	if err != nil {
		t.Fatalf("binary frame data was not valid base64: %v", err)
	}
	if len(decoded) != wsBinaryFrameLimit {
		t.Errorf("truncated binary length = %d, want %d", len(decoded), wsBinaryFrameLimit)
	}

	// Verify Upgrade header survived through to the request side.
	hasUpgrade := false
	for _, h := range entry.Request.Headers {
		if strings.EqualFold(h.Name, "Upgrade") && strings.EqualFold(h.Value, "websocket") {
			hasUpgrade = true
		}
	}
	if !hasUpgrade {
		t.Errorf("Upgrade: websocket header missing from request: %+v", entry.Request.Headers)
	}
}

// TestWebSocketCapture_NoStreamingNoOutput verifies that with streaming
// disabled the recorder doesn't crash on WebSocket events even though it
// drops them — important because cdp passes the same handler regardless
// of the harl flag.
func TestWebSocketCapture_NoStreamingNoOutput(t *testing.T) {
	t.Parallel()
	r, err := New() // streaming off, no output dir
	if err != nil {
		t.Fatal(err)
	}
	handler := r.HandleNetworkEvent(context.Background())
	handler(&network.EventWebSocketCreated{RequestID: "x", URL: "wss://example.com/"})
	handler(&network.EventWebSocketFrameReceived{
		RequestID: "x",
		Response:  &network.WebSocketFrame{Opcode: 1, PayloadData: "hi"},
	})
	handler(&network.EventWebSocketClosed{RequestID: "x"})
}
