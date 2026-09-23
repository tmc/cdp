package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"reflect"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestRawCDPResultUnmarshal(t *testing.T) {
	tests := []struct {
		name string
		data string
		want rawCDPResult
	}{
		{name: "empty object", data: "{}", want: rawCDPResult{}},
		{name: "result object", data: `{"result":{"value":"title"}}`, want: rawCDPResult{"result": map[string]any{"value": "title"}}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got rawCDPResult
			if err := json.Unmarshal([]byte(tt.data), &got); err != nil {
				t.Fatalf("Unmarshal(%q): %v", tt.data, err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("result = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestIsEmptyRawCDPResultError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{name: "nil", err: nil, want: false},
		{name: "eof", err: io.EOF, want: true},
		{name: "jsontext eof", err: errString("jsontext: unexpected EOF"), want: true},
		{name: "json eof", err: errString("unexpected end of JSON input"), want: true},
		{name: "other", err: errString("method not found"), want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isEmptyRawCDPResultError(tt.err); got != tt.want {
				t.Fatalf("isEmptyRawCDPResultError(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}

type errString string

func (e errString) Error() string { return string(e) }

func TestRunRawCDPWebSocket(t *testing.T) {
	upgrader := websocket.Upgrader{}
	srv := http.Server{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/devtools/page/target-1" {
				t.Errorf("path = %q, want /devtools/page/target-1", r.URL.Path)
			}
			conn, err := upgrader.Upgrade(w, r, nil)
			if err != nil {
				t.Errorf("upgrade: %v", err)
				return
			}
			defer conn.Close()

			var req struct {
				ID     int64          `json:"id"`
				Method string         `json:"method"`
				Params map[string]any `json:"params"`
			}
			if err := conn.ReadJSON(&req); err != nil {
				t.Errorf("read request: %v", err)
				return
			}
			if req.Method != "Runtime.evaluate" {
				t.Errorf("method = %q, want Runtime.evaluate", req.Method)
			}
			if got := req.Params["expression"]; got != "document.title" {
				t.Errorf("expression = %#v, want document.title", got)
			}
			if err := conn.WriteJSON(map[string]any{
				"id": req.ID,
				"result": map[string]any{
					"result": map[string]any{
						"type":  "string",
						"value": "browser title",
					},
				},
			}); err != nil {
				t.Errorf("write response: %v", err)
			}
		}),
	}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		if err := srv.Serve(ln); err != nil && err != http.ErrServerClosed && !errors.Is(err, net.ErrClosed) {
			t.Errorf("serve: %v", err)
		}
	}()
	t.Cleanup(func() { _ = srv.Shutdown(context.Background()) })

	result, err := runRawCDPWebSocket(context.Background(), "ws://"+ln.Addr().String()+"/devtools/page/target-1", "Runtime.evaluate", map[string]any{
		"expression":    "document.title",
		"returnByValue": true,
	})
	if err != nil {
		t.Fatalf("runRawCDPWebSocket: %v", err)
	}
	remoteObject, ok := result["result"].(map[string]any)
	if !ok {
		t.Fatalf("result = %#v, want remote object", result)
	}
	if got := remoteObject["value"]; got != "browser title" {
		t.Fatalf("value = %#v, want browser title", got)
	}
}

// fakeTarget serves a target WebSocket whose handler receives each
// connection after the upgrade, and returns its ws URL.
func fakeTarget(t *testing.T, handle func(*websocket.Conn)) string {
	t.Helper()
	upgrader := websocket.Upgrader{}
	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		handle(conn)
	})}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go srv.Serve(ln)
	t.Cleanup(func() { srv.Close() })
	return "ws://" + ln.Addr().String() + "/devtools/page/x"
}

func TestRunRawCDPWebSocketFailures(t *testing.T) {
	tests := []struct {
		name    string
		timeout time.Duration
		handle  func(*websocket.Conn, chan struct{})
		wantCtx bool
	}{
		{
			name:    "no reply",
			timeout: 200 * time.Millisecond,
			handle: func(c *websocket.Conn, done chan struct{}) {
				var req map[string]any
				c.ReadJSON(&req)
				<-done
			},
			wantCtx: true,
		},
		{
			name:    "abrupt close",
			timeout: 5 * time.Second,
			handle: func(c *websocket.Conn, done chan struct{}) {
				var req map[string]any
				c.ReadJSON(&req)
				c.UnderlyingConn().Close()
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			done := make(chan struct{})
			defer close(done)
			u := fakeTarget(t, func(c *websocket.Conn) { tt.handle(c, done) })
			ctx, cancel := context.WithTimeout(context.Background(), tt.timeout)
			defer cancel()
			start := time.Now()
			result, err := runRawCDPWebSocket(ctx, u, "Runtime.evaluate", nil)
			if err == nil {
				t.Fatalf("runRawCDPWebSocket = %v, nil; want error", result)
			}
			if got := errors.Is(err, context.DeadlineExceeded); got != tt.wantCtx {
				t.Fatalf("err = %v; deadline exceeded = %v, want %v", err, got, tt.wantCtx)
			}
			if d := time.Since(start); d > tt.timeout+2*time.Second {
				t.Fatalf("returned after %v, want within %v", d, tt.timeout)
			}
		})
	}
}
